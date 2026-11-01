package safetrade_test

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/anuketfluxus/safetrade-api-client"
)

const (
	http2FrameHeaders   = 0x1
	http2FrameRSTStream = 0x3
	http2FrameSettings  = 0x4
	http2FlagAck        = 0x1
	http2ProtocolError  = 0x1
	http2PrefaceLength  = 24
	http2HeaderLength   = 9
)

func newResettingHTTP2Server(t *testing.T) (baseURL string, httpClient *http.Client, requests *atomic.Int32) {
	t.Helper()
	certificateSource := httptest.NewUnstartedServer(nil)
	certificateSource.EnableHTTP2 = true
	certificateSource.StartTLS()
	t.Cleanup(certificateSource.Close)

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: certificateSource.TLS.Certificates,
		NextProtos:   []string{"h2"},
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	requests = new(atomic.Int32)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go resetEveryStream(conn, requests)
		}
	}()
	return "https://" + listener.Addr().String() + "/api/v2", certificateSource.Client(), requests
}

func resetEveryStream(conn net.Conn, requests *atomic.Int32) {
	defer conn.Close()
	if _, err := io.ReadFull(conn, make([]byte, http2PrefaceLength)); err != nil {
		return
	}
	writeHTTP2Frame(conn, http2FrameSettings, 0, 0, nil)
	header := make([]byte, http2HeaderLength)
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
		if _, err := io.ReadFull(conn, make([]byte, length)); err != nil {
			return
		}
		frameType, flags := header[3], header[4]
		stream := binary.BigEndian.Uint32(header[5:]) & 0x7fffffff
		switch {
		case frameType == http2FrameSettings && flags&http2FlagAck == 0:
			writeHTTP2Frame(conn, http2FrameSettings, http2FlagAck, 0, nil)
		case frameType == http2FrameHeaders:
			requests.Add(1)
			code := make([]byte, 4)
			binary.BigEndian.PutUint32(code, http2ProtocolError)
			writeHTTP2Frame(conn, http2FrameRSTStream, 0, stream, code)
		}
	}
}

func writeHTTP2Frame(conn net.Conn, frameType, flags byte, stream uint32, payload []byte) {
	frame := make([]byte, http2HeaderLength, http2HeaderLength+len(payload))
	frame[0], frame[1], frame[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
	frame[3], frame[4] = frameType, flags
	binary.BigEndian.PutUint32(frame[5:], stream)
	_, _ = conn.Write(append(frame, payload...))
}

func TestRequestsThatChangeTheAccountReachHTTP2ServersAtMostOnce(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *safetrade.Client) error
	}{
		{
			name: "CreateOrder",
			call: func(ctx context.Context, client *safetrade.Client) error {
				_, err := client.Market.CreateOrder(ctx, safetrade.CreateOrderParams{
					Market: "btcusdt", Side: safetrade.SideBuy, Type: safetrade.OrderTypeMarket,
					Amount: decimal.RequireFromString("1"),
				})
				return err
			},
		},
		{
			name: "CancelOrder",
			call: func(ctx context.Context, client *safetrade.Client) error {
				_, err := client.Market.CancelOrder(ctx, 42)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseURL, httpClient, requests := newResettingHTTP2Server(t)
			client, err := safetrade.NewClient(
				safetrade.WithBaseURL(baseURL),
				safetrade.WithHTTPClient(httpClient),
				safetrade.WithAPIKey("key", "secret"),
			)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err = test.call(ctx, client)

			if got := requests.Load(); got != 1 {
				t.Errorf("the server received %d requests, want 1", got)
			}
			if !safetrade.MayHaveTakenEffect(err) {
				t.Errorf("MayHaveTakenEffect(%v) = false, want true", err)
			}
		})
	}
}
