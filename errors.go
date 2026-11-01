package safetrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrMissingCredentials = errors.New("credentials required for this endpoint")

	ErrInvalidArgument = errors.New("invalid argument")
)

var (
	ErrBadRequest          = errors.New("bad request")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrNotFound            = errors.New("not found")
	ErrUnprocessableEntity = errors.New("unprocessable entity")
	ErrTooManyRequests     = errors.New("too many requests")
	ErrServerError         = errors.New("server error")
)

const (
	maxErrorBodySize = 64 << 10

	maxSnippetRunes = 200
)

type APIError struct {
	StatusCode int

	Body []byte
}

func (e *APIError) Messages() []string {
	var payload struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(e.Body, &payload); err != nil {
		return nil
	}
	return payload.Errors
}

func (e *APIError) Error() string {
	status := strconv.Itoa(e.StatusCode)
	if text := http.StatusText(e.StatusCode); text != "" {
		status += " " + text
	}
	if messages := e.Messages(); len(messages) > 0 {
		return status + ": " + strings.Join(messages, ", ")
	}
	if snippet := bodySnippet(e.Body); snippet != "" {
		return status + ": " + snippet
	}
	return status
}

func (e *APIError) Is(target error) bool {
	switch target {
	case ErrBadRequest:
		return e.StatusCode == http.StatusBadRequest
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrUnprocessableEntity:
		return e.StatusCode == http.StatusUnprocessableEntity
	case ErrTooManyRequests:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrServerError:
		return e.StatusCode >= 500 && e.StatusCode <= 599
	default:
		return false
	}
}

type DecodeError struct {
	StatusCode int

	Body []byte

	Err error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("decoding %d response: %v", e.StatusCode, e.Err)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}

func readAPIError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	return &APIError{StatusCode: resp.StatusCode, Body: body}
}

func bodySnippet(body []byte) string {
	text := strings.ToValidUTF8(string(body), "�")
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= maxSnippetRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxSnippetRunes]) + "…"
}

func invalidArgument(format string, args ...any) error {
	return fmt.Errorf("safetrade: %w: %s", ErrInvalidArgument, fmt.Sprintf(format, args...))
}

func MayHaveTakenEffect(err error) bool {
	var notSent *notSentError
	var apiErr *APIError
	switch {
	case errors.Is(err, ErrInvalidArgument), errors.As(err, &notSent):
		return false
	case errors.As(err, &apiErr):
		return apiErr.StatusCode < 400 || apiErr.StatusCode > 499
	default:
		return true
	}
}

type notSentError struct {
	err error
}

func (e *notSentError) Error() string {
	return e.err.Error()
}

func (e *notSentError) Unwrap() error {
	return e.err
}
