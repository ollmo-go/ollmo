package errs

import "fmt"

type Code int

const (
	CodeOK Code = iota
	CodeBadRequest
	CodeUnauthorized
	CodeForbidden
	CodeNotFound
	CodeConflict
	CodeTenantMismatch
	CodeInternal
)

// Error is a typed application error with HTTP status mapping.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// HTTPStatus maps the code to an HTTP status code.
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case CodeOK:
		return 200
	case CodeBadRequest:
		return 400
	case CodeUnauthorized:
		return 401
	case CodeForbidden:
		return 403
	case CodeNotFound:
		return 404
	case CodeConflict:
		return 409
	case CodeTenantMismatch:
		return 403
	default:
		return 500
	}
}

func New(code Code, message string) *Error { return &Error{Code: code, Message: message} }

func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// Common error constructors.
func BadRequest(msg string) *Error   { return New(CodeBadRequest, msg) }
func Unauthorized(msg string) *Error { return New(CodeUnauthorized, msg) }
func Forbidden(msg string) *Error    { return New(CodeForbidden, msg) }
func NotFound(msg string) *Error     { return New(CodeNotFound, msg) }
func Conflict(msg string) *Error     { return New(CodeConflict, msg) }
func Internal(msg string) *Error     { return New(CodeInternal, msg) }
