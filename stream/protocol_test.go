package stream

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEncodeRequests(t *testing.T) {
	t.Parallel()

	messages, err := encodeRequests(eventSubscribe, []string{GlobalTickers, "btcusdt.depth"})
	if err != nil {
		t.Fatalf("encodeRequests() error = %v", err)
	}
	want := `{"event":"subscribe","streams":["global.tickers","btcusdt.depth"]}`
	if len(messages) != 1 || string(messages[0]) != want {
		t.Errorf("encodeRequests() = %q, want [%s]", messages, want)
	}
}

func TestEncodeRequestsSplitsAtTheSizeLimit(t *testing.T) {
	t.Parallel()

	emptySize := len(`{"event":"subscribe","streams":[]}`)
	perStream := len(`"0000000000"`) + 1
	fitting := (maxRequestSize - emptySize + 1) / perStream

	tests := []struct {
		name         string
		count        int
		wantMessages int
	}{
		{name: "one stream", count: 1, wantMessages: 1},
		{name: "as many as fit", count: fitting, wantMessages: 1},
		{name: "one more than fits", count: fitting + 1, wantMessages: 2},
		{name: "many", count: 5*fitting + 3, wantMessages: 6},
	}
	for _, tt := range tests {
		streams := make([]string, tt.count)
		for i := range streams {
			streams[i] = fmt.Sprintf("%010d", i)
		}
		messages, err := encodeRequests(eventSubscribe, streams)
		if err != nil {
			t.Fatalf("%s: encodeRequests() error = %v", tt.name, err)
		}
		if len(messages) != tt.wantMessages {
			t.Errorf("%s: got %d messages, want %d", tt.name, len(messages), tt.wantMessages)
		}

		var decoded []string
		for _, message := range messages {
			if len(message) > maxRequestSize {
				t.Errorf("%s: message of %d bytes exceeds %d", tt.name, len(message), maxRequestSize)
			}
			var req streamRequest
			if err := json.Unmarshal(message, &req); err != nil || req.Event != eventSubscribe {
				t.Fatalf("%s: message %s does not decode to a subscribe request: %v", tt.name, message, err)
			}
			decoded = append(decoded, req.Streams...)
		}
		if !slices.Equal(decoded, streams) {
			t.Errorf("%s: the messages carry %d streams out of order or incomplete", tt.name, len(decoded))
		}
	}
}

func TestEncodeRequestsFillsMessagesExactly(t *testing.T) {
	t.Parallel()

	overhead := len(`{"event":"subscribe","streams":[""]}`)
	exact := strings.Repeat("a", maxRequestSize-overhead)
	messages, err := encodeRequests(eventSubscribe, []string{exact})
	if err != nil {
		t.Fatalf("encodeRequests() error = %v", err)
	}
	if len(messages) != 1 || len(messages[0]) != maxRequestSize {
		t.Errorf("got %d messages of %d bytes, want one of %d", len(messages), len(messages[0]), maxRequestSize)
	}

	if _, err := encodeRequests(eventSubscribe, []string{exact + "a"}); err == nil {
		t.Error("encodeRequests() accepted a stream name that cannot fit a request")
	}
}

func TestEncodeRequestsAccountsForEscaping(t *testing.T) {
	t.Parallel()

	streams := []string{strings.Repeat("<", 70), strings.Repeat("<", 70)}
	messages, err := encodeRequests(eventUnsubscribe, streams)
	if err != nil {
		t.Fatalf("encodeRequests() error = %v", err)
	}
	for _, message := range messages {
		if len(message) > maxRequestSize {
			t.Errorf("message of %d bytes exceeds %d", len(message), maxRequestSize)
		}
	}
	if len(messages) != 2 {
		t.Errorf("got %d messages, want 2", len(messages))
	}
}

func TestParseFrame(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		frame string
		want  []Message
	}{
		{
			name:  "subscription acknowledgement",
			frame: `{"success":{"message":"subscribed","streams":["global.tickers","order"]}}`,
			want:  []Message{Subscriptions{Message: "subscribed", Streams: []string{"global.tickers", "order"}}},
		},
		{
			name:  "acknowledgement with a text message",
			frame: `{"success":"ok"}`,
			want:  []Message{Subscriptions{Message: "ok"}},
		},
		{
			name:  "server error as a string",
			frame: `{"error":"Could not parse Type: Invalid event"}`,
			want:  []Message{ServerError{Message: "Could not parse Type: Invalid event"}},
		},
		{
			name:  "server error as an object",
			frame: `{"error":{"message":"unsupported method"}}`,
			want:  []Message{ServerError{Message: "unsupported method"}},
		},
		{
			name:  "server error in another shape",
			frame: `{"error":{"code":42}}`,
			want:  []Message{ServerError{Message: `{"code":42}`}},
		},
		{
			name:  "update",
			frame: `{"btcusdt.depth":{"asks":[["1","2"]],"bids":[],"sequence":7}}`,
			want: []Message{Update{
				Stream:  "btcusdt.depth",
				Payload: json.RawMessage(`{"asks":[["1","2"]],"bids":[],"sequence":7}`),
			}},
		},
		{
			name:  "several members in order",
			frame: `{"b.trades":[1],"a.trades":[2],"order":{"id":3}}`,
			want: []Message{
				Update{Stream: "b.trades", Payload: json.RawMessage(`[1]`)},
				Update{Stream: "a.trades", Payload: json.RawMessage(`[2]`)},
				Update{Stream: "order", Payload: json.RawMessage(`{"id":3}`)},
			},
		},
		{name: "empty object", frame: `{}`, want: nil},
		{name: "text pong", frame: "pong", want: nil},
		{name: "blank frame", frame: " \n", want: nil},
		{name: "invalid JSON", frame: `{"order":`, want: []Message{UnknownFrame{Data: []byte(`{"order":`)}}},
		{name: "array", frame: `[1,2]`, want: []Message{UnknownFrame{Data: []byte(`[1,2]`)}}},
		{name: "trailing data", frame: `{"order":{}} {}`, want: []Message{UnknownFrame{Data: []byte(`{"order":{}} {}`)}}},
	}
	for _, tt := range tests {
		got := parseFrame([]byte(tt.frame))
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: parseFrame(%s) = %#v, want %#v", tt.name, tt.frame, got, tt.want)
		}
	}
}

func TestServerErrorIsAnError(t *testing.T) {
	t.Parallel()

	var err error = ServerError{Message: "unsupported method"}
	if got, want := err.Error(), "stream: server error: unsupported method"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
