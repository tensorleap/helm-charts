package local

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// IsStdinTerminal reports whether stdin is an interactive terminal. It is a
// variable so tests can simulate a shell without one.
var IsStdinTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// RequireTerminal fails when there is no terminal to answer a prompt (e.g. the
// installer runs from a script or an AI assistant's shell), instead of letting
// the prompt fail with a bare EOF. question says what would have been asked and
// howToAnswer how to answer it with flags. Call it right before the prompt,
// after the non-interactive (--yes) branch.
func RequireTerminal(question, howToAnswer string) error {
	if IsStdinTerminal() {
		return nil
	}
	return fmt.Errorf("no terminal to ask %s: %s", question, howToAnswer)
}
