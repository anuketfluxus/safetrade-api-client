package stream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/anuketfluxus/safetrade-api-client"
)

func TestDialPublic(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)

	if sc.endpoint != "public" {
		t.Errorf("connected to %q, want public", sc.endpoint)
	}
	ack, ok := readMessage(t, client).(Subscriptions)
	if !ok || ack.Message != "subscribed" || len(ack.Streams) != 0 {
		t.Errorf("first message = %#v, want the empty acknowledgement sent on connect", ack)
	}
	header := server.handshakeHeaders()[0]
	if got := header.Get("User-Agent"); got != safetrade.DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, safetrade.DefaultUserAgent)
	}
}

func TestDialPrivateAuthenticatesTheHandshake(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	dialTest(t, server, Private, WithUserAgent("my-bot/1.0"))
	sc := server.nextConn(t)

	if sc.endpoint != "private" {
		t.Errorf("connected to %q, want private", sc.endpoint)
	}
	header := server.handshakeHeaders()[0]
	if !validSignature(header) {
		t.Errorf("handshake headers %v carry no valid signature", header)
	}
	if got := header.Get("User-Agent"); got != "my-bot/1.0" {
		t.Errorf("User-Agent = %q, want my-bot/1.0", got)
	}
}

func TestDialRejectedHandshake(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	_, err := Dial(context.Background(), Private,
		WithBaseURL(server.baseURL()),
		WithAPIKey(testAPIKey, "wrong-secret"),
	)
	if !errors.Is(err, safetrade.ErrUnauthorized) {
		t.Fatalf("Dial() error = %v, want it to match safetrade.ErrUnauthorized", err)
	}
	var apiErr *safetrade.APIError
	if !errors.As(err, &apiErr) || !slices.Equal(apiErr.Messages(), []string{"authz.invalid_signature"}) {
		t.Errorf("Dial() error = %v, want an *APIError with the server's message", err)
	}
}

func TestDialValidatesConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint Endpoint
		opts     []Option
		wantErr  string
	}{
		{name: "private without credentials", endpoint: Private, wantErr: "requires credentials"},
		{name: "unknown endpoint", endpoint: "admin", wantErr: "unknown endpoint"},
		{name: "invalid base URL", endpoint: Public, opts: []Option{WithBaseURL("ftp://safe.trade")}, wantErr: "scheme must be http or https"},
		{name: "nil HTTP client", endpoint: Public, opts: []Option{WithHTTPClient(nil)}, wantErr: "client is nil"},
		{name: "nil authenticator", endpoint: Public, opts: []Option{WithAuthenticator(nil)}, wantErr: "authenticator is nil"},
		{name: "empty API key", endpoint: Public, opts: []Option{WithAPIKey("", "secret")}, wantErr: "must not be empty"},
		{name: "negative keepalive", endpoint: Public, opts: []Option{WithKeepAlive(-time.Second, time.Second)}, wantErr: "keepalive interval"},
		{name: "zero pong timeout", endpoint: Public, opts: []Option{WithKeepAlive(time.Second, 0)}, wantErr: "keepalive timeout"},
		{name: "zero handshake timeout", endpoint: Public, opts: []Option{WithHandshakeTimeout(0)}, wantErr: "handshake timeout"},
		{name: "zero read limit", endpoint: Public, opts: []Option{WithReadLimit(0)}, wantErr: "read limit"},
		{name: "negative buffer", endpoint: Public, opts: []Option{WithBufferSize(-1)}, wantErr: "buffer size"},
		{
			name:     "negative reconnect attempts",
			endpoint: Public,
			opts:     []Option{WithReconnect(ReconnectPolicy{MaxAttempts: -1})},
			wantErr:  "must not be negative",
		},
		{
			name:     "inverted reconnect delays",
			endpoint: Public,
			opts:     []Option{WithReconnect(ReconnectPolicy{MinDelay: time.Minute, MaxDelay: time.Second})},
			wantErr:  "exceeds MaxDelay",
		},
	}
	for _, tt := range tests {
		client, err := Dial(context.Background(), tt.endpoint, tt.opts...)
		if err == nil {
			_ = client.Close()
			t.Errorf("%s: Dial() succeeded, want error", tt.name)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%s: Dial() error = %q, want it to contain %q", tt.name, err, tt.wantErr)
		}
	}
}

func TestReconnectDefaultsKeepCustomMinDelay(t *testing.T) {
	t.Parallel()

	cfg, err := newConfig(Public, []Option{WithReconnect(ReconnectPolicy{MinDelay: time.Minute})})
	if err != nil {
		t.Fatalf("newConfig() error = %v", err)
	}
	if cfg.reconnect.MinDelay != time.Minute || cfg.reconnect.MaxDelay != time.Minute {
		t.Errorf("delays = %v and %v, want a minute each", cfg.reconnect.MinDelay, cfg.reconnect.MaxDelay)
	}
}

func TestSubscribeAndReceiveUpdates(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	ctx := context.Background()

	if err := client.Subscribe(ctx, GlobalTickers, MarketTrades("btcusdt")); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, GlobalTickers, "btcusdt.trades")
	if got := sc.receivedRequests(); len(got) != 1 || got[0].Event != "subscribe" ||
		!slices.Equal(got[0].Streams, []string{GlobalTickers, "btcusdt.trades"}) {
		t.Errorf("server received %+v, want one subscribe request for both streams", got)
	}

	sc.publish(t, GlobalTickers, `{"btcusdt":{"id":"btcusdt","last":"67050.25","price_change_percent":"+1.59%"}}`)
	tickers, err := readUpdate(t, client, GlobalTickers).Tickers()
	if err != nil {
		t.Fatalf("Tickers() error = %v", err)
	}
	if got := tickers["btcusdt"].Last.String(); got != "67050.25" {
		t.Errorf("btcusdt last = %s, want 67050.25", got)
	}

	sc.publish(t, "btcusdt.trades", `{"trades":[{"id":1,"price":"67000","amount":"0.5","side":"sell"}]}`)
	update := readUpdate(t, client, "btcusdt.trades")
	if update.Market() != "btcusdt" {
		t.Errorf("Market() = %q, want btcusdt", update.Market())
	}
	trades, err := update.MarketTrades()
	if err != nil {
		t.Fatalf("MarketTrades() error = %v", err)
	}
	if len(trades) != 1 || trades[0].Side != safetrade.SideSell || trades[0].Amount.String() != "0.5" {
		t.Errorf("trades = %+v, want one sell of 0.5", trades)
	}
}

func TestPrivateStreams(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Private)
	sc := server.nextConn(t)

	if err := client.Subscribe(context.Background(), Orders, Trades, Balances); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, Orders, Trades, Balances)

	sc.publish(t, Orders, `{"id":42,"market":"btcusdt","side":"buy","state":"wait","price":"67000","origin_amount":"1"}`)
	order, err := readUpdate(t, client, Orders).Order()
	if err != nil {
		t.Fatalf("Order() error = %v", err)
	}
	if order.ID != 42 || order.State != safetrade.OrderStateWait {
		t.Errorf("order = %+v, want order 42 waiting", order)
	}

	sc.publish(t, Balances, `[{"currency":"usdt","balance":"100","locked":"5"},{"currency":"btc","balance":"0.1","locked":"0"}]`)
	balances, err := readUpdate(t, client, Balances).Balances()
	if err != nil {
		t.Fatalf("Balances() error = %v", err)
	}
	if len(balances) != 2 || balances[1].Currency != "btc" {
		t.Errorf("balances = %+v, want usdt and btc", balances)
	}
}

func TestSubscribeRejectsPrivateStreamsOnThePublicEndpoint(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)

	err := client.Subscribe(context.Background(), GlobalTickers, Orders)
	if err == nil || !strings.Contains(err.Error(), "private stream") {
		t.Fatalf("Subscribe() error = %v, want a private stream error", err)
	}
	if got := client.Subscriptions(); len(got) != 0 {
		t.Errorf("Subscriptions() = %v, want none after a rejected call", got)
	}
}

func TestSubscribeValidatesNames(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)

	if err := client.Subscribe(context.Background(), ""); err == nil {
		t.Error("Subscribe(\"\") succeeded, want error")
	}
	if err := client.Unsubscribe(context.Background(), ""); err == nil {
		t.Error("Unsubscribe(\"\") succeeded, want error")
	}
}

func TestSubscribeSendsOnlyNewStreams(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	ctx := context.Background()

	if err := client.Subscribe(ctx, MarketDepth("btcusdt"), MarketDepth("btcusdt")); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := client.Subscribe(ctx, MarketDepth("btcusdt")); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := client.Subscribe(ctx, MarketDepth("btcusdt"), MarketDepth("ethusdt")); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, "btcusdt.depth", "ethusdt.depth")

	want := []streamRequest{
		{Event: "subscribe", Streams: []string{"btcusdt.depth"}},
		{Event: "subscribe", Streams: []string{"ethusdt.depth"}},
	}
	if got := sc.receivedRequests(); !equalRequests(got, want) {
		t.Errorf("server received %+v, want %+v", got, want)
	}
	if got := client.Subscriptions(); !slices.Equal(got, []string{"btcusdt.depth", "ethusdt.depth"}) {
		t.Errorf("Subscriptions() = %v", got)
	}
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	ctx := context.Background()

	if err := client.Subscribe(ctx, GlobalTickers, MarketTrades("btcusdt")); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := client.Unsubscribe(ctx, GlobalTickers, MarketTrades("not-subscribed")); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
	readUntil(t, client, func(s Subscriptions) bool { return s.Message == "unsubscribed" })

	want := []streamRequest{
		{Event: "subscribe", Streams: []string{GlobalTickers, "btcusdt.trades"}},
		{Event: "unsubscribe", Streams: []string{GlobalTickers}},
	}
	if got := sc.receivedRequests(); !equalRequests(got, want) {
		t.Errorf("server received %+v, want %+v", got, want)
	}
	if got := client.Subscriptions(); !slices.Equal(got, []string{"btcusdt.trades"}) {
		t.Errorf("Subscriptions() = %v, want [btcusdt.trades]", got)
	}
	if got := sc.subscriptions(); !slices.Equal(got, []string{"btcusdt.trades"}) {
		t.Errorf("server subscriptions = %v, want [btcusdt.trades]", got)
	}
}

func TestSubscribeSplitsRequestsAtTheServerLimit(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)

	streams := make([]string, 80)
	for i := range streams {
		streams[i] = MarketDepth(fmt.Sprintf("market%02dusdt", i))
	}
	if err := client.Subscribe(context.Background(), streams...); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	readSubscriptions(t, client, streams...)
	if got := len(sc.receivedRequests()); got < 2 {
		t.Errorf("server received %d requests, want several", got)
	}
}

func TestFramesWithSeveralMembers(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	readMessage(t, client)

	sc.send(`{"btcusdt.trades":[{"id":1}],"ethusdt.trades":[{"id":2}],"success":{"message":"subscribed","streams":["x.trades"]}}`)

	first, _ := readMessage(t, client).(Update)
	second, _ := readMessage(t, client).(Update)
	third, _ := readMessage(t, client).(Subscriptions)
	if first.Stream != "btcusdt.trades" || second.Stream != "ethusdt.trades" {
		t.Errorf("updates on %q and %q, want btcusdt.trades then ethusdt.trades", first.Stream, second.Stream)
	}
	if !slices.Equal(third.Streams, []string{"x.trades"}) {
		t.Errorf("third message = %#v, want the acknowledgement", third)
	}
}

func TestServerErrorsAndUnknownFrames(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	readMessage(t, client)

	sc.send(`{"error":"Could not parse Type: Invalid event"}`)
	sc.send(`{"error":{"message":"legacy format"}}`)
	sc.send(`pong`)
	sc.send(`not json`)
	sc.sendBinary([]byte{0x01, 0x02})

	if got, ok := readMessage(t, client).(ServerError); !ok || got.Message != "Could not parse Type: Invalid event" {
		t.Errorf("message = %#v, want the server error", got)
	}
	if got, ok := readMessage(t, client).(ServerError); !ok || got.Message != "legacy format" {
		t.Errorf("message = %#v, want the legacy server error", got)
	}

	if got, ok := readMessage(t, client).(UnknownFrame); !ok || string(got.Data) != "not json" {
		t.Errorf("message = %#v, want the text frame as UnknownFrame", got)
	}
	if got, ok := readMessage(t, client).(UnknownFrame); !ok || !slices.Equal(got.Data, []byte{0x01, 0x02}) {
		t.Errorf("message = %#v, want the binary frame as UnknownFrame", got)
	}
}

func TestReconnectRenewsSubscriptions(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Private)
	first := server.nextConn(t)
	ctx := context.Background()

	if err := client.Subscribe(ctx, Orders, GlobalTickers); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, Orders, GlobalTickers)

	first.drop()
	disconnected := readUntil(t, client, func(Disconnected) bool { return true })
	if disconnected.Err == nil || disconnected.Attempts != 0 {
		t.Errorf("Disconnected = %+v, want the cause of the loss and no attempt yet", disconnected)
	}
	second := server.nextConn(t)
	reconnected := readUntil(t, client, func(Reconnected) bool { return true })
	if reconnected.Attempts != 1 {
		t.Errorf("Reconnected = %+v, want one attempt", reconnected)
	}
	readSubscriptions(t, client, Orders, GlobalTickers)
	if got := second.receivedRequests(); len(got) != 1 || !slices.Equal(got[0].Streams, []string{Orders, GlobalTickers}) {
		t.Errorf("new connection received %+v, want the renewed subscriptions", got)
	}

	headers := server.handshakeHeaders()
	if len(headers) != 2 || !validSignature(headers[1]) ||
		headers[0].Get("X-Auth-Nonce") == headers[1].Get("X-Auth-Nonce") {
		t.Errorf("handshakes %v, want two signed with different nonces", headers)
	}

	second.publish(t, Orders, `{"id":7,"market":"btcusdt","side":"sell","state":"wait"}`)
	if order, err := readUpdate(t, client, Orders).Order(); err != nil || order.ID != 7 {
		t.Errorf("Order() = %+v, %v; want order 7", order, err)
	}
}

func TestSubscriptionsChangedWhileDisconnectedApplyOnReconnect(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	first := server.nextConn(t)
	ctx := context.Background()

	if err := client.Subscribe(ctx, GlobalTickers, MarketDepth("btcusdt")); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, GlobalTickers, "btcusdt.depth")

	server.setRejectStatus(http.StatusServiceUnavailable)
	first.drop()
	eventually(t, "the client retries", func() bool { return len(server.handshakeHeaders()) >= 2 })
	if err := client.Subscribe(ctx, MarketTrades("btcusdt")); err != nil {
		t.Errorf("Subscribe() while disconnected error = %v", err)
	}
	if err := client.Unsubscribe(ctx, GlobalTickers); err != nil {
		t.Errorf("Unsubscribe() while disconnected error = %v", err)
	}

	server.setRejectStatus(0)
	second := server.nextConn(t)
	readUntil(t, client, func(Reconnected) bool { return true })
	readSubscriptions(t, client, "btcusdt.depth", "btcusdt.trades")
	if got := second.subscriptions(); !slices.Equal(got, []string{"btcusdt.depth", "btcusdt.trades"}) {
		t.Errorf("server subscriptions = %v, want [btcusdt.depth btcusdt.trades]", got)
	}
}

func TestReconnectRetriesRejectedHandshakes(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	first := server.nextConn(t)

	server.setRejectStatus(http.StatusForbidden)
	first.drop()
	failed := readUntil(t, client, func(d Disconnected) bool { return d.Attempts == 2 })
	if !errors.Is(failed.Err, safetrade.ErrForbidden) {
		t.Errorf("Disconnected.Err = %v, want it to match safetrade.ErrForbidden", failed.Err)
	}

	server.setRejectStatus(0)
	reconnected := readUntil(t, client, func(Reconnected) bool { return true })
	if reconnected.Attempts < 3 {
		t.Errorf("Reconnected.Attempts = %d, want at least 3", reconnected.Attempts)
	}
}

func TestReconnectGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithReconnect(ReconnectPolicy{
		MinDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, MaxAttempts: 3,
	}))
	first := server.nextConn(t)

	server.setRejectStatus(http.StatusServiceUnavailable)
	first.drop()

	var attempts []int
	err := readUntilError(t, client, func(msg Message) {
		if disconnected, ok := msg.(Disconnected); ok {
			attempts = append(attempts, disconnected.Attempts)
		}
	})
	if !errors.Is(err, safetrade.ErrServerError) || !strings.Contains(err.Error(), "giving up after 3 reconnection attempts") {
		t.Errorf("Read() error = %v, want to give up after 3 attempts with a server error", err)
	}
	if want := []int{0, 1, 2}; !slices.Equal(attempts, want) {
		t.Errorf("Disconnected attempts = %v, want %v: the loss, then each failure before the last", attempts, want)
	}
	if got := len(server.handshakeHeaders()); got != 4 {
		t.Errorf("server saw %d handshakes, want 1 + 3 attempts", got)
	}
}

func TestReconnectingDoesNotWaitForRead(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithBufferSize(1))
	first := server.nextConn(t)

	eventually(t, "the queue is full", func() bool { return len(client.messages) == cap(client.messages) })

	server.setRejectStatus(http.StatusServiceUnavailable)
	first.drop()
	eventually(t, "the client retries while Read is behind", func() bool { return len(server.handshakeHeaders()) >= 5 })
	server.setRejectStatus(0)
	server.nextConn(t)

	if _, ok := readMessage(t, client).(Subscriptions); !ok {
		t.Fatal("first message is not the acknowledgement that filled the queue")
	}
	lost, ok := readMessage(t, client).(Disconnected)
	if !ok || lost.Attempts != 0 {
		t.Fatalf("second message = %#v, want the loss of the connection", lost)
	}

	failed, ok := readMessage(t, client).(Disconnected)
	if !ok || failed.Attempts < 4 || !errors.Is(failed.Err, safetrade.ErrServerError) {
		t.Fatalf("third message = %#v, want the last of at least 4 rejected attempts", failed)
	}
	reconnected, ok := readMessage(t, client).(Reconnected)
	if !ok || reconnected.Attempts != failed.Attempts+1 {
		t.Fatalf("fourth message = %#v, want Reconnected after %d attempts", reconnected, failed.Attempts+1)
	}
	if _, ok := readMessage(t, client).(Subscriptions); !ok {
		t.Fatal("fifth message is not the acknowledgement of the new connection")
	}
}

func TestSubscribeReportsGivingUpBeforeReadSeesTheNotices(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithBufferSize(1), WithReconnect(ReconnectPolicy{
		MinDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, MaxAttempts: 2,
	}))
	first := server.nextConn(t)

	eventually(t, "the queue is full", func() bool { return len(client.messages) == cap(client.messages) })

	server.setRejectStatus(http.StatusServiceUnavailable)
	first.drop()
	eventually(t, "the client gives up", func() bool { return len(server.handshakeHeaders()) == 3 })

	var err error
	eventually(t, "Subscribe reports giving up", func() bool {
		err = client.Subscribe(context.Background(), GlobalTickers)
		return err != nil
	})
	if !strings.Contains(err.Error(), "giving up after 2 reconnection attempts") {
		t.Errorf("Subscribe() error = %v, want the reason the client stopped", err)
	}

	var attempts []int
	err = readUntilError(t, client, func(msg Message) {
		if disconnected, ok := msg.(Disconnected); ok {
			attempts = append(attempts, disconnected.Attempts)
		}
	})
	if !strings.Contains(err.Error(), "giving up after 2 reconnection attempts") {
		t.Errorf("Read() error = %v, want the reason the client stopped", err)
	}
	if want := []int{0, 1}; !slices.Equal(attempts, want) {
		t.Errorf("Disconnected attempts = %v, want %v, before the error", attempts, want)
	}
}

func TestReconnectBacksOffFromUnstableConnections(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)

	client := dialTest(t, server, Public, WithReconnect(ReconnectPolicy{
		MinDelay: 10 * time.Millisecond, MaxDelay: 160 * time.Millisecond,
	}))
	first := server.nextConn(t)
	server.setDropOnAccept(true)
	first.drop()

	eventually(t, "seven handshakes", func() bool { return len(server.handshakeTimes()) >= 7 })
	_ = client.Close()

	times := server.handshakeTimes()
	if gap := times[6].Sub(times[5]); gap < 60*time.Millisecond {
		t.Errorf("wait before the sixth attempt = %v, want at least 80 ms less scheduling slack", gap)
	}
}

func TestNotifierReportsEveryFailedAttemptWhileReadKeepsUp(t *testing.T) {
	t.Parallel()

	queue := make(chan Message, 10)
	n := &notifier{queue: queue}
	for attempt := range 4 {
		n.notify(Disconnected{Attempts: attempt})
	}
	if len(queue) != 4 || len(n.pending) != 0 {
		t.Errorf("%d notices queued and %d pending, want 4 queued", len(queue), len(n.pending))
	}
}

func TestNotifierKeepsTheLatestFailedAttemptWhileReadIsBehind(t *testing.T) {
	t.Parallel()

	queue := make(chan Message, 1)
	n := &notifier{queue: queue}
	n.notify(Disconnected{Attempts: 0})
	for attempt := 1; attempt <= 3; attempt++ {
		n.notify(Disconnected{Attempts: attempt})
	}
	n.notify(Reconnected{Attempts: 4})

	flushed := make(chan bool, 1)
	go func() { flushed <- n.flush(context.Background()) }()
	got := []Message{<-queue, <-queue, <-queue}
	if !<-flushed {
		t.Fatal("flush() = false, want true")
	}
	want := []Message{Disconnected{Attempts: 0}, Disconnected{Attempts: 3}, Reconnected{Attempts: 4}}
	if !slices.Equal(got, want) {
		t.Errorf("notices = %v, want %v", got, want)
	}
}

func TestNotifierSleepPassesNoticesOnAsReadMakesRoom(t *testing.T) {
	t.Parallel()

	queue := make(chan Message, 1)
	n := &notifier{queue: queue}
	n.notify(Disconnected{Attempts: 0})
	n.notify(Disconnected{Attempts: 1})

	ctx, cancel := context.WithCancel(context.Background())
	slept := make(chan bool, 1)
	go func() { slept <- n.sleep(ctx, time.Hour) }()
	if got := <-queue; got != (Disconnected{Attempts: 0}) {
		t.Errorf("first notice = %v, want the loss", got)
	}
	select {
	case got := <-queue:
		if got != (Disconnected{Attempts: 1}) {
			t.Errorf("second notice = %v, want the failed attempt", got)
		}
	case <-time.After(waitTimeout):
		t.Fatal("sleep() kept the pending notice although the queue had room")
	}
	cancel()
	if <-slept {
		t.Error("sleep() = true after its context was canceled, want false")
	}
}

func TestStableUptimeIsAtMostAMinute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		maxDelay time.Duration
		want     time.Duration
	}{
		{maxDelay: 20 * time.Millisecond, want: 20 * time.Millisecond},
		{maxDelay: 30 * time.Second, want: 30 * time.Second},
		{maxDelay: time.Hour, want: time.Minute},
	}
	for _, tt := range tests {
		if got := (ReconnectPolicy{MaxDelay: tt.maxDelay}).stableUptime(); got != tt.want {
			t.Errorf("stableUptime() with MaxDelay %v = %v, want %v", tt.maxDelay, got, tt.want)
		}
	}
}

func TestReconnectDisabled(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithReconnect(ReconnectPolicy{Disabled: true}))
	server.nextConn(t).drop()

	err := readUntilError(t, client, nil)
	if err == nil || !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("Read() error = %v, want connection lost", err)
	}
	if got := len(server.handshakeHeaders()); got != 1 {
		t.Errorf("server saw %d handshakes, want 1", got)
	}
}

func TestKeepAliveReplacesAnUnresponsiveConnection(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	server.setUnresponsive(true)
	client := dialTest(t, server, Public, WithKeepAlive(20*time.Millisecond, 50*time.Millisecond))
	server.nextConn(t)
	server.setUnresponsive(false)

	disconnected := readUntil(t, client, func(Disconnected) bool { return true })
	if disconnected.Err == nil || !strings.Contains(disconnected.Err.Error(), "no pong") {
		t.Errorf("Disconnected.Err = %v, want a pong timeout", disconnected.Err)
	}
	readUntil(t, client, func(Reconnected) bool { return true })
	server.nextConn(t)
}

func TestReadHonorsContextWithoutDroppingTheConnection(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	readMessage(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Read(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Read() error = %v, want context.DeadlineExceeded", err)
	}

	if err := client.Subscribe(context.Background(), GlobalTickers); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, GlobalTickers)
	sc.publish(t, GlobalTickers, `{}`)
	readUpdate(t, client, GlobalTickers)
	if got := len(server.handshakeHeaders()); got != 1 {
		t.Errorf("server saw %d handshakes, want 1: the connection must survive", got)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-sc.closed:
	case <-time.After(waitTimeout):
		t.Fatal("the server did not see the connection close")
	}
	if got := websocket.CloseStatus(sc.closeErr); got != websocket.StatusNormalClosure {
		t.Errorf("close status = %v, want %v", got, websocket.StatusNormalClosure)
	}

	if err := readUntilError(t, client, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("Read() after Close error = %v, want ErrClosed", err)
	}
	if err := client.Subscribe(context.Background(), GlobalTickers); !errors.Is(err, ErrClosed) {
		t.Errorf("Subscribe() after Close error = %v, want ErrClosed", err)
	}
	if err := client.Unsubscribe(context.Background(), GlobalTickers); !errors.Is(err, ErrClosed) {
		t.Errorf("Unsubscribe() after Close error = %v, want ErrClosed", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestCloseAfterTheKeepAliveClosedTheConnection(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)

	client := dialTest(t, server, Public, WithBufferSize(1), WithKeepAlive(20*time.Millisecond, 50*time.Millisecond))
	sc := server.nextConn(t)
	sc.send("not json")
	select {
	case <-sc.closed:
	case <-time.After(waitTimeout):
		t.Fatal("the keepalive did not close the connection")
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil: the connection was already closed", err)
	}
}

func TestFailedWriteDetachesTheConnection(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	server.nextConn(t)

	client.mu.Lock()
	conn := client.conn
	_ = conn.CloseNow()
	err := client.write(conn, eventSubscribe, [][]byte{[]byte(`{"event":"subscribe","streams":["global.tickers"]}`)})
	detached := client.conn == nil
	client.mu.Unlock()

	if err == nil {
		t.Fatal("write() on a closed connection succeeded")
	}
	if !detached {
		t.Error("write() kept the connection it failed on live, so later requests and Close would use it")
	}
}

func TestCloseWhileReconnecting(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithReconnect(ReconnectPolicy{MinDelay: time.Hour}))
	server.nextConn(t).drop()
	eventually(t, "the connection is lost", func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return client.conn == nil
	})

	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Close() blocked while the client waited to reconnect")
	}
	if err := readUntilError(t, client, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("Read() error = %v, want ErrClosed", err)
	}
}

func TestReadDeliversQueuedMessagesBeforeTheError(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithReconnect(ReconnectPolicy{Disabled: true}))
	sc := server.nextConn(t)
	if err := client.Subscribe(context.Background(), GlobalTickers); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	eventually(t, "the server registers the subscription", func() bool { return len(sc.subscriptions()) == 1 })

	for i := range 3 {
		sc.publish(t, GlobalTickers, fmt.Sprintf(`{"n":%d}`, i))
	}
	sc.drop()
	eventually(t, "the client stops", func() bool {
		select {
		case <-client.done:
			return true
		default:
			return false
		}
	})

	var payloads []string
	for {
		msg, err := client.Read(context.Background())
		if err != nil {
			break
		}
		if update, ok := msg.(Update); ok {
			payloads = append(payloads, string(update.Payload))
		}
	}
	if want := []string{`{"n":0}`, `{"n":1}`, `{"n":2}`}; !slices.Equal(payloads, want) {
		t.Errorf("updates read after the connection was lost = %v, want %v", payloads, want)
	}
}

func TestCloseReleasesGoroutines(t *testing.T) {
	server := newFakeServer(t)
	baseline := runtime.NumGoroutine()

	client, err := Dial(context.Background(), Public,
		WithBaseURL(server.baseURL()),
		WithKeepAlive(10*time.Millisecond, 50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	sc := server.nextConn(t)
	if err := client.Subscribe(context.Background(), GlobalTickers); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	<-sc.closed

	eventually(t, "the client goroutines exit", func() bool { return runtime.NumGoroutine() <= baseline })
}

func TestDialFailsWhenTheServerIsUnreachable(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	baseURL := server.baseURL()
	server.server.Close()

	_, err := Dial(context.Background(), Public, WithBaseURL(baseURL))
	if err == nil {
		t.Fatal("Dial() succeeded, want error")
	}
	var apiErr *safetrade.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("Dial() error = %v, want a network error rather than an *APIError", err)
	}
}

func TestWithHTTPClientAndAuthenticator(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	var dials atomic.Int32
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		dials.Add(1)
		return http.DefaultTransport.RoundTrip(req)
	})}
	auth, err := safetrade.NewBasicAuth(testAPIKey, testAPISecret)
	if err != nil {
		t.Fatalf("NewBasicAuth() error = %v", err)
	}

	client, err := Dial(context.Background(), Private,
		WithBaseURL(server.baseURL()),
		WithHTTPClient(httpClient),
		WithAuthenticator(auth),
	)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if got := dials.Load(); got != 1 {
		t.Errorf("the custom HTTP client made %d requests, want 1", got)
	}
	if _, _, ok := (&http.Request{Header: server.handshakeHeaders()[0]}).BasicAuth(); !ok {
		t.Error("the handshake carries no Basic credentials")
	}
}

func TestPublicHandshakeCarriesNoCredentials(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	dialTest(t, server, Public)
	header := server.handshakeHeaders()[0]
	for _, name := range []string{"X-Auth-Apikey", "X-Auth-Nonce", "X-Auth-Signature", "Authorization"} {
		if value := header.Get(name); value != "" {
			t.Errorf("the public handshake carries %s: %q", name, value)
		}
	}
}

func TestPrivateHandshakeRefusesRedirects(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.server.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	_, err := Dial(context.Background(), Private,
		WithBaseURL(redirector.URL+"/api/v2"),
		WithAPIKey(testAPIKey, testAPISecret),
	)
	if err == nil || !strings.Contains(err.Error(), "refusing to follow the redirect") {
		t.Fatalf("Dial() error = %v, want a refused redirect", err)
	}
	if got := len(server.handshakeHeaders()); got != 0 {
		t.Errorf("the redirect target received %d handshakes carrying credentials, want 0", got)
	}
}

func TestSubscribeWithACanceledContextChangesNothing(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	for range 50 {
		if err := client.Subscribe(canceled, GlobalTickers); !errors.Is(err, context.Canceled) {
			t.Fatalf("Subscribe() error = %v, want context.Canceled", err)
		}
	}
	if got := client.Subscriptions(); len(got) != 0 {
		t.Fatalf("Subscriptions() = %v after canceled calls, want none", got)
	}

	if err := client.Subscribe(context.Background(), GlobalTickers); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, GlobalTickers)
	if err := client.Unsubscribe(canceled, GlobalTickers); !errors.Is(err, context.Canceled) {
		t.Fatalf("Unsubscribe() error = %v, want context.Canceled", err)
	}
	if got := sc.subscriptions(); !slices.Equal(got, []string{GlobalTickers}) {
		t.Errorf("server subscriptions = %v, want [global.tickers]", got)
	}
	if got := len(server.handshakeHeaders()); got != 1 {
		t.Errorf("server saw %d handshakes, want 1: canceled calls must not break the connection", got)
	}
}

func TestSubscribeAfterTheClientStopped(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public, WithReconnect(ReconnectPolicy{Disabled: true}))
	server.nextConn(t).drop()
	readErr := readUntilError(t, client, nil)

	for name, call := range map[string]func() error{
		"Subscribe":   func() error { return client.Subscribe(context.Background(), GlobalTickers) },
		"Unsubscribe": func() error { return client.Unsubscribe(context.Background(), GlobalTickers) },
	} {
		if err := call(); err == nil || err.Error() != readErr.Error() {
			t.Errorf("%s() after the client stopped = %v, want %v", name, err, readErr)
		}
	}
	if got := client.Subscriptions(); len(got) != 0 {
		t.Errorf("Subscriptions() = %v, want none", got)
	}
}

func TestSubscribeRejectsStreamNamesTooLongForARequest(t *testing.T) {
	t.Parallel()

	server := newFakeServer(t)
	client := dialTest(t, server, Public)
	sc := server.nextConn(t)

	tooLong := strings.Repeat("x", rangoMaxMessageSize) + ".trades"
	err := client.Subscribe(context.Background(), GlobalTickers, tooLong)
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("Subscribe() error = %v, want the name rejected", err)
	}
	if got := client.Subscriptions(); len(got) != 0 {
		t.Errorf("Subscriptions() = %v, want none: a rejected call changes nothing", got)
	}

	if err := client.Subscribe(context.Background(), GlobalTickers); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	readSubscriptions(t, client, GlobalTickers)
	sc.drop()
	readUntil(t, client, func(Reconnected) bool { return true })
	readSubscriptions(t, client, GlobalTickers)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBackoff(t *testing.T) {
	t.Parallel()

	b := newBackoff(10*time.Millisecond, 50*time.Millisecond)

	for _, delay := range []time.Duration{10, 20, 40, 50, 50} {
		delay *= time.Millisecond
		if got := b.next(); got < delay/2 || got > delay {
			t.Errorf("next() = %v, want between %v and %v", got, delay/2, delay)
		}
	}
	b.reset()
	if got := b.next(); got < 5*time.Millisecond || got > 10*time.Millisecond {
		t.Errorf("next() after reset() = %v, want between 5ms and 10ms", got)
	}

	huge := newBackoff(time.Duration(1<<62), time.Duration(1<<62)+1)
	for range 3 {
		if got := huge.next(); got <= 0 {
			t.Fatalf("next() = %v, want a positive wait", got)
		}
	}
}

func TestJitterStaysWithinBounds(t *testing.T) {
	t.Parallel()

	for _, d := range []time.Duration{0, 1, 2, time.Millisecond, time.Second, time.Hour} {
		for range 100 {
			if got := jitter(d); got < d/2 || got > d {
				t.Fatalf("jitter(%v) = %v, want between %v and %v", d, got, d/2, d)
			}
		}
	}
}

func readUntilError(t *testing.T, c *Client, observe func(Message)) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	for {
		msg, err := c.Read(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("timed out waiting for Read to fail")
			}
			return err
		}
		if observe != nil {
			observe(msg)
		}
	}
}

func equalRequests(a, b []streamRequest) bool {
	return slices.EqualFunc(a, b, func(x, y streamRequest) bool {
		return x.Event == y.Event && slices.Equal(x.Streams, y.Streams)
	})
}
