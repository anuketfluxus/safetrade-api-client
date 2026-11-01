package safetrade

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHMACAuthMatchesReferenceSignatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		nonce  int64
		key    string
		secret string
		want   string
	}{
		{
			nonce:  1700000000000,
			key:    "my-api-key",
			secret: "my-api-secret",
			want:   "064c743e919261d1f7ec94c4825dd613c665c95ad5f857aab026d114e5de2bd5",
		},
		{
			nonce:  1691239429000,
			key:    "4b1c7f0e2f3d4a5b",
			secret: "c0ffee-secret-with-unicode-é",
			want:   "cb7179370db90bd431703ecbc442638a3e190a8bc3c2d22adbba02a38e70233e",
		},
	}
	for _, tt := range tests {
		auth, err := NewHMACAuth(tt.key, tt.secret)
		if err != nil {
			t.Fatalf("NewHMACAuth() error = %v", err)
		}
		auth.now = func() time.Time { return time.UnixMilli(tt.nonce) }

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if err := auth.Authenticate(req); err != nil {
			t.Fatalf("Authenticate() error = %v", err)
		}

		if got := req.Header.Get("X-Auth-Apikey"); got != tt.key {
			t.Errorf("X-Auth-Apikey = %q, want %q", got, tt.key)
		}
		if got, want := req.Header.Get("X-Auth-Nonce"), strconv.FormatInt(tt.nonce, 10); got != want {
			t.Errorf("X-Auth-Nonce = %q, want %q", got, want)
		}
		if got := req.Header.Get("X-Auth-Signature"); got != tt.want {
			t.Errorf("X-Auth-Signature = %q, want %q", got, tt.want)
		}
	}
}

func TestHMACAuthNoncesStrictlyIncrease(t *testing.T) {
	t.Parallel()

	auth, err := NewHMACAuth("key", "secret")
	if err != nil {
		t.Fatalf("NewHMACAuth() error = %v", err)
	}
	clock := time.UnixMilli(1_700_000_000_000)
	auth.now = func() time.Time { return clock }

	steps := []struct {
		advance time.Duration
		want    int64
	}{
		{advance: 0, want: 1_700_000_000_000},
		{advance: 0, want: 1_700_000_000_001},
		{advance: 0, want: 1_700_000_000_002},
		{advance: 10 * time.Millisecond, want: 1_700_000_000_010},
		{advance: -time.Second, want: 1_700_000_000_011},
	}
	for i, step := range steps {
		clock = clock.Add(step.advance)
		if got := auth.nextNonce(); got != step.want {
			t.Errorf("step %d: nextNonce() = %d, want %d", i, got, step.want)
		}
	}
}

func TestHMACAuthNoncesAreUniqueUnderConcurrency(t *testing.T) {
	t.Parallel()

	auth, err := NewHMACAuth("key", "secret")
	if err != nil {
		t.Fatalf("NewHMACAuth() error = %v", err)
	}
	const goroutines, perGoroutine = 32, 200
	nonces := make([][]int64, goroutines)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				if err := auth.Authenticate(req); err != nil {
					t.Errorf("Authenticate() error = %v", err)
					return
				}
				nonce, err := strconv.ParseInt(req.Header.Get("X-Auth-Nonce"), 10, 64)
				if err != nil {
					t.Errorf("X-Auth-Nonce is not an integer: %v", err)
					return
				}
				nonces[g] = append(nonces[g], nonce)
			}
		}()
	}
	wg.Wait()

	seen := make(map[int64]bool, goroutines*perGoroutine)
	for _, batch := range nonces {
		for _, nonce := range batch {
			if seen[nonce] {
				t.Fatalf("nonce %d was issued twice", nonce)
			}
			seen[nonce] = true
		}
	}
}

func TestNewAuthenticatorsRejectEmptyCredentials(t *testing.T) {
	t.Parallel()

	constructors := map[string]func(key, secret string) error{
		"NewHMACAuth": func(key, secret string) error {
			_, err := NewHMACAuth(key, secret)
			return err
		},
		"NewBasicAuth": func(key, secret string) error {
			_, err := NewBasicAuth(key, secret)
			return err
		},
	}
	for name, construct := range constructors {
		for _, creds := range [][2]string{{"", "secret"}, {"key", ""}, {"", ""}} {
			err := construct(creds[0], creds[1])
			if !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("%s(%q, %q) error = %v, want ErrInvalidArgument", name, creds[0], creds[1], err)
			}
		}
	}
}

func TestBasicAuthSetsAuthorizationHeader(t *testing.T) {
	t.Parallel()

	auth, err := NewBasicAuth("my-key", "my-secret")
	if err != nil {
		t.Fatalf("NewBasicAuth() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := auth.Authenticate(req); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	user, password, ok := req.BasicAuth()
	if !ok || user != "my-key" || password != "my-secret" {
		t.Errorf("BasicAuth() = %q, %q, %v; want my-key, my-secret, true", user, password, ok)
	}
}

func TestAuthenticatorsDoNotRevealSecrets(t *testing.T) {
	t.Parallel()

	const secret = "super-secret-value"
	hmacAuth, err := NewHMACAuth("visible-key", secret)
	if err != nil {
		t.Fatalf("NewHMACAuth() error = %v", err)
	}
	basicAuth, err := NewBasicAuth("visible-key", secret)
	if err != nil {
		t.Fatalf("NewBasicAuth() error = %v", err)
	}

	values := map[string]any{
		"*HMACAuth":  hmacAuth,
		"*BasicAuth": basicAuth,
		"BasicAuth":  *basicAuth,
	}
	for name, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			text := fmt.Sprintf(verb, value)
			if strings.Contains(text, secret) {
				t.Errorf("fmt %s of %s = %q reveals the secret", verb, name, text)
			}
			if !strings.Contains(text, "visible-key") {
				t.Errorf("fmt %s of %s = %q, want it to name the key", verb, name, text)
			}
		}
	}
}
