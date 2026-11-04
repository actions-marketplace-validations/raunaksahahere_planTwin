// Package errs holds user-facing errors. Every error says what happened, why, and what to do next.
package errs

import (
	"fmt"
	"strings"
)

// User is an error meant for people. The CLI prints it without a stack trace.
type User struct {
	What        string // what happened
	Why         string // why it happened
	Fix         string // what to do next
	Environment bool   // true when Docker or another required tool is missing or not running (exit code 3)
	Err         error  // underlying error, shown with --verbose only
}

func (e *User) Error() string {
	var b strings.Builder
	b.WriteString(e.What)
	if e.Why != "" {
		b.WriteString("\n  Why: " + e.Why)
	}
	if e.Fix != "" {
		b.WriteString("\n  Fix: " + e.Fix)
	}
	return b.String()
}

// Unwrap exposes the underlying error to errors.Is and errors.As.
func (e *User) Unwrap() error { return e.Err }

// New builds a user error. The format string describes the underlying cause; it is shown with --verbose.
func New(what, fix, format string, args ...any) *User {
	return &User{What: what, Fix: fix, Err: fmt.Errorf(format, args...)}
}

// Env builds a user error for a missing or stopped environment (Docker, Git). It uses exit code 3.
func Env(what, fix, format string, args ...any) *User {
	u := New(what, fix, format, args...)
	u.Environment = true
	return u
}
