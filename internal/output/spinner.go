package output

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
)

// Spinner wraps a spinner for progress indication.
type Spinner struct {
	s       *spinner.Spinner
	enabled bool
}

// NewSpinner creates a new spinner.
func NewSpinner(message string) *Spinner {
	// Disable spinner if not a TTY or NO_COLOR is set
	enabled := isTerminal() && os.Getenv("NO_COLOR") == ""

	s := &Spinner{enabled: enabled}

	if enabled {
		s.s = spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
		s.s.Suffix = " " + message
		_ = s.s.Color("cyan")
	}

	return s
}

// Start starts the spinner.
func (s *Spinner) Start() {
	if s.enabled && s.s != nil {
		s.s.Start()
	}
}

// Stop stops the spinner.
func (s *Spinner) Stop() {
	if s.enabled && s.s != nil {
		s.s.Stop()
	}
}

// Success stops the spinner with a success message.
func (s *Spinner) Success(message string) {
	s.Stop()
	if os.Getenv("NO_COLOR") == "" {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", Green("✓ "+message))
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "✓ %s\n", message)
	}
}

// Fail stops the spinner with a failure message.
func (s *Spinner) Fail(message string) {
	s.Stop()
	if os.Getenv("NO_COLOR") == "" {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", Red("✗ "+message))
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "✗ %s\n", message)
	}
}

// UpdateMessage updates the spinner message.
func (s *Spinner) UpdateMessage(message string) {
	if s.enabled && s.s != nil {
		s.s.Suffix = " " + message
	}
}

// isTerminal checks if stdout is a terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
