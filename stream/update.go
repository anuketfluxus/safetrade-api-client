package stream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuketfluxus/safetrade-api-client"
)

const (
	GlobalTickers = "global.tickers"

	Orders = "order"

	Trades = "trade"

	Balances = "balance"
)

func MarketTrades(market string) string {
	return market + ".trades"
}

func MarketDepth(market string) string {
	return market + ".depth"
}

func isPrivate(stream string) bool {
	return !strings.Contains(stream, ".")
}

type Kind int

const (
	KindUnknown Kind = iota
	KindTickers
	KindMarketTrades
	KindMarketDepth
	KindOrder
	KindTrade
	KindBalance
)

func (k Kind) String() string {
	switch k {
	case KindTickers:
		return "tickers"
	case KindMarketTrades:
		return "market trades"
	case KindMarketDepth:
		return "market depth"
	case KindOrder:
		return "order"
	case KindTrade:
		return "trade"
	case KindBalance:
		return "balance"
	default:
		return "unknown"
	}
}

func (u Update) Kind() Kind {
	switch u.Stream {
	case GlobalTickers:
		return KindTickers
	case Orders:
		return KindOrder
	case Trades:
		return KindTrade
	case Balances:
		return KindBalance
	}
	market, channel, found := strings.Cut(u.Stream, ".")
	if !found || market == "" {
		return KindUnknown
	}
	switch channel {
	case "trades":
		return KindMarketTrades
	case "depth":
		return KindMarketDepth
	default:
		return KindUnknown
	}
}

func (u Update) Market() string {
	switch u.Kind() {
	case KindMarketTrades, KindMarketDepth:
		market, _, _ := strings.Cut(u.Stream, ".")
		return market
	default:
		return ""
	}
}

func (u Update) Decode(v any) error {
	if err := json.Unmarshal(u.Payload, v); err != nil {
		return fmt.Errorf("stream: decoding %q update: %w", u.Stream, err)
	}
	return nil
}

func (u Update) Tickers() (map[string]safetrade.Ticker, error) {
	if err := u.expectKind(KindTickers); err != nil {
		return nil, err
	}
	var tickers map[string]safetrade.Ticker
	if err := u.Decode(&tickers); err != nil {
		return nil, err
	}
	return tickers, nil
}

func (u Update) MarketTrades() ([]safetrade.PublicTrade, error) {
	if err := u.expectKind(KindMarketTrades); err != nil {
		return nil, err
	}
	payload := u.Payload
	if isObject(payload) {
		var envelope map[string]json.RawMessage
		if err := u.Decode(&envelope); err != nil {
			return nil, err
		}
		if list, ok := envelope["trades"]; ok {
			payload = list
		}
	}
	trades, err := decodeList[safetrade.PublicTrade](u.Stream, payload)
	if err != nil {
		return nil, err
	}
	for _, trade := range trades {
		err := u.require("trade",
			field{"id", trade.ID != 0},
			field{"price", !trade.Price.IsZero()},
			field{"amount", !trade.Amount.IsZero()},
		)
		if err != nil {
			return nil, err
		}
	}
	return trades, nil
}

func (u Update) Depth() (safetrade.Depth, error) {
	if err := u.expectKind(KindMarketDepth); err != nil {
		return safetrade.Depth{}, err
	}
	var depth safetrade.Depth
	if err := u.Decode(&depth); err != nil {
		return safetrade.Depth{}, err
	}

	var fields struct {
		Asks     json.RawMessage `json:"asks"`
		Bids     json.RawMessage `json:"bids"`
		Sequence json.RawMessage `json:"sequence"`
	}
	if err := u.Decode(&fields); err != nil {
		return safetrade.Depth{}, err
	}
	if fields.Asks == nil && fields.Bids == nil && fields.Sequence == nil {
		return safetrade.Depth{}, fmt.Errorf("stream: %q payload has no asks, no bids, and no sequence; use Update.Decode for other shapes", u.Stream)
	}
	return depth, nil
}

func (u Update) Order() (safetrade.Order, error) {
	if err := u.expectKind(KindOrder); err != nil {
		return safetrade.Order{}, err
	}
	var order safetrade.Order
	if err := u.Decode(&order); err != nil {
		return safetrade.Order{}, err
	}
	err := u.require("order",
		field{"id", order.ID != 0},
		field{"market", order.Market != ""},
		field{"side", order.Side != ""},
		field{"state", order.State != ""},
	)
	if err != nil {
		return safetrade.Order{}, err
	}
	return order, nil
}

func (u Update) Trade() (safetrade.Trade, error) {
	if err := u.expectKind(KindTrade); err != nil {
		return safetrade.Trade{}, err
	}
	var trade safetrade.Trade
	if err := u.Decode(&trade); err != nil {
		return safetrade.Trade{}, err
	}
	err := u.require("trade",
		field{"id", trade.ID != 0},
		field{"market", trade.Market != ""},
		field{"price", !trade.Price.IsZero()},
		field{"amount", !trade.Amount.IsZero()},
	)
	if err != nil {
		return safetrade.Trade{}, err
	}
	return trade, nil
}

func (u Update) Balances() ([]safetrade.Balance, error) {
	if err := u.expectKind(KindBalance); err != nil {
		return nil, err
	}
	balances, err := decodeList[safetrade.Balance](u.Stream, u.Payload)
	if err != nil {
		return nil, err
	}
	for _, balance := range balances {
		if err := u.require("balance", field{"currency", balance.Currency != ""}); err != nil {
			return nil, err
		}
	}
	return balances, nil
}

type field struct {
	name    string
	present bool
}

func (u Update) require(entity string, fields ...field) error {
	for _, f := range fields {
		if !f.present {
			return fmt.Errorf("stream: %q payload has no %s, which every %s has; use Update.Decode for other shapes", u.Stream, f.name, entity)
		}
	}
	return nil
}

func (u Update) expectKind(want Kind) error {
	if got := u.Kind(); got != want {
		return fmt.Errorf("stream: update on %q has kind %v, want %v", u.Stream, got, want)
	}
	return nil
}

func decodeList[T any](stream string, payload json.RawMessage) ([]T, error) {
	if isObject(payload) {
		var item T
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, fmt.Errorf("stream: decoding %q update: %w", stream, err)
		}
		return []T{item}, nil
	}
	var items []T
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("stream: decoding %q update: %w", stream, err)
	}
	return items, nil
}

func isObject(payload json.RawMessage) bool {
	trimmed := bytes.TrimSpace(payload)
	return len(trimmed) > 0 && trimmed[0] == '{'
}
