package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/anuketfluxus/safetrade-api-client"
	"github.com/anuketfluxus/safetrade-api-client/internal/redirect"
)

var ErrClosed = errors.New("stream: client closed")

type Client struct {
	cfg      config
	endpoint Endpoint
	url      string

	httpClient *http.Client

	mu sync.Mutex

	conn *websocket.Conn

	streams []string

	closed bool

	err error

	messages chan Message

	done chan struct{}

	stop context.CancelFunc

	closeOnce sync.Once
	closeErr  error
}

func Dial(ctx context.Context, ep Endpoint, opts ...Option) (*Client, error) {
	cfg, err := newConfig(ep, opts)
	if err != nil {
		return nil, err
	}
	c := &Client{
		cfg:        cfg,
		endpoint:   ep,
		url:        cfg.baseURL + "/websocket/" + string(ep),
		httpClient: cfg.httpClient,
		messages:   make(chan Message, cfg.bufferSize),
		done:       make(chan struct{}),
	}
	if ep == Private {
		base := cfg.httpClient
		if base == nil {
			base = http.DefaultClient
		}

		c.httpClient = redirect.Restrict(base, func(*http.Request) bool { return true })
	}

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	runCtx, stop := context.WithCancel(context.Background())
	c.stop = stop
	c.conn = conn
	go c.run(runCtx, conn)
	return c, nil
}

func (c *Client) Subscribe(ctx context.Context, streams ...string) error {
	requested, err := c.validateStreams(streams, true)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.stopErr(); err != nil {
		return err
	}
	var added []string
	for _, stream := range requested {
		if !slices.Contains(c.streams, stream) {
			added = append(added, stream)
		}
	}
	if len(added) == 0 {
		return nil
	}
	requests, err := encodeRequests(eventSubscribe, added)
	if err != nil {
		return err
	}
	c.streams = append(c.streams, added...)
	if c.conn == nil {
		return nil
	}
	return c.write(c.conn, eventSubscribe, requests)
}

func (c *Client) Unsubscribe(ctx context.Context, streams ...string) error {
	requested, err := c.validateStreams(streams, false)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.stopErr(); err != nil {
		return err
	}
	var removed []string
	for _, stream := range requested {
		if slices.Contains(c.streams, stream) {
			removed = append(removed, stream)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	requests, err := encodeRequests(eventUnsubscribe, removed)
	if err != nil {
		return err
	}
	c.streams = slices.DeleteFunc(c.streams, func(stream string) bool {
		return slices.Contains(removed, stream)
	})
	if c.conn == nil {
		return nil
	}
	return c.write(c.conn, eventUnsubscribe, requests)
}

func (c *Client) Subscriptions() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.streams)
}

func (c *Client) Read(ctx context.Context) (Message, error) {
	select {
	case msg := <-c.messages:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		select {
		case msg := <-c.messages:
			return msg, nil
		default:
			return nil, c.err
		}
	}
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		conn := c.conn
		c.conn = nil
		c.mu.Unlock()

		if conn != nil {
			if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil && !errors.Is(err, net.ErrClosed) {
				c.closeErr = fmt.Errorf("stream: closing connection: %w", err)
			}
		}
		c.stop()
		<-c.done
	})
	return c.closeErr
}

func (c *Client) validateStreams(streams []string, subscribing bool) ([]string, error) {
	unique := make([]string, 0, len(streams))
	for _, stream := range streams {
		switch {
		case stream == "":
			return nil, errors.New("stream: stream name must not be empty")
		case !fitsRequest(stream):
			return nil, fmt.Errorf("stream: stream name %q is too long for a %d-byte request", stream, maxRequestSize)
		case subscribing && isPrivate(stream) && c.endpoint == Public:
			return nil, fmt.Errorf("stream: %q is a private stream; dial the Private endpoint", stream)
		}
		if !slices.Contains(unique, stream) {
			unique = append(unique, stream)
		}
	}
	return unique, nil
}

func (c *Client) stopErr() error {
	if c.closed {
		return ErrClosed
	}
	return c.err
}

func (c *Client) finish(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.handshakeTimeout)
	defer cancel()

	header := http.Header{}
	header.Set("User-Agent", c.cfg.userAgent)
	if c.endpoint == Private {

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
		if err != nil {
			return nil, fmt.Errorf("stream: %w", err)
		}
		req.Header = header
		if err := c.cfg.auth.Authenticate(req); err != nil {
			return nil, fmt.Errorf("stream: authenticating handshake: %w", err)
		}
	}

	conn, resp, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{
		HTTPClient: c.httpClient,
		HTTPHeader: header,
	})
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, fmt.Errorf("stream: connecting to %s: %w", c.url, handshakeError(resp))
		}
		return nil, fmt.Errorf("stream: connecting to %s: %w", c.url, err)
	}
	conn.SetReadLimit(c.cfg.readLimit)
	return conn, nil
}

func handshakeError(resp *http.Response) *safetrade.APIError {
	var body []byte
	if resp.Body != nil {

		body, _ = io.ReadAll(resp.Body)
	}
	return &safetrade.APIError{StatusCode: resp.StatusCode, Body: body}
}

func (c *Client) run(ctx context.Context, conn *websocket.Conn) {
	defer close(c.done)
	policy := c.cfg.reconnect
	backoff := newBackoff(policy.MinDelay, policy.MaxDelay)
	for {
		connected := time.Now()
		cause := c.serve(ctx, conn)
		stopped := c.detachOrFinish(ctx, conn, cause, !policy.Disabled)
		_ = conn.CloseNow()
		if stopped {
			return
		}
		if time.Since(connected) >= policy.stableUptime() {

			backoff.reset()
		}

		outage := &notifier{queue: c.messages}
		outage.notify(Disconnected{Err: cause})
		next, attempts, err := c.reconnect(ctx, backoff, outage)
		if err != nil {

			c.finish(err)
			if !errors.Is(err, ErrClosed) {
				outage.flush(ctx)
			}
			return
		}

		outage.notify(Reconnected{Attempts: attempts})
		if !outage.flush(ctx) {
			c.detach(next)
			_ = next.CloseNow()
			c.finish(ErrClosed)
			return
		}
		conn = next
	}
}

func (c *Client) serve(ctx context.Context, conn *websocket.Conn) error {
	keepAliveCtx, stopKeepAlive := context.WithCancel(ctx)
	keepAliveDone := make(chan error, 1)
	go func() {
		keepAliveDone <- c.keepAlive(keepAliveCtx, conn)
	}()

	err := c.readLoop(ctx, conn)

	stopKeepAlive()
	if keepAliveErr := <-keepAliveDone; keepAliveErr != nil {

		return keepAliveErr
	}
	return err
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		frameType, frame, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var messages []Message
		if frameType == websocket.MessageText {
			messages = parseFrame(frame)
		} else {
			messages = []Message{UnknownFrame{Data: frame}}
		}
		for _, msg := range messages {
			if !c.deliver(ctx, msg) {
				return ctx.Err()
			}
		}
	}
}

func (c *Client) keepAlive(ctx context.Context, conn *websocket.Conn) error {
	if c.cfg.pingInterval <= 0 {
		return nil
	}
	ticker := time.NewTicker(c.cfg.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		pingCtx, cancel := context.WithTimeout(ctx, c.cfg.pongTimeout)
		err := conn.Ping(pingCtx)
		cancel()
		switch {
		case err == nil:
			continue
		case ctx.Err() != nil || !errors.Is(err, context.DeadlineExceeded):

			return nil
		default:
			_ = conn.CloseNow()
			return fmt.Errorf("stream: no pong within %v", c.cfg.pongTimeout)
		}
	}
}

func (c *Client) reconnect(ctx context.Context, backoff *backoff, outage *notifier) (*websocket.Conn, int, error) {
	maxAttempts := c.cfg.reconnect.MaxAttempts
	for attempt := 1; ; attempt++ {
		if !outage.sleep(ctx, backoff.next()) {
			return nil, attempt - 1, ErrClosed
		}
		conn, err := c.dial(ctx)
		if err == nil {
			if err = c.attach(conn); err == nil {
				return conn, attempt, nil
			}
			_ = conn.CloseNow()
		}

		switch {
		case ctx.Err() != nil || errors.Is(err, ErrClosed):
			return nil, attempt, ErrClosed
		case maxAttempts > 0 && attempt >= maxAttempts:
			return nil, attempt, fmt.Errorf("stream: giving up after %d reconnection attempts: %w", attempt, err)
		}
		outage.notify(Disconnected{Err: err, Attempts: attempt})
	}
}

func (c *Client) attach(conn *websocket.Conn) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}

	requests, err := encodeRequests(eventSubscribe, c.streams)
	if err != nil {
		return err
	}
	if err := c.write(conn, eventSubscribe, requests); err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) detach(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
}

func (c *Client) detachOrFinish(ctx context.Context, conn *websocket.Conn, cause error, reconnects bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
	switch {
	case ctx.Err() != nil || c.closed:
		c.err = ErrClosed
	case !reconnects:
		c.err = fmt.Errorf("stream: connection lost: %w", cause)
	default:
		return false
	}
	return true
}

func (c *Client) write(conn *websocket.Conn, event string, requests [][]byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	for _, request := range requests {
		if err := conn.Write(ctx, websocket.MessageText, request); err != nil {
			_ = conn.CloseNow()
			if c.conn == conn {

				c.conn = nil
			}
			return fmt.Errorf("stream: sending %s request: %w", event, err)
		}
	}
	return nil
}

func (c *Client) deliver(ctx context.Context, msg Message) bool {
	select {
	case c.messages <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

type notifier struct {
	queue   chan<- Message
	pending []Message
}

func (n *notifier) notify(msg Message) {
	n.offer()
	if last := len(n.pending) - 1; last >= 0 && isFailedAttempt(msg) && isFailedAttempt(n.pending[last]) {
		n.pending[last] = msg
	} else {
		n.pending = append(n.pending, msg)
	}
	n.offer()
}

func (n *notifier) offer() {
	for len(n.pending) > 0 {
		select {
		case n.queue <- n.pending[0]:
			n.pending = n.pending[1:]
		default:
			return
		}
	}
}

func (n *notifier) sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {

		var queue chan<- Message
		var next Message
		if len(n.pending) > 0 {
			queue, next = n.queue, n.pending[0]
		}
		select {
		case queue <- next:
			n.pending = n.pending[1:]
		case <-timer.C:
			n.offer()
			return true
		case <-ctx.Done():
			return false
		}
	}
}

func (n *notifier) flush(ctx context.Context) bool {
	for len(n.pending) > 0 {
		select {
		case n.queue <- n.pending[0]:
			n.pending = n.pending[1:]
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func isFailedAttempt(msg Message) bool {
	disconnected, ok := msg.(Disconnected)
	return ok && disconnected.Attempts > 0
}

type backoff struct {
	min, max time.Duration
	delay    time.Duration
}

func newBackoff(minDelay, maxDelay time.Duration) *backoff {
	return &backoff{min: minDelay, max: maxDelay, delay: minDelay}
}

func (b *backoff) next() time.Duration {
	wait := jitter(b.delay)
	if b.delay > b.max/2 {
		b.delay = b.max
	} else {
		b.delay *= 2
	}
	return wait
}

func (b *backoff) reset() {
	b.delay = b.min
}

func jitter(d time.Duration) time.Duration {
	half := d / 2
	return half + rand.N(d-half+1)
}
