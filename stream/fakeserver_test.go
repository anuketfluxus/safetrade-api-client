package stream

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const (
	testAPIKey    = "stream-test-key"
	testAPISecret = "stream-test-secret"

	waitTimeout = 5 * time.Second

	rangoMaxMessageSize = 512
)

type fakeServer struct {
	t      *testing.T
	server *httptest.Server

	mu    sync.Mutex
	conns []*serverConn

	handshakes        []http.Header
	handshakeArrivals []time.Time

	rejectStatus int

	unresponsive bool

	dropOnAccept bool

	connected chan *serverConn
}

type serverConn struct {
	conn     *websocket.Conn
	endpoint string

	mu       sync.Mutex
	streams  []string
	requests []streamRequest

	closed   chan struct{}
	closeErr error

	dropped  chan struct{}
	dropOnce sync.Once
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	s := &fakeServer{t: t, connected: make(chan *serverConn, 100)}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(func() {
		s.dropAll()
		s.server.Close()
	})
	return s
}

func (s *fakeServer) baseURL() string {
	return s.server.URL + "/api/v2"
}

func (s *fakeServer) handle(w http.ResponseWriter, r *http.Request) {
	endpoint, ok := strings.CutPrefix(r.URL.Path, "/api/v2/websocket/")
	if !ok || (endpoint != "public" && endpoint != "private") {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	s.handshakes = append(s.handshakes, r.Header.Clone())
	s.handshakeArrivals = append(s.handshakeArrivals, time.Now())
	rejectStatus, unresponsive, dropOnAccept := s.rejectStatus, s.unresponsive, s.dropOnAccept
	s.mu.Unlock()

	if rejectStatus != 0 {
		writeJSONError(w, rejectStatus, "server.rejected")
		return
	}
	if endpoint == "private" && !validCredentials(r.Header) {
		writeJSONError(w, http.StatusUnauthorized, "authz.invalid_signature")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.t.Errorf("fake server: accept: %v", err)
		return
	}
	conn.SetReadLimit(rangoMaxMessageSize)
	sc := &serverConn{
		conn:     conn,
		endpoint: endpoint,
		closed:   make(chan struct{}),
		dropped:  make(chan struct{}),
	}
	s.mu.Lock()
	s.conns = append(s.conns, sc)
	s.mu.Unlock()
	s.connected <- sc

	if dropOnAccept {
		sc.drop()
		close(sc.closed)
		return
	}

	sc.send(`{"success":{"message":"subscribed","streams":[]}}`)

	if unresponsive {

		<-sc.dropped
		return
	}
	sc.serve()
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"errors":[%q]}`, code)
}

func validCredentials(header http.Header) bool {
	if validSignature(header) {
		return true
	}
	req := &http.Request{Header: header}
	user, password, ok := req.BasicAuth()
	return ok && user == testAPIKey && password == testAPISecret
}

func validSignature(header http.Header) bool {
	if header.Get("X-Auth-Apikey") != testAPIKey {
		return false
	}
	mac := hmac.New(sha256.New, []byte(testAPISecret))
	mac.Write([]byte(header.Get("X-Auth-Nonce") + testAPIKey))
	return hmac.Equal([]byte(header.Get("X-Auth-Signature")), []byte(hex.EncodeToString(mac.Sum(nil))))
}

func (sc *serverConn) serve() {
	defer close(sc.closed)
	for {
		_, data, err := sc.conn.Read(context.Background())
		if err != nil {
			sc.closeErr = err
			return
		}
		if string(data) == "ping" {
			sc.send("pong")
			continue
		}

		var req streamRequest
		if err := json.Unmarshal(data, &req); err != nil || (req.Event != eventSubscribe && req.Event != eventUnsubscribe) {
			sc.send(`{"error":"Could not parse Type: Invalid event"}`)
			continue
		}
		sc.mu.Lock()
		sc.requests = append(sc.requests, req)
		for _, stream := range req.Streams {
			if req.Event == eventSubscribe {

				if !slices.Contains(sc.streams, stream) && (sc.endpoint == "private" || !isPrivate(stream)) {
					sc.streams = append(sc.streams, stream)
				}
			} else {
				sc.streams = slices.DeleteFunc(sc.streams, func(s string) bool { return s == stream })
			}
		}
		reply, _ := json.Marshal(map[string]any{
			"success": map[string]any{"message": req.Event + "d", "streams": append([]string{}, sc.streams...)},
		})
		sc.mu.Unlock()
		sc.send(string(reply))
	}
}

func (sc *serverConn) send(frame string) {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	_ = sc.conn.Write(ctx, websocket.MessageText, []byte(frame))
}

func (sc *serverConn) sendBinary(data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	_ = sc.conn.Write(ctx, websocket.MessageBinary, data)
}

func (sc *serverConn) publish(t *testing.T, stream, payload string) {
	t.Helper()
	sc.mu.Lock()
	subscribed := slices.Contains(sc.streams, stream)
	sc.mu.Unlock()
	if !subscribed {
		t.Fatalf("publishing on %q, to which the connection is not subscribed", stream)
	}
	sc.send(fmt.Sprintf(`{%q:%s}`, stream, payload))
}

func (sc *serverConn) subscriptions() []string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return slices.Clone(sc.streams)
}

func (sc *serverConn) receivedRequests() []streamRequest {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return slices.Clone(sc.requests)
}

func (sc *serverConn) drop() {
	sc.dropOnce.Do(func() {
		close(sc.dropped)
		_ = sc.conn.CloseNow()
	})
}

func (s *fakeServer) dropAll() {
	s.mu.Lock()
	conns := slices.Clone(s.conns)
	s.mu.Unlock()
	for _, sc := range conns {
		sc.drop()
	}
}

func (s *fakeServer) setRejectStatus(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectStatus = status
}

func (s *fakeServer) setDropOnAccept(drop bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropOnAccept = drop
}

func (s *fakeServer) setUnresponsive(unresponsive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unresponsive = unresponsive
}

func (s *fakeServer) handshakeHeaders() []http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.handshakes)
}

func (s *fakeServer) handshakeTimes() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.handshakeArrivals)
}

func (s *fakeServer) nextConn(t *testing.T) *serverConn {
	t.Helper()
	select {
	case sc := <-s.connected:
		return sc
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for a connection")
		return nil
	}
}

func eventually(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func dialTest(t *testing.T, s *fakeServer, ep Endpoint, opts ...Option) *Client {
	t.Helper()
	defaults := []Option{
		WithBaseURL(s.baseURL()),
		WithAPIKey(testAPIKey, testAPISecret),
		WithReconnect(ReconnectPolicy{MinDelay: 5 * time.Millisecond, MaxDelay: 20 * time.Millisecond}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	client, err := Dial(ctx, ep, append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func readMessage(t *testing.T, c *Client) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	return msg
}

func readUntil[M Message](t *testing.T, c *Client, match func(M) bool) M {
	t.Helper()
	for {
		if msg, ok := readMessage(t, c).(M); ok && match(msg) {
			return msg
		}
	}
}

func readUpdate(t *testing.T, c *Client, stream string) Update {
	t.Helper()
	return readUntil(t, c, func(u Update) bool { return u.Stream == stream })
}

func readSubscriptions(t *testing.T, c *Client, streams ...string) {
	t.Helper()
	readUntil(t, c, func(s Subscriptions) bool { return slices.Equal(s.Streams, streams) })
}
