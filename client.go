package safetrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/anuketfluxus/safetrade-api-client/internal/endpoint"
	"github.com/anuketfluxus/safetrade-api-client/internal/redirect"
)

const DefaultBaseURL = "https://safe.trade/api/v2"

const Version = "1.0.0"

const DefaultUserAgent = "safetrade-go/" + Version

const (
	defaultTimeout = 30 * time.Second

	maxResponseSize = 32 << 20

	maxDrainSize = 64 << 10
)

type Client struct {
	Public *PublicService

	Account *AccountService

	Market *MarketService

	baseURL    string
	httpClient *http.Client
	auth       Authenticator
	userAgent  string
}

func NewClient(opts ...Option) (*Client, error) {
	cfg := config{
		baseURL:   DefaultBaseURL,
		userAgent: DefaultUserAgent,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.err != nil {
		return nil, cfg.err
	}

	baseURL, err := endpoint.ParseBaseURL(cfg.baseURL)
	if err != nil {
		return nil, fmt.Errorf("safetrade: %w", err)
	}
	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	c := &Client{
		baseURL:    baseURL,
		httpClient: redirect.Restrict(httpClient, carriesCredentials),
		auth:       cfg.auth,
		userAgent:  cfg.userAgent,
	}
	c.Public = &PublicService{client: c}
	c.Account = &AccountService{client: c}
	c.Market = &MarketService{client: c}
	return c, nil
}

type request struct {
	method string

	path  string
	query query

	body any

	private bool
}

func publicGet(path string, q query) request {
	return request{method: http.MethodGet, path: path, query: q}
}

func privateGet(path string, q query) request {
	return request{method: http.MethodGet, path: path, query: q, private: true}
}

func privatePost(path string, body any) request {
	if body == nil {
		body = struct{}{}
	}
	return request{method: http.MethodPost, path: path, body: body, private: true}
}

type credentialsKey struct{}

func carriesCredentials(req *http.Request) bool {
	return req.Context().Value(credentialsKey{}) != nil
}

func call[T any](ctx context.Context, c *Client, r request) (T, error) {
	var out T
	if err := c.do(ctx, r, &out); err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, r request, out any) error {
	operation := r.method + " " + r.path
	req, err := c.newHTTPRequest(ctx, r)
	if err != nil {
		return fmt.Errorf("safetrade: %s: %w", operation, &notSentError{err: err})
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("safetrade: %s: %w", operation, &notSentError{err: err})
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("safetrade: %s: %w", operation, err)
	}
	defer func() {

		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainSize))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("safetrade: %s: %w", operation, readAPIError(resp))
	}
	if out == nil {
		return nil
	}
	if err := decodeResponse(resp, out); err != nil {
		return fmt.Errorf("safetrade: %s: %w", operation, err)
	}
	return nil
}

func (c *Client) newHTTPRequest(ctx context.Context, r request) (*http.Request, error) {
	if r.private && c.auth == nil {
		return nil, ErrMissingCredentials
	}

	target := c.baseURL + r.path
	if len(r.query) > 0 {
		target += "?" + url.Values(r.query).Encode()
	}
	var body io.Reader
	if r.body != nil {
		payload, err := json.Marshal(r.body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		body = bytes.NewReader(payload)
	}
	if r.private {
		ctx = context.WithValue(ctx, credentialsKey{}, true)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return nil, err
	}

	req.GetBody = nil
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.private {
		if err := c.auth.Authenticate(req); err != nil {
			return nil, fmt.Errorf("authenticating request: %w", err)
		}
	}
	return req, nil
}

func decodeResponse(resp *http.Response, out any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err == nil && len(body) > maxResponseSize {
		err = fmt.Errorf("response body exceeds %d bytes", maxResponseSize)
	}
	if err == nil {
		err = decodeJSON(body, out)
	}
	if err != nil {
		if len(body) > maxErrorBodySize {
			body = body[:maxErrorBodySize]
		}
		return &DecodeError{StatusCode: resp.StatusCode, Body: body, Err: err}
	}
	return nil
}

func decodeJSON(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}
