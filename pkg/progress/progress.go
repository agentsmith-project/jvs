// Package progress provides progress reporting for long-running operations.
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/agentsmith-project/jvs/internal/terminal"
)

// Callback receives progress updates during long operations.
type Callback func(op string, current, total int, message string)

// Noop is a no-op callback for default behavior.
func Noop(op string, current, total int, message string) {}

// AutoEnabled reports whether terminal progress output is appropriate.
func AutoEnabled(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	return terminal.IsInteractive(file)
}

// Progress tracks operation progress.
type Progress struct {
	Op      string
	Total   int
	current int
	cb      Callback
}

// New creates a new Progress tracker.
func New(op string, total int, cb Callback) *Progress {
	if cb == nil {
		cb = Noop
	}
	return &Progress{Op: op, Total: total, cb: cb}
}

// Increment advances the progress and calls the callback.
func (p *Progress) Increment(message string) {
	p.current++
	p.cb(p.Op, p.current, p.Total, message)
}

// Set sets the current progress value.
func (p *Progress) Set(current int, message string) {
	p.current = current
	p.cb(p.Op, p.current, p.Total, message)
}

// Done marks the operation as complete.
func (p *Progress) Done(message string) {
	p.current = p.Total
	p.cb(p.Op, p.current, p.Total, message)
}

// Current returns the current progress value.
func (p *Progress) Current() int {
	return p.current
}

// Terminal provides a terminal-based progress bar.
type Terminal struct {
	writer      io.Writer
	op          string
	total       int
	current     atomic.Int64
	lastLineLen atomic.Int64
	enabled     atomic.Bool
}

// NewTerminal creates a new terminal progress bar.
func NewTerminal(op string, total int, enabled bool) *Terminal {
	t := &Terminal{
		writer: os.Stderr,
		op:     op,
		total:  total,
	}
	t.enabled.Store(enabled)
	t.current.Store(0)
	return t
}

// Callback returns a Callback function for this terminal.
func (t *Terminal) Callback() Callback {
	return func(op string, current, total int, message string) {
		if !t.enabled.Load() {
			return
		}
		t.current.Store(int64(current))
		t.render(message)
	}
}

// render draws the progress bar.
func (t *Terminal) render(message string) {
	current := t.current.Load()
	total := int64(t.total)

	if total <= 0 {
		total = 1
	}

	percentage := float64(current) / float64(total) * 100

	// Build the progress bar
	barWidth := 30
	filled := int(float64(barWidth) * float64(current) / float64(total))
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)

	// Clear previous line
	clear := "\r"
	if lastLen := t.lastLineLen.Load(); lastLen > 0 {
		clear = "\r" + strings.Repeat(" ", int(lastLen)) + "\r"
	}

	// Format line
	line := fmt.Sprintf("%s%s [%s] %d/%d (%.0f%%)", clear, t.op, bar, current, total, percentage)
	if message != "" {
		line += " " + message
	}

	fmt.Fprint(t.writer, line)
	t.lastLineLen.Store(int64(len(line)))
}

// Done marks the operation as complete and prints a final newline.
func (t *Terminal) Done(message string) {
	if !t.enabled.Load() {
		return
	}
	t.current.Store(int64(t.total))
	t.render(message)
	fmt.Fprintln(t.writer)
}

// SetEnabled enables or disables the progress bar.
func (t *Terminal) SetEnabled(enabled bool) {
	t.enabled.Store(enabled)
}

// IsEnabled returns whether the progress bar is enabled.
func (t *Terminal) IsEnabled() bool {
	return t.enabled.Load()
}

// CountingTerminal is a progress bar for counting operations where the total isn't known upfront.
type CountingTerminal struct {
	writer  io.Writer
	op      string
	current atomic.Int32
	enabled atomic.Bool
}

// NewCountingTerminal creates a new counting terminal progress bar.
func NewCountingTerminal(op string, enabled bool) *CountingTerminal {
	t := &CountingTerminal{
		writer: os.Stderr,
		op:     op,
	}
	t.enabled.Store(enabled)
	t.current.Store(0)
	return t
}

// Increment advances the counter.
func (t *CountingTerminal) Increment() {
	if !t.enabled.Load() {
		return
	}
	current := t.current.Add(1)
	t.render(fmt.Sprintf("%d items", current))
}

// render draws the current count.
func (t *CountingTerminal) render(message string) {
	_ = t.current.Load() // Read for consistency

	// Clear previous line
	clear := "\r"
	if lastLen := t.lastLineLen(); lastLen > 0 {
		clear = "\r" + strings.Repeat(" ", lastLen) + "\r"
	}

	// Format line
	line := fmt.Sprintf("%s... %s", t.op, message)

	fmt.Fprint(t.writer, clear+line)
}

// lastLineLen returns the approximate length of the last line.
func (t *CountingTerminal) lastLineLen() int {
	current := t.current.Load()
	return len(t.op) + 10 + len(fmt.Sprintf("%d", current))
}

// Done marks the operation as complete.
func (t *CountingTerminal) Done(finalMessage string) {
	if !t.enabled.Load() {
		return
	}
	clear := "\r" + strings.Repeat(" ", 100) + "\r"
	if finalMessage != "" {
		fmt.Fprint(t.writer, clear+finalMessage+"\n")
	} else {
		current := t.current.Load()
		fmt.Fprint(t.writer, clear+fmt.Sprintf("%s complete (%d items)\n", t.op, current))
	}
}

// SetEnabled enables or disables the progress bar.
func (t *CountingTerminal) SetEnabled(enabled bool) {
	t.enabled.Store(enabled)
}
