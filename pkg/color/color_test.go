package color

import (
	"os"
	"testing"

	"github.com/agentsmith-project/jvs/internal/terminal"
)

func TestEnabled(t *testing.T) {
	resetColorStateForTest(t)

	Enable()
	if !Enabled() {
		t.Error("expected colors to be enabled")
	}

	Disable()
	if Enabled() {
		t.Error("expected colors to be disabled")
	}
}

func TestEnableDisable(t *testing.T) {
	resetColorStateForTest(t)

	Enable()
	if !Enabled() {
		t.Error("expected colors to be enabled after Enable()")
	}

	Disable()
	if Enabled() {
		t.Error("expected colors to be disabled after Disable()")
	}
}

func TestColorFuncs(t *testing.T) {
	resetColorStateForTest(t)
	Enable()

	tests := []struct {
		name     string
		fn       func(string) string
		input    string
		contains string
	}{
		{"Redf", Redf, "test", Red},
		{"Greenf", Greenf, "test", Green},
		{"Yellowf", Yellowf, "test", Yellow},
		{"Bluef", Bluef, "test", Blue},
		{"Cyanf", Cyanf, "test", Cyan},
		{"Boldf", Boldf, "test", Bold},
		{"Dimf", Dimf, "test", DimCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if !containsString(result, tt.contains) {
				t.Errorf("%s(%q) = %q, expected to contain %q", tt.name, tt.input, result, tt.contains)
			}
			// Should always end with Reset
			if !containsString(result, Reset) {
				t.Errorf("%s(%q) = %q, expected to contain reset code", tt.name, tt.input, result)
			}
		})
	}
}

func TestColorFuncsDisabled(t *testing.T) {
	resetColorStateForTest(t)
	Disable()

	tests := []struct {
		name  string
		fn    func(string) string
		input string
	}{
		{"Redf", Redf, "test"},
		{"Greenf", Greenf, "test"},
		{"Success", Success, "test"},
		{"Error", Error, "test"},
		{"Warning", Warning, "test"},
		{"Info", Info, "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if result != tt.input {
				t.Errorf("%s(%q) = %q, expected %q (no color when disabled)", tt.name, tt.input, result, tt.input)
			}
		})
	}
}

func TestSpecializedFormatters(t *testing.T) {
	resetColorStateForTest(t)
	Enable()

	tests := []struct {
		name  string
		fn    func(string) string
		input string
		color string
	}{
		{"Success", Success, "ok", Green},
		{"Error", Error, "fail", Red},
		{"Warning", Warning, "warn", Yellow},
		{"Info", Info, "info", Cyan},
		{"SnapshotID", SnapshotID, "abc123", Cyan},
		{"Tag", Tag, "v1.0", Blue},
		{"Header", Header, "Title", Bold},
		{"Dim", Dim, "subtle", DimCode},
		{"Highlight", Highlight, "important", Yellow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if !containsString(result, tt.color) {
				t.Errorf("%s(%q) = %q, expected to contain color code", tt.name, tt.input, result)
			}
		})
	}
}

func TestFormattedFunctions(t *testing.T) {
	resetColorStateForTest(t)
	Enable()

	if result := Successf("test %d", 123); !containsString(result, Green) {
		t.Errorf("Successf() should contain green color code, got %q", result)
	}

	if result := Errorf("err %s", "x"); !containsString(result, Red) {
		t.Errorf("Errorf() should contain red color code, got %q", result)
	}

	if result := Warningf("warn %d", 42); !containsString(result, Yellow) {
		t.Errorf("Warningf() should contain yellow color code, got %q", result)
	}

	if result := Infof("info %s", "test"); !containsString(result, Cyan) {
		t.Errorf("Infof() should contain cyan color code, got %q", result)
	}
}

func TestCode(t *testing.T) {
	resetColorStateForTest(t)
	Enable()

	result := Code("jvs init")
	if !containsString(result, Bold) {
		t.Errorf("Code() should contain bold code, got %q", result)
	}
	if !containsString(result, Reset) {
		t.Errorf("Code() should contain reset code, got %q", result)
	}

	Disable()
	result = Code("test")
	if result != "test" {
		t.Errorf("Code() disabled should return plain text, got %q", result)
	}
	Enable()
}

func TestInitRespectsNoColorEnv(t *testing.T) {
	resetColorStateForTest(t)
	t.Setenv("NO_COLOR", "1")
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	Init(false)
	if Enabled() {
		t.Error("expected colors to be disabled when NO_COLOR is set")
	}
}

func TestInitRespectsNoColorFlag(t *testing.T) {
	resetColorStateForTest(t)
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	Init(true)
	if Enabled() {
		t.Error("expected colors to be disabled when noColorFlag is true")
	}
}

func TestInitDisablesColorInCI(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	Init(false)
	if Enabled() {
		t.Error("expected colors to be disabled in CI")
	}
}

func TestInitRespectsTermDumb(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "dumb")

	Init(false)
	if Enabled() {
		t.Error("expected colors to be disabled when TERM=dumb")
	}
}

func TestInitDisablesColorWhenStdoutIsNotTerminal(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = r.Close()
		_ = w.Close()
	})

	Init(false)
	if Enabled() {
		t.Error("expected colors to be disabled when stdout is not a terminal")
	}
}

func TestInitDisablesColorWhenStdoutIsDevNull(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = devNull
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = devNull.Close()
	})

	Init(false)
	if Enabled() {
		t.Error("expected colors to be disabled when stdout is /dev/null")
	}
}

func TestInitNoColorFlagCanDisableAfterPriorInit(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	Init(false)
	if !Enabled() {
		t.Fatal("expected colors to be enabled before --no-color")
	}

	Init(true)
	if Enabled() {
		t.Error("expected --no-color to disable colors after prior initialization")
	}
}

func TestInitEnablesColorForInteractiveTerminal(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	Init(false)
	if !Enabled() {
		t.Error("expected colors to be enabled for an interactive terminal")
	}
}

func TestInitTracksStdoutAndStderrSeparately(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := terminal.WithIsTerminalForTest(func(file *os.File) bool {
		return file == os.Stdout
	})
	t.Cleanup(restoreTerminal)

	Init(false)
	if !Enabled() {
		t.Fatal("expected stdout colors to be enabled")
	}
	if EnabledFor(os.Stderr) {
		t.Fatal("expected stderr colors to be disabled")
	}
	if got := ErrorFor(os.Stderr, "jvs:"); got != "jvs:" {
		t.Fatalf("expected stderr formatter to stay plain, got %q", got)
	}
	if got := DimFor(os.Stderr, "  hint"); got != "  hint" {
		t.Fatalf("expected stderr dim formatter to stay plain, got %q", got)
	}
}

func TestEnabledForEvaluatesArbitraryFiles(t *testing.T) {
	resetColorStateForTest(t)
	unsetEnvForTest(t, "NO_COLOR")
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	interactiveR, interactiveW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create interactive pipe: %v", err)
	}
	plainR, plainW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create plain pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = interactiveR.Close()
		_ = interactiveW.Close()
		_ = plainR.Close()
		_ = plainW.Close()
	})
	restoreTerminal := terminal.WithIsTerminalForTest(func(file *os.File) bool {
		return file == interactiveW
	})
	t.Cleanup(restoreTerminal)

	Init(false)

	if !EnabledFor(interactiveW) {
		t.Fatal("expected colors to be enabled for the interactive file")
	}
	if EnabledFor(plainW) {
		t.Fatal("expected colors to be disabled for a non-interactive file")
	}
	if got := CodeFor(interactiveW, "jvs init"); !containsString(got, Bold) {
		t.Fatalf("expected CodeFor to colorize the interactive file, got %q", got)
	}
	if got := CodeFor(plainW, "jvs init"); got != "jvs init" {
		t.Fatalf("expected CodeFor to keep the non-interactive file plain, got %q", got)
	}
}

func resetColorStateForTest(t *testing.T) {
	t.Helper()
	origEnabled := state.enabled.Load()
	origDisabled := state.disabled.Load()
	origOverridden := state.overridden.Load()
	origInitialized := state.initialized.Load()
	t.Cleanup(func() {
		state.enabled.Store(origEnabled)
		state.disabled.Store(origDisabled)
		state.overridden.Store(origOverridden)
		state.initialized.Store(origInitialized)
	})
	state.enabled.Store(false)
	state.disabled.Store(false)
	state.overridden.Store(false)
	state.initialized.Store(false)
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	original, exists := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(key, original)
			return
		}
		_ = os.Unsetenv(key)
	})
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
