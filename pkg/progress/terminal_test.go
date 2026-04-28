package progress

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"sync/atomic"
)

func TestTerminal_Callback(t *testing.T) {
	var buf bytes.Buffer
	term := &Terminal{
		writer: &buf,
		op:     "test-op",
		total:  100,
	}

	term.enabled.Store(true)
	term.current.Store(0)

	cb := term.Callback()

	// Call callback
	cb("test-op", 50, 100, "halfway")

	output := buf.String()
	assert.Contains(t, output, "test-op")
	assert.Contains(t, output, "50/100")
	assert.Contains(t, output, "50%")
}

func TestTerminal_Done(t *testing.T) {
	var buf bytes.Buffer
	term := NewTerminal("test", 10, true)
	term.writer = &buf

	cb := term.Callback()

	// Progress to completion
	for i := 0; i < 10; i++ {
		cb("test", i+1, 10, "")
	}

	// Clear the buffer to check Done output
	buf.Reset()

	term.Done("complete")

	output := buf.String()
	assert.Contains(t, output, "complete")
}

func TestTerminal_Disabled(t *testing.T) {
	var buf bytes.Buffer
	term := NewTerminal("test", 10, false)
	term.writer = &buf

	cb := term.Callback()
	cb("test", 5, 10, "halfway")

	// No output when disabled
	assert.Equal(t, 0, buf.Len())
}

func TestCountingTerminal_Increment(t *testing.T) {
	var buf bytes.Buffer
	term := NewCountingTerminal("counting", true)
	term.writer = &buf

	term.Increment()
	term.Increment()
	term.Increment()

	output := buf.String()
	assert.Contains(t, output, "counting")
	assert.Contains(t, output, "items")
}

func TestCountingTerminal_Done(t *testing.T) {
	var buf bytes.Buffer
	term := NewCountingTerminal("counting", true)
	term.writer = &buf

	term.Increment()
	term.Increment()

	buf.Reset()
	term.Done("all done")

	output := buf.String()
	assert.Contains(t, output, "all done")
}

func TestTerminal_ProgressBar(t *testing.T) {
	var buf bytes.Buffer
	term := NewTerminal("copy", 100, true)
	term.writer = &buf

	cb := term.Callback()

	// 0% progress
	cb("copy", 0, 100, "")
	output1 := buf.String()
	assert.Contains(t, output1, "[") // Has progress bar brackets

	// 50% progress
	buf.Reset()
	cb("copy", 50, 100, "halfway")
	output2 := buf.String()
	assert.Contains(t, output2, "50%")
	assert.Contains(t, output2, "halfway")

	// 100% progress
	buf.Reset()
	cb("copy", 100, 100, "done")
	output3 := buf.String()
	assert.Contains(t, output3, "100%")
	assert.Contains(t, output3, "done")
}

func TestTerminal_SetEnabled(t *testing.T) {
	term := NewTerminal("test", 10, true)
	assert.True(t, term.IsEnabled())

	term.SetEnabled(false)
	assert.False(t, term.IsEnabled())

	term.SetEnabled(true)
	assert.True(t, term.IsEnabled())
}

func TestCountingTerminal_SetEnabled(t *testing.T) {
	term := NewCountingTerminal("test", true)
	term.SetEnabled(false)

	var buf bytes.Buffer
	term.writer = &buf

	term.Increment()
	assert.Equal(t, 0, buf.Len(), "no output when disabled")
}

func TestAutoEnabledDisablesNonTerminalWriters(t *testing.T) {
	var buf bytes.Buffer
	assert.False(t, AutoEnabled(&buf), "buffer output should not be treated as terminal progress")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	assert.False(t, AutoEnabled(w), "pipe output should not be treated as terminal progress")

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	assert.False(t, AutoEnabled(devNull), "/dev/null should not be treated as terminal progress")
}

func TestAutoEnabledDisablesCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("TERM", "xterm-256color")

	assert.False(t, AutoEnabled(os.Stderr), "CI output should not use terminal progress")
}

func TestAutoDisabledTerminalEmitsNoControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	term := NewTerminal("test", 10, AutoEnabled(&buf))
	term.writer = &buf

	cb := term.Callback()
	cb("test", 5, 10, "halfway")
	term.Done("complete")

	assert.NotContains(t, buf.String(), "\r")
	assert.Empty(t, buf.String())
}

func TestTerminal_ProgressBarFormat(t *testing.T) {
	var buf bytes.Buffer
	term := NewTerminal("test-op", 100, true)
	term.writer = &buf

	cb := term.Callback()
	cb("test-op", 25, 100, "processing")

	output := buf.String()

	// Check for expected elements
	assert.Contains(t, output, "test-op")
	assert.Contains(t, output, "[")
	assert.Contains(t, output, "]")
	assert.Contains(t, output, "25/100")
	assert.Contains(t, output, "25%")
	assert.Contains(t, output, "processing")

	// Check progress bar has roughly right amount of filled characters
	// 25% of 30 chars = 7-8 filled
	lines := strings.Split(output, "\r")
	lastLine := lines[len(lines)-1]
	equalCount := strings.Count(lastLine, "=")
	assert.GreaterOrEqual(t, equalCount, 5)
	assert.LessOrEqual(t, equalCount, 10)
}

func TestAtomicInt32(t *testing.T) {
	// Test that atomic.Int32 works as expected
	var val atomic.Int32
	val.Store(42)
	assert.Equal(t, int32(42), val.Load())

	val.Add(1)
	assert.Equal(t, int32(43), val.Load())
}
