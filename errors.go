package tap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// APIError is returned when Tap responds with a non-2xx status.
//
// Tap error bodies usually hold an errors array of {code, description}
// objects, sometimes a single error object instead. Body keeps the raw
// response for anything the parsed fields miss.
type APIError struct {
	// StatusCode is the HTTP response status code.
	StatusCode int
	// Errors are the error entries Tap returned.
	Errors []ErrorDetail
	// Message joins the error descriptions, or holds the raw body when it
	// could not be parsed.
	Message string
	// Body is the raw response body returned by Tap.
	Body []byte
	// Header holds the response headers.
	Header http.Header
}

// ErrorDetail is one entry of a Tap error response.
type ErrorDetail struct {
	// Code is Tap's numeric error code, e.g. "1108".
	Code string `json:"code"`
	// Error is a machine-readable error name, e.g. "REQUIRED_INPUTS_INVALID".
	Error string `json:"error,omitempty"`
	// Description is the human-readable error message.
	Description string `json:"description"`
}

func (e *APIError) Error() string {
	parts := []string{fmt.Sprintf("tap: status %d", e.StatusCode)}
	if len(e.Errors) > 0 && e.Errors[0].Code != "" {
		parts = append(parts, e.Errors[0].Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, ": ")
}

type errorBody struct {
	Errors []ErrorDetail `json:"errors"`
	Error  *ErrorDetail  `json:"error"`
}

func decodeAPIError(resp *http.Response, body []byte) error {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Body:       append([]byte(nil), body...),
		Header:     resp.Header.Clone(),
	}
	var parsed errorBody
	if len(body) > 0 && json.Unmarshal(body, &parsed) == nil {
		apiErr.Errors = parsed.Errors
		if len(apiErr.Errors) == 0 && parsed.Error != nil {
			apiErr.Errors = []ErrorDetail{*parsed.Error}
		}
	}
	var msgs []string
	for _, d := range apiErr.Errors {
		if d.Description != "" {
			msgs = append(msgs, d.Description)
		} else if d.Error != "" {
			msgs = append(msgs, d.Error)
		}
	}
	switch {
	case len(msgs) > 0:
		apiErr.Message = strings.Join(msgs, "; ")
	case len(body) > 0:
		apiErr.Message = string(body)
	default:
		apiErr.Message = http.StatusText(resp.StatusCode)
	}
	return apiErr
}
