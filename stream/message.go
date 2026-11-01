package stream

import (
	"encoding/json"
)

type Message interface {
	isMessage()
}

type Update struct {
	Stream string

	Payload json.RawMessage
}

type Subscriptions struct {
	Message string

	Streams []string
}

type ServerError struct {
	Message string
}

func (e ServerError) Error() string {
	return "stream: server error: " + e.Message
}

type Disconnected struct {
	Err error

	Attempts int
}

type Reconnected struct {
	Attempts int
}

type UnknownFrame struct {
	Data []byte
}

func (Update) isMessage()        {}
func (Subscriptions) isMessage() {}
func (ServerError) isMessage()   {}
func (Disconnected) isMessage()  {}
func (Reconnected) isMessage()   {}
func (UnknownFrame) isMessage()  {}
