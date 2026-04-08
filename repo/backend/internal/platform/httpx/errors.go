package httpx

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// APIError is the normalized error response envelope returned by every
// endpoint. It matches the project's error contract exactly.
type APIError struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody holds the details inside the "error" key.
type ErrorBody struct {
	Code        string              `json:"code"`
	Message     string              `json:"message"`
	FieldErrors map[string][]string `json:"field_errors,omitempty"`
	RequestID   string              `json:"request_id,omitempty"`
}

// NewAPIError creates an APIError with the given HTTP status code, error code,
// and human-readable message.
func NewAPIError(httpStatus int, code, message string) *echo.HTTPError {
	return &echo.HTTPError{
		Code: httpStatus,
		Message: APIError{
			Error: ErrorBody{
				Code:    code,
				Message: message,
			},
		},
	}
}

// NewValidationError creates a 422 Unprocessable Entity error with per-field
// validation messages.
func NewValidationError(fieldErrors map[string][]string) *echo.HTTPError {
	return &echo.HTTPError{
		Code: http.StatusUnprocessableEntity,
		Message: APIError{
			Error: ErrorBody{
				Code:        "validation_error",
				Message:     "One or more fields failed validation.",
				FieldErrors: fieldErrors,
			},
		},
	}
}

// NewNotFoundError creates a 404 Not Found error.
func NewNotFoundError(message string) *echo.HTTPError {
	return NewAPIError(http.StatusNotFound, "not_found", message)
}

// NewForbiddenError creates a 403 Forbidden error.
func NewForbiddenError(message string) *echo.HTTPError {
	return NewAPIError(http.StatusForbidden, "forbidden", message)
}

// NewConflictError creates a 409 Conflict error.
func NewConflictError(code, message string) *echo.HTTPError {
	return NewAPIError(http.StatusConflict, code, message)
}

// ErrorHandler is a custom Echo error handler that ensures every error
// response conforms to the normalized error contract.
func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	he, ok := err.(*echo.HTTPError)
	if !ok {
		// Unexpected error - wrap it
		he = NewAPIError(http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}

	// If the message is already our APIError, inject the request ID and send.
	if apiErr, ok := he.Message.(APIError); ok {
		apiErr.Error.RequestID = requestID
		_ = c.JSON(he.Code, apiErr)
		return
	}

	// Fallback for Echo's own errors (404 route not found, etc.)
	msg := "An unexpected error occurred."
	if m, ok := he.Message.(string); ok {
		msg = m
	}

	resp := APIError{
		Error: ErrorBody{
			Code:      http.StatusText(he.Code),
			Message:   msg,
			RequestID: requestID,
		},
	}
	_ = c.JSON(he.Code, resp)
}
