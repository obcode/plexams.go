// Package apierr holds errors with a stable, machine-readable code. The portal API puts
// the code into extensions.code, so a client -- the GUI or a script -- can react without
// parsing the English message.
package apierr

import (
	"errors"
	"fmt"
)

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

// Codes of the planning API. The portal declares its own next to its services; these
// are the ones a planner can run into. A client reacts to the code, never to the
// wording of the message, so the wording is free to change.
const (
	// CodeForbidden: the role does not allow this call.
	CodeForbidden = "FORBIDDEN"
	// CodeReadOnlyRole: a read-only role tried to change data.
	CodeReadOnlyRole = "READ_ONLY_ROLE"
	// CodeSemesterReadOnly: the active semester is write-protected.
	CodeSemesterReadOnly = "SEMESTER_READ_ONLY"
	// CodeBusy: a validation, transfer, generation or mail run is in progress.
	CodeBusy = "BUSY"
	// CodePublishedLocked: the plan of this area is published, so it may not be regenerated.
	CodePublishedLocked = "PUBLISHED_LOCKED"
	// CodeAlreadySent: a send-once mail has been sent.
	CodeAlreadySent = "ALREADY_SENT"
	// CodeAlreadyExists: the thing to create is there already.
	CodeAlreadyExists = "ALREADY_EXISTS"
	// CodeNotFound: the thing asked for does not exist.
	CodeNotFound = "NOT_FOUND"
	// CodeInvalidInput: the input cannot be used as given.
	CodeInvalidInput = "INVALID_INPUT"
	// CodeExternalZPA, CodeExternalAnny, CodeExternalSMTP: a neighbouring system
	// refused or could not be reached.
	CodeExternalZPA  = "EXTERNAL_ZPA"
	CodeExternalAnny = "EXTERNAL_ANNY"
	CodeExternalSMTP = "EXTERNAL_SMTP"
	// CodeInternal: a fault in the server or its database; nothing the user can fix.
	CodeInternal = "INTERNAL"
)

// Newf builds a coded error with a formatted message.
func Newf(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

// Code returns the code of the coded error in err's chain, or "" if there is none.
func Code(err error) string {
	if c, ok := As(err); ok {
		return c.Code
	}
	return ""
}
