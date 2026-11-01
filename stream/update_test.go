package stream

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anuketfluxus/safetrade-api-client"
)

func TestUpdateKindAndMarket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		stream string
		kind   Kind
		market string
	}{
		{stream: GlobalTickers, kind: KindTickers},
		{stream: MarketTrades("btcusdt"), kind: KindMarketTrades, market: "btcusdt"},
		{stream: MarketDepth("qubicusdt"), kind: KindMarketDepth, market: "qubicusdt"},
		{stream: Orders, kind: KindOrder},
		{stream: Trades, kind: KindTrade},
		{stream: Balances, kind: KindBalance},
		{stream: "btcusdt.kline-1m", kind: KindUnknown},
		{stream: ".trades", kind: KindUnknown},
		{stream: "btcusdt.trades.extra", kind: KindUnknown},
		{stream: "notifications", kind: KindUnknown},
	}
	for _, tt := range tests {
		update := Update{Stream: tt.stream}
		if got := update.Kind(); got != tt.kind {
			t.Errorf("Kind() of %q = %v, want %v", tt.stream, got, tt.kind)
		}
		if got := update.Market(); got != tt.market {
			t.Errorf("Market() of %q = %q, want %q", tt.stream, got, tt.market)
		}
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	want := map[Kind]string{
		KindUnknown:      "unknown",
		KindTickers:      "tickers",
		KindMarketTrades: "market trades",
		KindMarketDepth:  "market depth",
		KindOrder:        "order",
		KindTrade:        "trade",
		KindBalance:      "balance",
		Kind(99):         "unknown",
	}
	for kind, name := range want {
		if got := kind.String(); got != name {
			t.Errorf("Kind(%d).String() = %q, want %q", int(kind), got, name)
		}
	}
}

func TestUpdateMarketTradesAcceptsEveryListShape(t *testing.T) {
	t.Parallel()

	payloads := map[string]string{
		"list":     `[{"id":1,"side":"buy","price":"10","amount":"1"},{"id":2,"side":"sell","price":11,"amount":2}]`,
		"envelope": `{"trades":[{"id":1,"side":"buy","price":"10","amount":"1"},{"id":2,"side":"sell","price":11,"amount":2}]}`,
	}
	for shape, payload := range payloads {
		trades, err := Update{Stream: "btcusdt.trades", Payload: json.RawMessage(payload)}.MarketTrades()
		if err != nil {
			t.Errorf("%s: MarketTrades() error = %v", shape, err)
			continue
		}
		if len(trades) != 2 || trades[0].ID != 1 || trades[1].Side != safetrade.SideSell || trades[1].Price.String() != "11" {
			t.Errorf("%s: MarketTrades() = %+v", shape, trades)
		}
	}

	single, err := Update{Stream: "btcusdt.trades", Payload: json.RawMessage(`{"id":3,"price":"9","amount":"0.5"}`)}.MarketTrades()
	if err != nil || len(single) != 1 || single[0].ID != 3 || single[0].Amount.String() != "0.5" {
		t.Errorf("MarketTrades() of a single trade = %+v, %v", single, err)
	}
}

func TestUpdateBalancesAcceptsListAndObject(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{
		`[{"currency":"usdt","balance":"100","locked":"5","type":"spot"}]`,
		`{"currency":"usdt","balance":"100","locked":"5","type":"spot"}`,
	} {
		balances, err := Update{Stream: Balances, Payload: json.RawMessage(payload)}.Balances()
		if err != nil {
			t.Errorf("Balances(%s) error = %v", payload, err)
			continue
		}
		if len(balances) != 1 || balances[0].Currency != "usdt" || balances[0].Locked.String() != "5" ||
			balances[0].Type != safetrade.AccountTypeSpot {
			t.Errorf("Balances(%s) = %+v", payload, balances)
		}
	}
}

func TestUpdateTypedDecoders(t *testing.T) {
	t.Parallel()

	depth, err := Update{
		Stream:  "btcusdt.depth",
		Payload: json.RawMessage(`{"asks":[["67010.5","0.25"]],"bids":[[67000,1]],"sequence":99}`),
	}.Depth()
	if err != nil || len(depth.Asks) != 1 || depth.Bids[0].Price.String() != "67000" || depth.Sequence != 99 {
		t.Errorf("Depth() = %+v, %v", depth, err)
	}

	order, err := Update{
		Stream:  Orders,
		Payload: json.RawMessage(`{"id":5,"market":"btcusdt","side":"buy","state":"done","filled_amount":"1","created_at":1761436800}`),
	}.Order()
	if err != nil || order.State != safetrade.OrderStateDone || order.FilledAmount.String() != "1" ||
		order.CreatedAt.Unix() != 1761436800 {
		t.Errorf("Order() = %+v, %v", order, err)
	}

	trade, err := Update{
		Stream:  Trades,
		Payload: json.RawMessage(`{"id":8,"market":"btcusdt","order_id":5,"price":"67000","amount":"0.01","fee":"0.001","fee_currency":"btc"}`),
	}.Trade()
	if err != nil || trade.OrderID != 5 || trade.Fee.String() != "0.001" {
		t.Errorf("Trade() = %+v, %v", trade, err)
	}

	tickers, err := Update{
		Stream:  GlobalTickers,
		Payload: json.RawMessage(`{"xtmusdt":{"last":0.45,"amount":"125000","avg_price":"0.451","price_change_percent":"+2.27%"}}`),
	}.Tickers()
	if err != nil || tickers["xtmusdt"].Last.String() != "0.45" || tickers["xtmusdt"].PriceChangePercent != "+2.27%" {
		t.Errorf("Tickers() = %+v, %v", tickers, err)
	}
}

func TestUpdateTypedDecodersRejectPayloadsOfAnotherShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		update  Update
		decode  func(Update) error
		missing string
	}{
		{
			update:  Update{Stream: "eurusd.trades", Payload: json.RawMessage(`{"trades":[{"tid":7,"taker_type":"buy","date":1588000798,"price":"1020.0","amount":"0.001"}]}`)},
			decode:  func(u Update) error { _, err := u.MarketTrades(); return err },
			missing: "has no id, which every trade has",
		},
		{
			update:  Update{Stream: Orders, Payload: json.RawMessage(`{"id":22,"at":1588000798,"market":"eurusd","kind":"bid","price":"1026.0","state":"wait","volume":"0.001","origin_volume":"0.001"}`)},
			decode:  func(u Update) error { _, err := u.Order(); return err },
			missing: "has no side, which every order has",
		},
		{
			update:  Update{Stream: Trades, Payload: json.RawMessage(`{"id":7,"kind":"ask","at":1588000798,"price":"1020.0","volume":"0.001","ask_id":15,"bid_id":22,"market":"eurusd"}`)},
			decode:  func(u Update) error { _, err := u.Trade(); return err },
			missing: "has no amount, which every trade has",
		},
		{
			update:  Update{Stream: Balances, Payload: json.RawMessage(`{"usdt":["100.0","5.0"]}`)},
			decode:  func(u Update) error { _, err := u.Balances(); return err },
			missing: "has no currency, which every balance has",
		},
		{
			update:  Update{Stream: "btcusdt.depth", Payload: json.RawMessage(`{"data":{"asks":[["1","2"]]}}`)},
			decode:  func(u Update) error { _, err := u.Depth(); return err },
			missing: "has no asks, no bids, and no sequence",
		},
		{
			update:  Update{Stream: "btcusdt.depth", Payload: json.RawMessage(`null`)},
			decode:  func(u Update) error { _, err := u.Depth(); return err },
			missing: "has no asks, no bids, and no sequence",
		},
	}
	for _, tt := range tests {
		err := tt.decode(tt.update)
		if err == nil || !strings.Contains(err.Error(), tt.missing) {
			t.Errorf("decoding %s = %v, want an error saying it %s", tt.update.Payload, err, tt.missing)
		}
	}
}

func TestUpdateDepthAcceptsPartialUpdates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		payload string
		levels  int
	}{
		{payload: `{"asks":[["67010.5","0.25"]]}`, levels: 1},
		{payload: `{"bids":[]}`, levels: 0},
		{payload: `{"sequence":7}`, levels: 0},

		{payload: `{"BIDS":[["67000","1"],["66990","2"]]}`, levels: 2},
	}
	for _, tt := range tests {
		depth, err := Update{Stream: "btcusdt.depth", Payload: json.RawMessage(tt.payload)}.Depth()
		if err != nil {
			t.Errorf("Depth() of %s error = %v", tt.payload, err)
			continue
		}
		if got := len(depth.Asks) + len(depth.Bids); got != tt.levels {
			t.Errorf("Depth() of %s has %d price levels, want %d", tt.payload, got, tt.levels)
		}
	}
}

func TestUpdateTypedDecodersCheckTheKind(t *testing.T) {
	t.Parallel()

	update := Update{Stream: GlobalTickers, Payload: json.RawMessage(`{}`)}
	decoders := map[string]func() error{
		"MarketTrades": func() error { _, err := update.MarketTrades(); return err },
		"Depth":        func() error { _, err := update.Depth(); return err },
		"Order":        func() error { _, err := update.Order(); return err },
		"Trade":        func() error { _, err := update.Trade(); return err },
		"Balances":     func() error { _, err := update.Balances(); return err },
	}
	for name, decode := range decoders {
		err := decode()
		if err == nil || !strings.Contains(err.Error(), `update on "global.tickers" has kind tickers`) {
			t.Errorf("%s() error = %v, want a kind mismatch", name, err)
		}
	}
	if _, err := (Update{Stream: Orders, Payload: json.RawMessage(`{}`)}).Tickers(); err == nil {
		t.Error("Tickers() of an order update succeeded, want a kind mismatch")
	}
}

func TestUpdateDecodeReportsInvalidPayloads(t *testing.T) {
	t.Parallel()

	update := Update{Stream: Orders, Payload: json.RawMessage(`{"id":"not a number"}`)}
	if _, err := update.Order(); err == nil || !strings.Contains(err.Error(), `decoding "order" update`) {
		t.Errorf("Order() error = %v, want a decoding error naming the stream", err)
	}

	var custom struct {
		ID int64 `json:"id"`
	}
	if err := (Update{Stream: "custom", Payload: json.RawMessage(`{"id":12}`)}).Decode(&custom); err != nil || custom.ID != 12 {
		t.Errorf("Decode() = %+v, %v", custom, err)
	}
}
