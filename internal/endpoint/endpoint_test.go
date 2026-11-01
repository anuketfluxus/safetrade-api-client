package endpoint

import (
	"strings"
	"testing"
)

func TestParseBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{raw: "https://safe.trade/api/v2", want: "https://safe.trade/api/v2"},
		{raw: "https://safe.trade/api/v2/", want: "https://safe.trade/api/v2"},
		{raw: "https://safe.trade/api/v2//", want: "https://safe.trade/api/v2"},
		{raw: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{raw: "http://127.0.0.1:8080/", want: "http://127.0.0.1:8080"},
	}
	for _, tt := range tests {
		got, err := ParseBaseURL(tt.raw)
		if err != nil {
			t.Errorf("ParseBaseURL(%q) error = %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestParseBaseURLRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw     string
		wantErr string
	}{
		{raw: "", wantErr: "scheme must be http or https"},
		{raw: "safe.trade/api/v2", wantErr: "scheme must be http or https"},
		{raw: "wss://safe.trade/api/v2", wantErr: "scheme must be http or https"},
		{raw: "https:///api/v2", wantErr: "host is missing"},
		{raw: "https://safe.trade/api/v2?x=1", wantErr: "query and fragment are not allowed"},
		{raw: "https://safe.trade/api/v2?", wantErr: "query and fragment are not allowed"},
		{raw: "https://safe.trade/api/v2#top", wantErr: "query and fragment are not allowed"},
		{raw: "https://user:secret@safe.trade/api/v2", wantErr: "user info is not allowed"},
		{raw: "https://safe.trade/%zz", wantErr: "invalid base URL"},
	}
	for _, tt := range tests {
		_, err := ParseBaseURL(tt.raw)
		if err == nil {
			t.Errorf("ParseBaseURL(%q) succeeded, want error containing %q", tt.raw, tt.wantErr)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("ParseBaseURL(%q) error = %q, want it to contain %q", tt.raw, err, tt.wantErr)
		}
	}
}

func TestParseBaseURLDoesNotLeakCredentials(t *testing.T) {
	t.Parallel()

	_, err := ParseBaseURL("https://user:hunter2@safe.trade/%zz")
	if err == nil {
		t.Fatal("ParseBaseURL succeeded, want error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error %q leaks the password", err)
	}
}
