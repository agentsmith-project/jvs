// Package terminal centralizes interactive terminal detection for CLI output.
package terminal

import (
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

var (
	isTerminalMu sync.RWMutex
	isTerminal   = defaultIsTerminal
)

// IsTerminal reports whether file is an actual terminal.
func IsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}

	isTerminalMu.RLock()
	detect := isTerminal
	isTerminalMu.RUnlock()

	return detect(file)
}

// WithIsTerminalForTest overrides terminal detection and returns a restore
// function. It is intended for tests that need deterministic TTY policy.
func WithIsTerminalForTest(fn func(*os.File) bool) func() {
	isTerminalMu.Lock()
	previous := isTerminal
	if fn == nil {
		isTerminal = defaultIsTerminal
	} else {
		isTerminal = fn
	}
	isTerminalMu.Unlock()

	return func() {
		isTerminalMu.Lock()
		isTerminal = previous
		isTerminalMu.Unlock()
	}
}

// IsInteractive reports whether terminal control output is appropriate.
func IsInteractive(file *os.File) bool {
	return ControlOutputAllowed() && IsTerminal(file)
}

// ControlOutputAllowed reports whether the environment allows terminal control output.
func ControlOutputAllowed() bool {
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if TruthyEnv("CI") || TruthyEnv("GITHUB_ACTIONS") {
		return false
	}
	return true
}

// TruthyEnv reports whether an environment variable is set to a truthy value.
func TruthyEnv(name string) bool {
	value, exists := os.LookupEnv(name)
	if !exists {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func defaultIsTerminal(file *os.File) bool {
	return term.IsTerminal(int(file.Fd()))
}
