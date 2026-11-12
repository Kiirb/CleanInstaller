// Package apperr carries a German message for the user alongside the real
// error, which goes to the log.
package apperr

import "errors"

// UserError pairs a user-facing German message with its cause.
type UserError struct {
	Msg string
	Err error
}

func (e *UserError) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return e.Msg + ": " + e.Err.Error()
}

func (e *UserError) Unwrap() error { return e.Err }

// New creates a UserError.
func New(msg string, err error) *UserError { return &UserError{Msg: msg, Err: err} }

// Message returns the German message to show for err.
func Message(err error) string {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Msg
	}
	return "Ein unerwarteter Fehler ist aufgetreten."
}
