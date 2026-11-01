package stream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	eventSubscribe   = "subscribe"
	eventUnsubscribe = "unsubscribe"

	maxRequestSize = 512

	replySuccess = "success"
	replyError   = "error"
)

type streamRequest struct {
	Event   string   `json:"event"`
	Streams []string `json:"streams"`
}

func encodeRequests(event string, streams []string) ([][]byte, error) {
	emptySize := len(mustMarshal(streamRequest{Event: event, Streams: []string{}}))

	var messages [][]byte
	var batch []string
	size := emptySize
	flush := func() error {
		message := mustMarshal(streamRequest{Event: event, Streams: batch})
		if len(message) > maxRequestSize {
			return fmt.Errorf("stream: %s request of %d bytes exceeds the %d-byte limit", event, len(message), maxRequestSize)
		}
		messages = append(messages, message)
		batch, size = nil, emptySize
		return nil
	}

	for _, stream := range streams {
		added := len(mustMarshal(stream))
		if len(batch) > 0 {
			added++
		}
		if size+added > maxRequestSize && len(batch) > 0 {
			if err := flush(); err != nil {
				return nil, err
			}
			added = len(mustMarshal(stream))
		}
		if size+added > maxRequestSize {
			return nil, fmt.Errorf("stream: stream name %q is too long to fit a %d-byte request", stream, maxRequestSize)
		}
		batch = append(batch, stream)
		size += added
	}
	if len(batch) > 0 {
		if err := flush(); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func fitsRequest(stream string) bool {
	_, err := encodeRequests(eventUnsubscribe, []string{stream})
	return err == nil
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("stream: encoding %T: %v", v, err))
	}
	return data
}

func parseFrame(frame []byte) []Message {
	trimmed := bytes.TrimSpace(frame)
	if len(trimmed) == 0 || string(trimmed) == "pong" {
		return nil
	}
	messages, err := parseObject(trimmed)
	if err != nil {
		return []Message{UnknownFrame{Data: frame}}
	}
	return messages
}

var errNotObject = errors.New("frame is not a JSON object")

func parseObject(data []byte) ([]Message, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errNotObject
	}

	var messages []Message
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errNotObject
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		messages = append(messages, newMessage(key, value))
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the JSON object")
	}
	return messages, nil
}

func newMessage(key string, value json.RawMessage) Message {
	switch key {
	case replySuccess:
		var reply struct {
			Message string   `json:"message"`
			Streams []string `json:"streams"`
		}
		if err := json.Unmarshal(value, &reply); err != nil {
			return Subscriptions{Message: text(value)}
		}
		return Subscriptions{Message: reply.Message, Streams: reply.Streams}
	case replyError:

		var reply struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(value, &reply); err == nil && reply.Message != "" {
			return ServerError{Message: reply.Message}
		}
		return ServerError{Message: text(value)}
	default:
		return Update{Stream: key, Payload: value}
	}
}

func text(value json.RawMessage) string {
	var s string
	if err := json.Unmarshal(value, &s); err == nil {
		return s
	}
	return string(value)
}
