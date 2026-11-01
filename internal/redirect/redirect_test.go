package redirect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newRedirectingServer(t *testing.T) (start string, reached *atomic.Int32) {
	t.Helper()
	reached = &atomic.Int32{}
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Add(1)
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)
	return origin.URL + "/start", reached
}

func TestRestrictRefusesRedirectsOfMarkedRequests(t *testing.T) {
	t.Parallel()

	start, reached := newRedirectingServer(t)
	client := Restrict(&http.Client{}, func(req *http.Request) bool {
		return req.Header.Get("X-Auth-Apikey") != ""
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, start, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	req.Header.Set("X-Auth-Apikey", "key")
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("Do() followed the redirect of a request carrying credentials")
	}
	if !strings.Contains(err.Error(), "refusing to follow the redirect") {
		t.Errorf("Do() error = %v, want a refused redirect", err)
	}
	if got := reached.Load(); got != 0 {
		t.Errorf("the redirect target received %d requests, want 0", got)
	}
}

func TestRestrictFollowsOtherRedirects(t *testing.T) {
	t.Parallel()

	start, reached := newRedirectingServer(t)
	client := Restrict(&http.Client{}, func(*http.Request) bool { return false })

	resp, err := get(t, client, start)
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	_ = resp.Body.Close()
	if got := reached.Load(); got != 1 {
		t.Errorf("the redirect target received %d requests, want 1", got)
	}
}

func TestRestrictKeepsTheClientPolicy(t *testing.T) {
	t.Parallel()

	start, reached := newRedirectingServer(t)
	policyErr := errors.New("no redirects here")
	client := Restrict(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return policyErr },
	}, func(*http.Request) bool { return false })

	resp, err := get(t, client, start)
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, policyErr) {
		t.Errorf("get() error = %v, want the client's own policy error", err)
	}
	if got := reached.Load(); got != 0 {
		t.Errorf("the redirect target received %d requests, want 0", got)
	}
}

func TestRestrictLimitsRedirectLoops(t *testing.T) {
	t.Parallel()

	var loop *httptest.Server
	loop = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, loop.URL+"/again", http.StatusFound)
	}))
	t.Cleanup(loop.Close)
	client := Restrict(&http.Client{}, func(*http.Request) bool { return false })

	resp, err := get(t, client, loop.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("get() followed an endless redirect loop")
	}
	if !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Errorf("get() error = %v, want the redirect limit", err)
	}
}

func TestRestrictLeavesTheOriginalClientUnchanged(t *testing.T) {
	t.Parallel()

	original := &http.Client{}
	_ = Restrict(original, func(*http.Request) bool { return true })
	if original.CheckRedirect != nil {
		t.Error("Restrict modified the client it was given")
	}
}

func get(t *testing.T, client *http.Client, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	return client.Do(req)
}
