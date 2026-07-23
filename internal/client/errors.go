package client

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when the API responds with HTTP 404.
// Resources map it to state removal.
var ErrNotFound = errors.New("not found")

// DokployError is the parsed tRPC error envelope.
type DokployError struct {
	Code       string
	Message    string
	HTTPStatus int
	Method     string
	Path       string
}

func (e *DokployError) Error() string {
	return fmt.Sprintf("%s %s: %s (code %s, http %d)", e.Method, e.Path, e.Message, e.Code, e.HTTPStatus)
}
