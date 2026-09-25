package github

import (
	"errors"
	"fmt"
)

// APIError is an error returned by a GitHub REST endpoint.
type APIError struct {
	Status  int
	Message string
	Body    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api: http %d: %s (body: %.200q)", e.Status, e.Message, e.Body)
}

// ErrAppNotInstalled reports that the GitHub App is not installed on the
// repository it was asked to act on.
var ErrAppNotInstalled = errors.New("github: app not installed on repository")
