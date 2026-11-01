package safetrade_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/anuketfluxus/safetrade-api-client"
)

func TestAPIErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body string
		want []string
	}{
		{body: `{"errors":["market.order.invalid_volume","market.order.invalid_price"]}`, want: []string{"market.order.invalid_volume", "market.order.invalid_price"}},
		{body: `{"errors":["authz.invalid_signature"],"detail":false}`, want: []string{"authz.invalid_signature"}},
		{body: `{"errors":[]}`, want: nil},
		{body: `{"error":"something"}`, want: nil},
		{body: `<html>Forbidden</html>`, want: nil},
		{body: ``, want: nil},
	}
	for _, tt := range tests {
		apiErr := &safetrade.APIError{StatusCode: 422, Body: []byte(tt.body)}
		if got := apiErr.Messages(); !slices.Equal(got, tt.want) {
			t.Errorf("Messages() for %s = %#v, want %#v", tt.body, got, tt.want)
		}
	}
}

func TestAPIErrorError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{
			name:       "JSON errors",
			statusCode: 422,
			body:       `{"errors":["market.order.invalid_volume","market.order.invalid_price"]}`,
			want:       "422 Unprocessable Entity: market.order.invalid_volume, market.order.invalid_price",
		},
		{
			name:       "HTML body",
			statusCode: 403,
			body:       "<html>\n  <title>Attention Required! | Cloudflare</title>\n</html>",
			want:       "403 Forbidden: <html> <title>Attention Required! | Cloudflare</title> </html>",
		},
		{
			name:       "empty body",
			statusCode: 500,
			body:       "",
			want:       "500 Internal Server Error",
		},
		{
			name:       "unknown status code",
			statusCode: 599,
			body:       "",
			want:       "599",
		},
	}
	for _, tt := range tests {
		apiErr := &safetrade.APIError{StatusCode: tt.statusCode, Body: []byte(tt.body)}
		if got := apiErr.Error(); got != tt.want {
			t.Errorf("%s: Error() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAPIErrorErrorTruncatesLongBodies(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("é", 500)
	apiErr := &safetrade.APIError{StatusCode: 502, Body: []byte(body)}
	got := apiErr.Error()
	want := "502 Bad Gateway: " + strings.Repeat("é", 200) + "…"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAPIErrorErrorReplacesInvalidUTF8(t *testing.T) {
	t.Parallel()

	apiErr := &safetrade.APIError{StatusCode: 400, Body: []byte("bad \xff\xfe body")}
	if got, want := apiErr.Error(), "400 Bad Request: bad � body"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAPIErrorIsMatchesSentinelsByStatusCode(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		safetrade.ErrBadRequest,
		safetrade.ErrUnauthorized,
		safetrade.ErrForbidden,
		safetrade.ErrNotFound,
		safetrade.ErrUnprocessableEntity,
		safetrade.ErrTooManyRequests,
		safetrade.ErrServerError,
	}
	tests := []struct {
		statusCode int
		want       error
	}{
		{statusCode: 400, want: safetrade.ErrBadRequest},
		{statusCode: 401, want: safetrade.ErrUnauthorized},
		{statusCode: 403, want: safetrade.ErrForbidden},
		{statusCode: 404, want: safetrade.ErrNotFound},
		{statusCode: 422, want: safetrade.ErrUnprocessableEntity},
		{statusCode: 429, want: safetrade.ErrTooManyRequests},
		{statusCode: 500, want: safetrade.ErrServerError},
		{statusCode: 503, want: safetrade.ErrServerError},
		{statusCode: 599, want: safetrade.ErrServerError},
		{statusCode: 409, want: nil},
		{statusCode: 600, want: nil},
	}
	for _, tt := range tests {

		err := fmt.Errorf("safetrade: GET /x: %w", &safetrade.APIError{StatusCode: tt.statusCode})
		for _, sentinel := range sentinels {
			if got, want := errors.Is(err, sentinel), errors.Is(tt.want, sentinel); got != want {
				t.Errorf("status %d: errors.Is(err, %q) = %v, want %v", tt.statusCode, sentinel, got, want)
			}
		}
		var apiErr *safetrade.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.statusCode {
			t.Errorf("status %d: errors.As did not find the *APIError", tt.statusCode)
		}
	}
}
