package stream

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anuketfluxus/safetrade-api-client"
	"github.com/anuketfluxus/safetrade-api-client/internal/endpoint"
)

type Endpoint string

const (
	Public Endpoint = "public"

	Private Endpoint = "private"
)

const (
	defaultPingInterval      = 30 * time.Second
	defaultPongTimeout       = 15 * time.Second
	defaultHandshakeTimeout  = 30 * time.Second
	defaultReadLimit         = 8 << 20
	defaultBufferSize        = 256
	defaultMinReconnectDelay = time.Second
	defaultMaxReconnectDelay = 30 * time.Second

	writeTimeout = 10 * time.Second

	maxStableUptime = time.Minute
)

type Option func(*config)

type ReconnectPolicy struct {
	Disabled bool

	MinDelay time.Duration
	MaxDelay time.Duration

	MaxAttempts int
}

func (p ReconnectPolicy) stableUptime() time.Duration {
	return min(p.MaxDelay, maxStableUptime)
}

type config struct {
	baseURL          string
	httpClient       *http.Client
	auth             safetrade.Authenticator
	userAgent        string
	pingInterval     time.Duration
	pongTimeout      time.Duration
	handshakeTimeout time.Duration
	readLimit        int64
	bufferSize       int
	reconnect        ReconnectPolicy

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
			c.fail(errors.New("stream: WithHTTPClient: client is nil"))
			return
		}
		c.httpClient = httpClient
	}
}

func WithAuthenticator(auth safetrade.Authenticator) Option {
	return func(c *config) {
		if auth == nil {
			c.fail(errors.New("stream: WithAuthenticator: authenticator is nil"))
			return
		}
		c.auth = auth
	}
}

func WithAPIKey(apiKey, apiSecret string) Option {
	return func(c *config) {
		auth, err := safetrade.NewHMACAuth(apiKey, apiSecret)
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

func WithKeepAlive(interval, timeout time.Duration) Option {
	return func(c *config) {
		c.pingInterval = interval
		c.pongTimeout = timeout
	}
}

func WithHandshakeTimeout(timeout time.Duration) Option {
	return func(c *config) {
		c.handshakeTimeout = timeout
	}
}

func WithReadLimit(bytes int64) Option {
	return func(c *config) {
		c.readLimit = bytes
	}
}

func WithBufferSize(messages int) Option {
	return func(c *config) {
		c.bufferSize = messages
	}
}

func WithReconnect(policy ReconnectPolicy) Option {
	return func(c *config) {
		c.reconnect = policy
	}
}

func newConfig(ep Endpoint, opts []Option) (config, error) {
	cfg := config{
		baseURL:          safetrade.DefaultBaseURL,
		userAgent:        safetrade.DefaultUserAgent,
		pingInterval:     defaultPingInterval,
		pongTimeout:      defaultPongTimeout,
		handshakeTimeout: defaultHandshakeTimeout,
		readLimit:        defaultReadLimit,
		bufferSize:       defaultBufferSize,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.err != nil {
		return config{}, cfg.err
	}

	switch ep {
	case Public:
	case Private:
		if cfg.auth == nil {
			return config{}, errors.New("stream: the private endpoint requires credentials; use WithAPIKey or WithAuthenticator")
		}
	default:
		return config{}, fmt.Errorf("stream: unknown endpoint %q", ep)
	}

	baseURL, err := endpoint.ParseBaseURL(cfg.baseURL)
	if err != nil {
		return config{}, fmt.Errorf("stream: %w", err)
	}
	cfg.baseURL = baseURL

	if err := cfg.validateLimits(); err != nil {
		return config{}, err
	}
	if err := cfg.applyReconnectDefaults(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (c *config) validateLimits() error {
	switch {
	case c.pingInterval < 0:
		return errors.New("stream: keepalive interval must not be negative")
	case c.pingInterval > 0 && c.pongTimeout <= 0:
		return errors.New("stream: keepalive timeout must be positive")
	case c.handshakeTimeout <= 0:
		return errors.New("stream: handshake timeout must be positive")
	case c.readLimit <= 0:
		return errors.New("stream: read limit must be positive")
	case c.bufferSize < 0:
		return errors.New("stream: buffer size must not be negative")
	}
	return nil
}

func (c *config) applyReconnectDefaults() error {
	policy := &c.reconnect
	if policy.MinDelay < 0 || policy.MaxDelay < 0 || policy.MaxAttempts < 0 {
		return errors.New("stream: reconnect delays and attempts must not be negative")
	}
	if policy.MinDelay == 0 {
		policy.MinDelay = defaultMinReconnectDelay
	}
	if policy.MaxDelay == 0 {
		policy.MaxDelay = max(defaultMaxReconnectDelay, policy.MinDelay)
	}
	if policy.MinDelay > policy.MaxDelay {
		return fmt.Errorf("stream: reconnect MinDelay %v exceeds MaxDelay %v", policy.MinDelay, policy.MaxDelay)
	}
	return nil
}
