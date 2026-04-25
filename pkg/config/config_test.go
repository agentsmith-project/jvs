package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.DefaultEngine != "" {
		t.Errorf("expected empty engine, got %s", cfg.DefaultEngine)
	}
	if cfg.DefaultTags != nil {
		t.Errorf("expected nil tags, got %v", cfg.DefaultTags)
	}
	if cfg.OutputFormat != "" {
		t.Errorf("expected empty output_format, got %s", cfg.OutputFormat)
	}
}

func TestLoad_NotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("expected config, got nil")
	}
	// Should return default config
	if cfg.DefaultEngine != "" {
		t.Errorf("expected empty default_engine, got %s", cfg.DefaultEngine)
	}
}

func TestLoad_Exists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .jvs directory and config file
	jvsDir := filepath.Join(tmpDir, ".jvs")
	if err := os.MkdirAll(jvsDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
default_engine: juicefs-clone
default_tags:
  - auto
  - dev
output_format: json
progress_enabled: false
`
	configPath := filepath.Join(jvsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cfg.DefaultEngine != "juicefs-clone" {
		t.Errorf("expected default_engine 'juicefs-clone', got %s", cfg.DefaultEngine)
	}
	if len(cfg.DefaultTags) != 2 {
		t.Errorf("expected 2 default tags, got %d", len(cfg.DefaultTags))
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("expected output_format 'json', got %s", cfg.OutputFormat)
	}
	if cfg.ProgressEnabled == nil || *cfg.ProgressEnabled != false {
		t.Error("expected progress_enabled to be false")
	}
}

func TestSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	enabled := false
	cfg := &Config{
		DefaultEngine:   "copy",
		DefaultTags:     []string{"test"},
		OutputFormat:    "text",
		ProgressEnabled: &enabled,
	}

	if err := Save(tmpDir, cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".jvs", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file not created")
	}

	// Load and verify
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Errorf("unexpected error loading: %v", err)
	}
	if loaded.DefaultEngine != "copy" {
		t.Errorf("expected engine 'copy', got %s", loaded.DefaultEngine)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jvsDir := filepath.Join(tmpDir, ".jvs")
	if err := os.MkdirAll(jvsDir, 0755); err != nil {
		t.Fatal(err)
	}

	invalidYAML := `
default_engine: [this is invalid yaml
`
	configPath := filepath.Join(jvsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(tmpDir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_WithNewConfigFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .jvs directory and config file with new fields
	jvsDir := filepath.Join(tmpDir, ".jvs")
	if err := os.MkdirAll(jvsDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
default_engine: juicefs-clone
default_tags:
  - auto
  - dev
output_format: json
progress_enabled: false
`
	configPath := filepath.Join(jvsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if string(cfg.DefaultEngine) != "juicefs-clone" {
		t.Errorf("expected default_engine 'juicefs-clone', got %s", cfg.DefaultEngine)
	}
	if len(cfg.DefaultTags) != 2 {
		t.Errorf("expected 2 default tags, got %d", len(cfg.DefaultTags))
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("expected output_format 'json', got %s", cfg.OutputFormat)
	}
	if cfg.ProgressEnabled == nil || *cfg.ProgressEnabled != false {
		t.Error("expected progress_enabled to be false")
	}
}

func TestConfig_GetDefaultEngine(t *testing.T) {
	cfg := &Config{DefaultEngine: "juicefs-clone"}
	if cfg.GetDefaultEngine() != "juicefs-clone" {
		t.Error("expected juicefs-clone")
	}

	cfg2 := &Config{DefaultEngine: ""}
	if cfg2.GetDefaultEngine() != "" {
		t.Error("expected empty string for unset engine")
	}

	cfg3 := &Config{DefaultEngine: "auto"}
	if cfg3.GetDefaultEngine() != "" {
		t.Error("expected empty string for auto engine")
	}
}

func TestConfig_GetDefaultTags(t *testing.T) {
	cfg := &Config{DefaultTags: []string{"auto", "dev"}}
	tags := cfg.GetDefaultTags()
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}

	cfg2 := &Config{DefaultTags: nil}
	if cfg2.GetDefaultTags() != nil {
		t.Error("expected nil for unset tags")
	}
}

func TestConfig_GetOutputFormat(t *testing.T) {
	cfg := &Config{OutputFormat: "json"}
	if cfg.GetOutputFormat() != "json" {
		t.Error("expected json")
	}

	cfg2 := &Config{OutputFormat: ""}
	if cfg2.GetOutputFormat() != "" {
		t.Error("expected empty string for unset format")
	}
}

func TestConfig_Set(t *testing.T) {
	cfg := &Config{}

	// Set default_engine
	if err := cfg.Set("default_engine", "juicefs-clone"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cfg.DefaultEngine != "juicefs-clone" {
		t.Errorf("expected juicefs-clone, got %s", cfg.DefaultEngine)
	}

	// Set default_tags
	if err := cfg.Set("default_tags", "[\"auto\", \"dev\"]"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(cfg.DefaultTags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(cfg.DefaultTags))
	}

	// Set output_format
	if err := cfg.Set("output_format", "json"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("expected json, got %s", cfg.OutputFormat)
	}

	// Set progress_enabled
	if err := cfg.Set("progress_enabled", "true"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cfg.ProgressEnabled == nil || !*cfg.ProgressEnabled {
		t.Error("expected progress_enabled to be true")
	}

	// Test invalid key
	if err := cfg.Set("invalid_key", "value"); err == nil {
		t.Error("expected error for invalid key")
	}

	// Test invalid progress_enabled value
	if err := cfg.Set("progress_enabled", "invalid"); err == nil {
		t.Error("expected error for invalid progress_enabled value")
	}
}

func TestConfig_Get(t *testing.T) {
	enabled := true
	cfg := &Config{
		DefaultEngine:   "juicefs-clone",
		DefaultTags:     []string{"auto", "dev"},
		OutputFormat:    "json",
		ProgressEnabled: &enabled,
	}

	// Get each value
	val, err := cfg.Get("default_engine")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "juicefs-clone" {
		t.Errorf("expected juicefs-clone, got %s", val)
	}

	val, err = cfg.Get("default_tags")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val == "" || val == "[]" {
		t.Error("expected tags list")
	}

	val, err = cfg.Get("output_format")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "json" {
		t.Errorf("expected json, got %s", val)
	}

	val, err = cfg.Get("progress_enabled")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "true" {
		t.Errorf("expected true, got %s", val)
	}

	// Test invalid key
	_, err = cfg.Get("invalid_key")
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestConfig_Validate(t *testing.T) {
	cfg := &Config{}

	// Valid engines
	validEngines := []model.EngineType{"juicefs-clone", "reflink-copy", "copy", "auto"}
	for _, engine := range validEngines {
		cfg.DefaultEngine = engine
		if err := cfg.validate(); err != nil {
			t.Errorf("unexpected error for engine %s: %v", engine, err)
		}
	}

	// Invalid engine
	cfg.DefaultEngine = "invalid"
	if err := cfg.validate(); err == nil {
		t.Error("expected error for invalid engine")
	}

	// Valid output formats
	validFormats := []string{"text", "json", ""}
	for _, format := range validFormats {
		cfg.DefaultEngine = "" // reset to valid
		cfg.OutputFormat = format
		if err := cfg.validate(); err != nil {
			t.Errorf("unexpected error for format %s: %v", format, err)
		}
	}

	// Invalid output format
	cfg.OutputFormat = "invalid"
	if err := cfg.validate(); err == nil {
		t.Error("expected error for invalid output format")
	}
}

func TestKeys(t *testing.T) {
	keys := Keys()
	if len(keys) != 4 {
		t.Errorf("expected 4 keys, got %d", len(keys))
	}

	expectedKeys := map[string]bool{
		"default_engine":   false,
		"default_tags":     false,
		"output_format":    false,
		"progress_enabled": false,
	}

	for _, key := range keys {
		if _, ok := expectedKeys[key]; !ok {
			t.Errorf("unexpected key: %s", key)
		}
		expectedKeys[key] = true
	}

	for key, found := range expectedKeys {
		if !found {
			t.Errorf("missing key: %s", key)
		}
	}
}

// TestGetProgressEnabled tests the GetProgressEnabled method.
func TestGetProgressEnabled(t *testing.T) {
	t.Run("Returns pointer when set", func(t *testing.T) {
		enabled := true
		cfg := &Config{ProgressEnabled: &enabled}
		result := cfg.GetProgressEnabled()
		if result == nil {
			t.Error("expected non-nil result")
		} else if *result != true {
			t.Error("expected true")
		}
	})

	t.Run("Returns nil when not set", func(t *testing.T) {
		cfg := &Config{ProgressEnabled: nil}
		result := cfg.GetProgressEnabled()
		if result != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("Returns false pointer when set to false", func(t *testing.T) {
		disabled := false
		cfg := &Config{ProgressEnabled: &disabled}
		result := cfg.GetProgressEnabled()
		if result == nil {
			t.Error("expected non-nil result")
		} else if *result != false {
			t.Error("expected false")
		}
	})
}

// TestGetRetentionPolicy tests the GetRetentionPolicy method.
func TestGetRetentionPolicy(t *testing.T) {
	t.Run("Returns default policy when retention is nil", func(t *testing.T) {
		cfg := &Config{Retention: nil}
		policy := cfg.GetRetentionPolicy()
		assert.Equal(t, model.DefaultRetentionPolicy(), policy)
	})

	t.Run("Returns configured policy when retention is set", func(t *testing.T) {
		cfg := &Config{
			Retention: &RetentionPolicy{
				Keep:   10,
				Within: "48h",
			},
		}
		policy := cfg.GetRetentionPolicy()
		if policy.KeepMinSnapshots != 10 {
			t.Errorf("expected 10 KeepMinSnapshots, got %d", policy.KeepMinSnapshots)
		}
		// 48h = 48 * 60 * 60 * 1e9 = 172800000000000 ns
		expectedDuration := 48 * time.Hour
		if policy.KeepMinAge != expectedDuration {
			t.Errorf("expected %v KeepMinAge, got %v", expectedDuration, policy.KeepMinAge)
		}
	})

	t.Run("Partial config uses defaults for unset fields", func(t *testing.T) {
		cfg := &Config{
			Retention: &RetentionPolicy{
				Keep:   5,
				Within: "",
			},
		}
		policy := cfg.GetRetentionPolicy()
		if policy.KeepMinSnapshots != 5 {
			t.Errorf("expected 5 KeepMinSnapshots, got %d", policy.KeepMinSnapshots)
		}
		assert.Equal(t, model.DefaultRetentionPolicy().KeepMinAge, policy.KeepMinAge)
	})

	t.Run("Only Within set uses default for Keep", func(t *testing.T) {
		cfg := &Config{
			Retention: &RetentionPolicy{
				Keep:   0,
				Within: "168h", // 7 days in hours
			},
		}
		policy := cfg.GetRetentionPolicy()
		if policy.KeepMinSnapshots != 0 {
			t.Errorf("expected 0 KeepMinSnapshots, got %d", policy.KeepMinSnapshots)
		}
		// 168h = 7 * 24 hours
		expectedDuration := 168 * time.Hour
		if policy.KeepMinAge != expectedDuration {
			t.Errorf("expected %v KeepMinAge, got %v", expectedDuration, policy.KeepMinAge)
		}
	})

	t.Run("Invalid Within string uses default", func(t *testing.T) {
		cfg := &Config{
			Retention: &RetentionPolicy{
				Keep:   0,
				Within: "invalid-duration",
			},
		}
		policy := cfg.GetRetentionPolicy()
		// Invalid legacy config values fall back to the v0 default: no age retention.
		assert.Equal(t, model.DefaultRetentionPolicy(), policy)
	})
}

// TestInvalidateCache tests the InvalidateCache function.
func TestInvalidateCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a config file
	jvsDir := filepath.Join(tmpDir, ".jvs")
	if err := os.MkdirAll(jvsDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
default_engine: reflink-copy
default_tags:
  - cached
`
	configPath := filepath.Join(jvsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load the config (will be cached)
	cfg1, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg1.DefaultEngine != "reflink-copy" {
		t.Errorf("expected reflink-copy, got %s", cfg1.DefaultEngine)
	}

	// Modify the config file directly
	configContent = `
default_engine: juicefs-clone
default_tags:
  - modified
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load again without invalidate - should get cached value
	cfg2, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.DefaultEngine != "reflink-copy" {
		t.Errorf("expected cached value reflink-copy, got %s", cfg2.DefaultEngine)
	}

	// Invalidate cache
	InvalidateCache(tmpDir)

	// Load again - should get new value
	cfg3, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg3.DefaultEngine != "juicefs-clone" {
		t.Errorf("expected juicefs-clone after invalidate, got %s", cfg3.DefaultEngine)
	}

	// Invalidate non-existent cache path (should not panic)
	InvalidateCache("/nonexistent/path/that/does/not/exist")
}

// TestSave_InvalidConfig tests Save with invalid configuration.
func TestSave_InvalidConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-save-invalid-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with invalid engine
	cfg := &Config{
		DefaultEngine: "invalid-engine-type",
	}

	err = Save(tmpDir, cfg)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

// TestSave_WithRetentionPolicy tests Save with retention policy.
func TestSave_WithRetentionPolicy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-retention-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		DefaultEngine: "copy",
		Retention: &RetentionPolicy{
			Keep:   20,
			Within: "72h",
		},
	}

	if err := Save(tmpDir, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Load and verify retention policy
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if loaded.Retention == nil {
		t.Fatal("expected retention policy to be loaded")
	}
	if loaded.Retention.Keep != 20 {
		t.Errorf("expected Keep=20, got %d", loaded.Retention.Keep)
	}
	if loaded.Retention.Within != "72h" {
		t.Errorf("expected Within=72h, got %s", loaded.Retention.Within)
	}
}

// TestConfig_Get_UnsetValues tests Get method with unset values.
func TestConfig_Get_UnsetValues(t *testing.T) {
	cfg := &Config{} // All fields unset

	// Get unset default_engine should return empty
	val, err := cfg.Get("default_engine")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %s", val)
	}

	// Get unset default_tags should return empty array representation
	val, err = cfg.Get("default_tags")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "[]" {
		t.Errorf("expected '[]', got %s", val)
	}

	// Get unset output_format should return empty
	val, err = cfg.Get("output_format")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %s", val)
	}

	// Get unset progress_enabled should return empty
	val, err = cfg.Get("progress_enabled")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %s", val)
	}
}

// TestConfig_GetWithNilProgressEnabled tests Get when ProgressEnabled is nil.
func TestConfig_GetWithNilProgressEnabled(t *testing.T) {
	cfg := &Config{
		DefaultEngine:   "copy",
		ProgressEnabled: nil,
	}

	val, err := cfg.Get("progress_enabled")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for nil ProgressEnabled, got %s", val)
	}
}

// TestConfig_SetInvalidValues tests Set with invalid values.
func TestConfig_SetInvalidValues(t *testing.T) {
	cfg := &Config{}

	// Test invalid default_tags (not valid YAML list)
	err := cfg.Set("default_tags", "not-a-list")
	if err == nil {
		t.Error("expected error for invalid default_tags format")
	}

	// Test invalid output_format (currently not validated by Set, but should be)
	// This test documents current behavior
	_ = cfg.Set("output_format", "xml")
	if cfg.OutputFormat != "xml" {
		t.Errorf("expected output_format to be set to xml, got %s", cfg.OutputFormat)
	}

	// Test unknown key
	err = cfg.Set("unknown_key", "value")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

// TestLoad_WithRetentionPolicy tests loading a config with retention policy.
func TestLoad_WithRetentionPolicy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-retention-load-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .jvs directory and config file
	jvsDir := filepath.Join(tmpDir, ".jvs")
	if err := os.MkdirAll(jvsDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
default_engine: copy
retention:
  keep: 15
  within: 30d
`
	configPath := filepath.Join(jvsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Retention == nil {
		t.Fatal("expected retention policy to be loaded")
	}
	if cfg.Retention.Keep != 15 {
		t.Errorf("expected Keep=15, got %d", cfg.Retention.Keep)
	}
	if cfg.Retention.Within != "30d" {
		t.Errorf("expected Within=30d, got %s", cfg.Retention.Within)
	}
}

// TestLoad_WithInvalidRetentionDuration tests loading with invalid duration.
func TestLoad_WithInvalidRetentionDuration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jvs-config-invalid-retention-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .jvs directory and config file
	jvsDir := filepath.Join(tmpDir, ".jvs")
	if err := os.MkdirAll(jvsDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
default_engine: copy
retention:
  keep: -5
  within: "not-a-duration"
`
	configPath := filepath.Join(jvsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load should succeed (validation is on GetRetentionPolicy, not Load)
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Getting the retention policy should use v0 defaults for invalid values:
	// no count retention and no age retention.
	policy := cfg.GetRetentionPolicy()
	assert.Equal(t, model.DefaultRetentionPolicy(), policy)
}

func TestLoad_CorruptedYAML(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := repo.Init(tmpDir, "test-repo")
	require.NoError(t, err)

	cfgPath := filepath.Join(tmpDir, ".jvs", "config.yaml")
	err = os.WriteFile(cfgPath, []byte("{{{{not: valid: yaml: \x00\xff"), 0644)
	require.NoError(t, err)

	InvalidateCache(tmpDir)

	_, err = Load(tmpDir)
	assert.Error(t, err)
}

func TestLoad_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := repo.Init(tmpDir, "test-repo")
	require.NoError(t, err)

	cfgPath := filepath.Join(tmpDir, ".jvs", "config.yaml")
	err = os.WriteFile(cfgPath, []byte("default_engine: copy\noutput_format: json\n"), 0644)
	require.NoError(t, err)

	InvalidateCache(tmpDir)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, loadErr := Load(tmpDir)
			assert.NoError(t, loadErr)
			assert.NotNil(t, cfg)
		}()
	}
	wg.Wait()
}

func TestLoad_CacheCopyIndependence(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := repo.Init(tmpDir, "test-repo")
	require.NoError(t, err)

	cfgPath := filepath.Join(tmpDir, ".jvs", "config.yaml")
	err = os.WriteFile(cfgPath, []byte("default_engine: copy\noutput_format: json\n"), 0644)
	require.NoError(t, err)

	InvalidateCache(tmpDir)

	cfg1, err := Load(tmpDir)
	require.NoError(t, err)
	require.Equal(t, model.EngineType("copy"), cfg1.DefaultEngine)

	cfg1.DefaultEngine = "reflink-copy"
	cfg1.OutputFormat = "text"

	cfg2, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, model.EngineType("copy"), cfg2.DefaultEngine, "cache should not be mutated by modifying a returned copy")
	assert.Equal(t, "json", cfg2.OutputFormat, "cache should not be mutated by modifying a returned copy")
}
