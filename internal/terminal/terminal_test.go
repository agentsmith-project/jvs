package terminal

import (
	"os"
	"sync"
	"testing"
)

func TestIsTerminalRejectsPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	if IsTerminal(w) {
		t.Fatal("expected pipe writer not to be a terminal")
	}
}

func TestIsTerminalRejectsDevNull(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	t.Cleanup(func() { _ = devNull.Close() })

	if IsTerminal(devNull) {
		t.Fatal("expected /dev/null not to be a terminal")
	}
}

func TestIsInteractiveDisablesCI(t *testing.T) {
	resetTerminalStateForTest(t)
	t.Setenv("CI", "true")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	if ControlOutputAllowed() {
		t.Fatal("expected CI to disable terminal control output")
	}
	if IsInteractive(os.Stdout) {
		t.Fatal("expected CI to disable interactive terminal policy")
	}
}

func TestIsInteractiveDisablesTermDumb(t *testing.T) {
	resetTerminalStateForTest(t)
	t.Setenv("TERM", "dumb")
	restoreTerminal := WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	if ControlOutputAllowed() {
		t.Fatal("expected TERM=dumb to disable terminal control output")
	}
	if IsInteractive(os.Stdout) {
		t.Fatal("expected TERM=dumb to disable interactive terminal policy")
	}
}

func TestIsInteractiveAllowsTerminal(t *testing.T) {
	resetTerminalStateForTest(t)
	unsetEnvForTest(t, "CI")
	unsetEnvForTest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)

	if !IsInteractive(os.Stdout) {
		t.Fatal("expected interactive terminal policy to allow a real terminal")
	}
}

func TestWithIsTerminalForTestIsRaceSafe(t *testing.T) {
	resetTerminalStateForTest(t)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				isTTY := j%2 == 0
				restoreTerminal := WithIsTerminalForTest(func(*os.File) bool { return isTTY })
				_ = IsTerminal(os.Stdout)
				restoreTerminal()
			}
		}()
	}
	wg.Wait()
}

func resetTerminalStateForTest(t *testing.T) {
	t.Helper()
	isTerminalMu.Lock()
	original := isTerminal
	isTerminal = defaultIsTerminal
	isTerminalMu.Unlock()
	t.Cleanup(func() {
		isTerminalMu.Lock()
		isTerminal = original
		isTerminalMu.Unlock()
	})
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
