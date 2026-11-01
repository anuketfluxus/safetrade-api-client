package safetrade

import (
	"github.com/shopspring/decimal"
)

type Order struct {
	ID     int64      `json:"id"`
	Market string     `json:"market"`
	Side   Side       `json:"side"`
	Type   OrderType  `json:"type"`
	State  OrderState `json:"state"`

	Ask string `json:"ask"`
	Bid string `json:"bid"`

	Price decimal.Decimal `json:"price"`

	AvgPrice  decimal.Decimal `json:"avg_price"`
	StopPrice decimal.Decimal `json:"stop_price"`
	MaxPrice  decimal.Decimal `json:"max_price"`

	OriginAmount decimal.Decimal `json:"origin_amount"`
	FilledAmount decimal.Decimal `json:"filled_amount"`
	MakerFee     decimal.Decimal `json:"maker_fee"`
	TakerFee     decimal.Decimal `json:"taker_fee"`
	TradesCount  int             `json:"trades_count"`
	CreatedAt    Timestamp       `json:"created_at"`
	UpdatedAt    Timestamp       `json:"updated_at"`

	TriggeredAt Timestamp `json:"triggered_at"`
}

type Trade struct {
	ID      int64  `json:"id"`
	Market  string `json:"market"`
	OrderID int64  `json:"order_id"`
	Side    Side   `json:"side"`

	OrderSide Side            `json:"order_side"`
	Price     decimal.Decimal `json:"price"`
	Amount    decimal.Decimal `json:"amount"`

	Total       decimal.Decimal `json:"total"`
	Fee         decimal.Decimal `json:"fee"`
	FeeCurrency string          `json:"fee_currency"`
	CreatedAt   Timestamp       `json:"created_at"`
}
