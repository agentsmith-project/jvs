// Package config provides configuration file support for JVS.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agentsmith-project/jvs/pkg/model"
	"gopkg.in/yaml.v3"
)

var (
	// cache is a per-repo config cache
	cache   = make(map[string]*Config)
	cacheMu sync.RWMutex
)

// Config represents the JVS configuration.
type Config struct {
	// DefaultEngine is the default snapshot engine to use.
	DefaultEngine model.EngineType `yaml:"default_engine,omitempty"`

	// DefaultTags are tags automatically added to each snapshot.
	DefaultTags []string `yaml:"default_tags,omitempty"`

	// OutputFormat is the default output format (text or json).
	OutputFormat string `yaml:"output_format,omitempty"`

	// ProgressEnabled enables progress bars by default.
	ProgressEnabled *bool `yaml:"progress_enabled,omitempty"`

	// Retention preserves legacy YAML compatibility for internal GC policy
	// conversion. The v0 public CLI does not expose retention configuration.
	Retention *RetentionPolicy `yaml:"retention,omitempty"`
}

// RetentionPolicy is a legacy config shape for optional internal GC retention.
// Unset or invalid values fall back to model.DefaultRetentionPolicy().
type RetentionPolicy struct {
	// Keep is the minimum number of snapshots to keep.
	Keep int `yaml:"keep,omitempty"`

	// Within is the minimum age before snapshots can be pruned (e.g., "24h", "168h").
	Within string `yaml:"within,omitempty"`
}

// Default returns the default configuration.
func Default() *Config {
	return &Config{
		DefaultEngine:   "",
		DefaultTags:     nil,
		OutputFormat:    "",
		ProgressEnabled: nil,
	}
}

// Load loads configuration from .jvs/config.yaml.
// Returns default config if file doesn't exist.
// The returned Config must not be modified; use Save() for changes.
func Load(repoRoot string) (*Config, error) {
	cacheMu.RLock()
	if cfg, ok := cache[repoRoot]; ok {
		cacheMu.RUnlock()
		return deepCopy(cfg), nil
	}
	cacheMu.RUnlock()

	cfg := Default()
	cfgPath := filepath.Join(repoRoot, ".jvs", "config.yaml")

	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		// No config file is OK, cache and return defaults
		cacheAndReturn(repoRoot, cfg)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Validate config
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cacheAndReturn(repoRoot, cfg)
	return cfg, nil
}

// Save writes configuration to .jvs/config.yaml.
func Save(repoRoot string, cfg *Config) error {
	if err := cfg.validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	cfgPath := filepath.Join(repoRoot, ".jvs", "config.yaml")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Update cache
	cacheAndReturn(repoRoot, cfg)
	return nil
}

// validate checks if the configuration is valid.
func (c *Config) validate() error {
	// Validate default_engine if set
	if c.DefaultEngine != "" {
		switch c.DefaultEngine {
		case model.EngineJuiceFSClone, model.EngineReflinkCopy, model.EngineCopy, "auto":
			// Valid
		default:
			return fmt.Errorf("invalid default_engine: %s (must be juicefs-clone, reflink-copy, copy, or auto)", c.DefaultEngine)
		}
	}

	// Validate output_format if set
	if c.OutputFormat != "" && c.OutputFormat != "text" && c.OutputFormat != "json" {
		return fmt.Errorf("invalid output_format: %s (must be text or json)", c.OutputFormat)
	}

	return nil
}

// GetDefaultEngine returns the default engine, or empty string if not set.
func (c *Config) GetDefaultEngine() model.EngineType {
	if c.DefaultEngine != "" && c.DefaultEngine != "auto" {
		return c.DefaultEngine
	}
	return ""
}

// GetDefaultTags returns the default tags.
func (c *Config) GetDefaultTags() []string {
	if c.DefaultTags != nil {
		return c.DefaultTags
	}
	return nil
}

// GetOutputFormat returns the output format, or empty string if not set.
func (c *Config) GetOutputFormat() string {
	return c.OutputFormat
}

// GetProgressEnabled returns whether progress is enabled.
// Returns nil if not configured (auto-detect based on terminal).
func (c *Config) GetProgressEnabled() *bool {
	return c.ProgressEnabled
}

// GetRetentionPolicy returns the legacy retention config as a model.RetentionPolicy.
func (c *Config) GetRetentionPolicy() model.RetentionPolicy {
	policy := model.DefaultRetentionPolicy()

	if c.Retention != nil {
		if c.Retention.Keep > 0 {
			policy.KeepMinSnapshots = c.Retention.Keep
		}
		if c.Retention.Within != "" {
			if d, err := time.ParseDuration(c.Retention.Within); err == nil {
				policy.KeepMinAge = d
			}
		}
	}

	return policy
}

// Set sets a configuration value by key.
func (c *Config) Set(key, value string) error {
	switch key {
	case "default_engine":
		c.DefaultEngine = model.EngineType(value)
	case "default_tags":
		// Parse as YAML list
		if err := yaml.Unmarshal([]byte(value), &c.DefaultTags); err != nil {
			return fmt.Errorf("parse tags: %w", err)
		}
	case "output_format":
		c.OutputFormat = value
	case "progress_enabled":
		var enabled bool
		switch value {
		case "true":
			enabled = true
		case "false":
			enabled = false
		default:
			return fmt.Errorf("invalid progress_enabled value: %s (must be true or false)", value)
		}
		c.ProgressEnabled = &enabled
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// Get gets a configuration value by key as a string.
func (c *Config) Get(key string) (string, error) {
	switch key {
	case "default_engine":
		if c.DefaultEngine == "" {
			return "", nil
		}
		return string(c.DefaultEngine), nil
	case "default_tags":
		if c.DefaultTags == nil {
			return "[]", nil
		}
		data, err := yaml.Marshal(c.DefaultTags)
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "output_format":
		return c.OutputFormat, nil
	case "progress_enabled":
		if c.ProgressEnabled == nil {
			return "", nil
		}
		if *c.ProgressEnabled {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

// Keys returns all valid configuration keys.
func Keys() []string {
	return []string{
		"default_engine",
		"default_tags",
		"output_format",
		"progress_enabled",
	}
}

// InvalidateCache clears the cached config for a repository.
func InvalidateCache(repoRoot string) {
	cacheMu.Lock()
	delete(cache, repoRoot)
	cacheMu.Unlock()
}

func deepCopy(cfg *Config) *Config {
	cp := *cfg
	if cfg.DefaultTags != nil {
		cp.DefaultTags = make([]string, len(cfg.DefaultTags))
		copy(cp.DefaultTags, cfg.DefaultTags)
	}
	if cfg.ProgressEnabled != nil {
		v := *cfg.ProgressEnabled
		cp.ProgressEnabled = &v
	}
	if cfg.Retention != nil {
		r := *cfg.Retention
		cp.Retention = &r
	}
	return &cp
}

// cacheAndReturn stores a deep copy of the config in cache so that
// the caller's pointer remains independent of the cached value.
func cacheAndReturn(repoRoot string, cfg *Config) {
	cacheMu.Lock()
	cache[repoRoot] = deepCopy(cfg)
	cacheMu.Unlock()
}
