package safetrade

import (
	"errors"
	"net/http"
)

type Option func(*config)

type config struct {
	baseURL    string
	httpClient *http.Client
	auth       Authenticator
	userAgent  string

	err error
}

func (c *config) fail(err error) {
	if c.err == nil {
		c.err = err
	}
}

func WithBaseURL(rawURL string) Option {
	return func(c *config) {
		c.baseURL = rawURL
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *config) {
		if httpClient == nil {
			c.fail(errors.New("safetrade: WithHTTPClient: client is nil"))
			return
		}
		c.httpClient = httpClient
	}
}

func WithAuthenticator(auth Authenticator) Option {
	return func(c *config) {
		if auth == nil {
			c.fail(errors.New("safetrade: WithAuthenticator: authenticator is nil"))
			return
		}
		c.auth = auth
	}
}

func WithAPIKey(apiKey, apiSecret string) Option {
	return func(c *config) {
		auth, err := NewHMACAuth(apiKey, apiSecret)
		if err != nil {
			c.fail(err)
			return
		}
		c.auth = auth
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *config) {
		c.userAgent = userAgent
	}
}
