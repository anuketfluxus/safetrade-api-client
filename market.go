package safetrade

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type MarketService struct {
	client *Client
}

type ListOrdersParams struct {
	Market    string
	States    []OrderState
	Types     []OrderType
	Sides     []Side
	BaseUnit  string
	QuoteUnit string
	TimeFrom  time.Time
	TimeTo    time.Time
	OrderBy   string
	Ordering  Ordering

	Page  int
	Limit int
}

func (s *MarketService) ListOrders(ctx context.Context, params ListOrdersParams) ([]Order, error) {
	q := query{}
	q.set("market", params.Market)
	setList(q, "state", params.States)
	setList(q, "type", params.Types)
	setList(q, "side", params.Sides)
	q.set("base_unit", params.BaseUnit)
	q.set("quote_unit", params.QuoteUnit)
	q.setUnix("time_from", params.TimeFrom)
	q.setUnix("time_to", params.TimeTo)
	q.setSort(params.OrderBy, params.Ordering)
	q.setPage(params.Page, params.Limit)
	return call[[]Order](ctx, s.client, privateGet("/trade/market/orders", q))
}

func (s *MarketService) GetOrder(ctx context.Context, id int64) (Order, error) {
	segment, err := idSegment("order ID", id)
	if err != nil {
		return Order{}, err
	}
	return call[Order](ctx, s.client, privateGet("/trade/market/orders/"+segment, nil))
}

type CreateOrderParams struct {
	Market string
	Side   Side
	Type   OrderType

	Amount decimal.Decimal

	Price decimal.Decimal

	StopPrice decimal.Decimal

	MaxPrice decimal.Decimal

	Total decimal.Decimal
}

var orderFieldRules = map[OrderType]struct{ required, forbidden []string }{
	OrderTypeLimit:       {required: []string{"amount", "price"}, forbidden: []string{"stop price", "total"}},
	OrderTypeStopLimit:   {required: []string{"amount", "price", "stop price"}, forbidden: []string{"total"}},
	OrderTypeMarket:      {required: []string{"amount"}, forbidden: []string{"price", "stop price", "total"}},
	OrderTypeMarketQuote: {required: []string{"total"}, forbidden: []string{"amount", "price", "stop price"}},
}

func (p CreateOrderParams) validate() error {
	if p.Market == "" {
		return invalidArgument("market must not be empty")
	}
	if err := validateSide(p.Side); err != nil {
		return err
	}
	if p.Type == "" {
		return invalidArgument("type must not be empty")
	}
	rules, known := orderFieldRules[p.Type]
	if !known {
		return invalidArgument("unknown order type %q, want %s, %s, %s, or %s",
			p.Type, OrderTypeLimit, OrderTypeStopLimit, OrderTypeMarket, OrderTypeMarketQuote)
	}

	names := []string{"amount", "price", "stop price", "max price", "total"}
	values := map[string]decimal.Decimal{
		"amount": p.Amount, "price": p.Price, "stop price": p.StopPrice, "max price": p.MaxPrice, "total": p.Total,
	}
	for _, name := range names {
		if values[name].IsNegative() {
			return invalidArgument("%s must not be negative, got %s", name, values[name])
		}
	}
	for _, name := range rules.required {
		if !values[name].IsPositive() {
			return invalidArgument("a %s order needs a positive %s", p.Type, name)
		}
	}
	for _, name := range rules.forbidden {
		if !values[name].IsZero() {
			return invalidArgument("a %s order must not set %s: the API would create another type of order", p.Type, name)
		}
	}
	return nil
}

func validateSide(side Side) error {
	switch side {
	case SideBuy, SideSell:
		return nil
	case "":
		return invalidArgument("side must not be empty")
	default:
		return invalidArgument("side must be %s or %s, got %q", SideBuy, SideSell, side)
	}
}

type createOrderBody struct {
	Market string `json:"market"`
	Side   Side   `json:"side"`

	Type      OrderType       `json:"type,omitempty"`
	Amount    decimal.Decimal `json:"amount,omitzero"`
	Price     decimal.Decimal `json:"price,omitzero"`
	StopPrice decimal.Decimal `json:"stop_price,omitzero"`
	MaxPrice  decimal.Decimal `json:"max_price,omitzero"`
	Total     decimal.Decimal `json:"total,omitzero"`
}

func newCreateOrderBody(p CreateOrderParams) createOrderBody {
	body := createOrderBody{
		Market:    p.Market,
		Side:      p.Side,
		Amount:    p.Amount,
		Price:     p.Price,
		StopPrice: p.StopPrice,
		MaxPrice:  p.MaxPrice,
		Total:     p.Total,
	}
	if p.Type == OrderTypeLimit || p.Type == OrderTypeMarket {
		body.Type = p.Type
	}
	return body
}

func (s *MarketService) CreateOrder(ctx context.Context, params CreateOrderParams) (Order, error) {
	if err := params.validate(); err != nil {
		return Order{}, err
	}
	return call[Order](ctx, s.client, privatePost("/trade/market/orders", newCreateOrderBody(params)))
}

func (s *MarketService) CancelOrder(ctx context.Context, id int64) (Order, error) {
	segment, err := idSegment("order ID", id)
	if err != nil {
		return Order{}, err
	}
	return call[Order](ctx, s.client, privatePost("/trade/market/orders/"+segment+"/cancel", nil))
}

type CancelOrdersParams struct {
	Market string `json:"market"`

	Side Side `json:"side,omitempty"`
}

func (s *MarketService) CancelOrders(ctx context.Context, params CancelOrdersParams) error {
	if params.Market == "" {
		return invalidArgument("market must not be empty; use AllMarkets to cancel on every market")
	}
	if params.Side != "" {
		if err := validateSide(params.Side); err != nil {
			return err
		}
	}
	return s.client.do(ctx, privatePost("/trade/market/orders/cancel", params), nil)
}

type ListTradesParams struct {
	Market   string
	Sides    []Side
	TimeFrom time.Time
	TimeTo   time.Time
	OrderBy  string
	Ordering Ordering

	Page  int
	Limit int
}

func (s *MarketService) ListTrades(ctx context.Context, params ListTradesParams) ([]Trade, error) {
	q := query{}
	q.set("market", params.Market)
	setList(q, "side", params.Sides)
	q.setUnix("time_from", params.TimeFrom)
	q.setUnix("time_to", params.TimeTo)
	q.setSort(params.OrderBy, params.Ordering)
	q.setPage(params.Page, params.Limit)
	return call[[]Trade](ctx, s.client, privateGet("/trade/market/trades", q))
}

func (s *MarketService) GetTrade(ctx context.Context, id int64) (Trade, error) {
	segment, err := idSegment("trade ID", id)
	if err != nil {
		return Trade{}, err
	}
	return call[Trade](ctx, s.client, privateGet("/trade/market/trades/"+segment, nil))
}
