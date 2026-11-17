// Package apierr holds errors with a stable, machine-readable code. The portal API puts
// the code into extensions.code, so a client -- the GUI or a script -- can react without
// parsing the English message.
package apierr

import "errors"

// Error is an error with a stable code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// New builds a coded error.
func New(code, msg string) *Error { return &Error{Code: code, Message: msg} }

// WithDetail keeps the code and adds a detail to the message.
func WithDetail(e *Error, detail string) error {
	return &Error{Code: e.Code, Message: e.Message + ": " + detail}
}

// As reports the coded error in err's chain, if there is one.
func As(err error) (*Error, bool) {
	var c *Error
	if errors.As(err, &c) {
		return c, true
	}
	return nil, false
}
