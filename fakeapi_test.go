package safetrade_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/anuketfluxus/safetrade-api-client"
)

const (
	testAPIKey    = "test-api-key"
	testAPISecret = "test-api-secret"
)

type recordedRequest struct {
	method string

	path   string
	query  url.Values
	header http.Header
	body   []byte
}

type fakeAPI struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
	status   int
	body     string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{status: http.StatusOK, body: "{}"}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{
		method: r.Method,
		path:   r.URL.EscapedPath(),
		query:  r.URL.Query(),
		header: r.Header.Clone(),
		body:   body,
	})
	status, responseBody := f.status, f.body
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, responseBody)
}

func (f *fakeAPI) respond(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *fakeAPI) baseURL() string {
	return f.server.URL + "/api/v2"
}

func (f *fakeAPI) newClient(t *testing.T, opts ...safetrade.Option) *safetrade.Client {
	t.Helper()
	defaults := []safetrade.Option{
		safetrade.WithBaseURL(f.baseURL()),
		safetrade.WithAPIKey(testAPIKey, testAPISecret),
	}
	client, err := safetrade.NewClient(append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func (f *fakeAPI) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeAPI) lastRequest(t *testing.T) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("the fake API received no request")
	}
	return f.requests[len(f.requests)-1]
}

func verifyHMACHeaders(t *testing.T, header http.Header) {
	t.Helper()
	if got := header.Get("X-Auth-Apikey"); got != testAPIKey {
		t.Errorf("X-Auth-Apikey = %q, want %q", got, testAPIKey)
	}
	nonce := header.Get("X-Auth-Nonce")
	if nonce == "" {
		t.Fatal("X-Auth-Nonce is missing")
	}
	mac := hmac.New(sha256.New, []byte(testAPISecret))
	mac.Write([]byte(nonce + testAPIKey))
	if got, want := header.Get("X-Auth-Signature"), hex.EncodeToString(mac.Sum(nil)); got != want {
		t.Errorf("X-Auth-Signature = %q, want %q", got, want)
	}
}

func verifyNoAuthHeaders(t *testing.T, header http.Header) {
	t.Helper()
	for _, name := range []string{"X-Auth-Apikey", "X-Auth-Nonce", "X-Auth-Signature", "Authorization"} {
		if value := header.Get(name); value != "" {
			t.Errorf("public request carries %s: %q", name, value)
		}
	}
}
