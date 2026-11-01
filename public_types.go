package safetrade

import (
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

type Currency struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	IconURL     string `json:"icon_url"`
	Status      string `json:"status"`

	Precision int `json:"precision"`

	Position int `json:"position"`

	Price decimal.Decimal `json:"price"`

	DefaultNetworkID int64 `json:"default_network_id"`

	Networks  []CurrencyNetwork `json:"networks"`
	CreatedAt Timestamp         `json:"created_at"`
	UpdatedAt Timestamp         `json:"updated_at"`
}

type CurrencyNetwork struct {
	ID         int64  `json:"id"`
	CurrencyID string `json:"currency_id"`

	BlockchainKey string `json:"blockchain_key"`
	ParentID      string `json:"parent_id"`
	Protocol      string `json:"protocol"`
	ProtocolName  string `json:"protocol_name"`
	Client        string `json:"client"`
	Status        string `json:"status"`

	Subunits            int             `json:"subunits"`
	MinConfirmations    int             `json:"min_confirmations"`
	DepositEnabled      bool            `json:"deposit_enabled"`
	DepositFee          decimal.Decimal `json:"deposit_fee"`
	DepositFeeRatio     decimal.Decimal `json:"deposit_fee_ratio"`
	MinDepositAmount    decimal.Decimal `json:"min_deposit_amount"`
	MinCollectionAmount decimal.Decimal `json:"min_collection_amount"`
	WithdrawEnabled     bool            `json:"withdraw_enabled"`
	WithdrawFee         decimal.Decimal `json:"withdraw_fee"`
	WithdrawFeeRatio    decimal.Decimal `json:"withdraw_fee_ratio"`
	MinWithdrawAmount   decimal.Decimal `json:"min_withdraw_amount"`
	UseParentFee        bool            `json:"use_parent_fee"`

	ExplorerAddress     string `json:"explorer_address"`
	ExplorerTransaction string `json:"explorer_transaction"`

	Options       map[string]any `json:"options"`
	SystemOptions map[string]any `json:"system_options"`
	CreatedAt     Timestamp      `json:"created_at"`
	UpdatedAt     Timestamp      `json:"updated_at"`
}

type PricePoint struct {
	Time  Timestamp       `json:"timestamp"`
	Price decimal.Decimal `json:"price"`
}

type MarketCapSnapshot struct {
	Time        Timestamp       `json:"timestamp"`
	MarketCap   decimal.Decimal `json:"market_cap"`
	Volume      decimal.Decimal `json:"volume"`
	BTCValue    decimal.Decimal `json:"btc_value"`
	ETHValue    decimal.Decimal `json:"eth_value"`
	StableValue decimal.Decimal `json:"stable_value"`
	OtherValue  decimal.Decimal `json:"other_value"`
}

type Market struct {
	ID     string `json:"id"`
	Symbol string `json:"symbol"`
	Name   string `json:"name"`

	BaseUnit  string `json:"base_unit"`
	QuoteUnit string `json:"quote_unit"`
	State     string `json:"state"`
	Position  int    `json:"position"`

	AmountPrecision int             `json:"amount_precision"`
	PricePrecision  int             `json:"price_precision"`
	TotalPrecision  int             `json:"total_precision"`
	MinAmount       decimal.Decimal `json:"min_amount"`
	MinPrice        decimal.Decimal `json:"min_price"`
	MaxPrice        decimal.Decimal `json:"max_price"`
	CreatedAt       Timestamp       `json:"created_at"`
	UpdatedAt       Timestamp       `json:"updated_at"`
}

type Ticker struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	BaseUnit  string          `json:"base_unit"`
	QuoteUnit string          `json:"quote_unit"`
	Open      decimal.Decimal `json:"open"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Last      decimal.Decimal `json:"last"`
	AvgPrice  decimal.Decimal `json:"avg_price"`
	Amount    decimal.Decimal `json:"amount"`
	Volume    decimal.Decimal `json:"volume"`

	PriceChangePercent string `json:"price_change_percent"`
}

type Depth struct {
	Asks []PriceLevel `json:"asks"`
	Bids []PriceLevel `json:"bids"`

	Sequence int64 `json:"sequence"`
}

type PriceLevel struct {
	Price  decimal.Decimal
	Amount decimal.Decimal
}

func (l *PriceLevel) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("safetrade: price level must be a [price, amount] array: %w", err)
	}
	if len(fields) < 2 {
		return fmt.Errorf("safetrade: price level must be a [price, amount] array, got %d elements", len(fields))
	}
	var level PriceLevel
	if err := decodeDecimal(fields[0], &level.Price, "price level price"); err != nil {
		return err
	}
	if err := decodeDecimal(fields[1], &level.Amount, "price level amount"); err != nil {
		return err
	}
	*l = level
	return nil
}

func (l PriceLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]decimal.Decimal{l.Price, l.Amount})
}

type KLine struct {
	Time   Timestamp
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
}

const klineFields = 6

func (k *KLine) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("safetrade: k-line must be a [time, open, high, low, close, volume] array: %w", err)
	}
	if len(fields) < klineFields {
		return fmt.Errorf("safetrade: k-line must have %d elements, got %d", klineFields, len(fields))
	}
	var kline KLine
	if err := json.Unmarshal(fields[0], &kline.Time); err != nil {
		return fmt.Errorf("safetrade: k-line time: %w", err)
	}
	prices := []struct {
		target *decimal.Decimal
		name   string
	}{
		{&kline.Open, "open"},
		{&kline.High, "high"},
		{&kline.Low, "low"},
		{&kline.Close, "close"},
		{&kline.Volume, "volume"},
	}
	for i, price := range prices {
		if err := decodeDecimal(fields[i+1], price.target, "k-line "+price.name); err != nil {
			return err
		}
	}
	*k = kline
	return nil
}

func (k KLine) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{k.Time.Unix(), k.Open, k.High, k.Low, k.Close, k.Volume})
}

type PublicTrade struct {
	ID     int64           `json:"id"`
	Market string          `json:"market"`
	Side   Side            `json:"side"`
	Price  decimal.Decimal `json:"price"`
	Amount decimal.Decimal `json:"amount"`

	Total     decimal.Decimal `json:"total"`
	CreatedAt Timestamp       `json:"created_at"`
}

type OrderDistribution struct {
	BuyCount  int64 `json:"number_of_buy"`
	SellCount int64 `json:"number_of_sell"`
}

type TradeDistribution struct {
	TakerBuyCount   int64           `json:"taker_buy"`
	TakerBuyVolume  decimal.Decimal `json:"taker_buy_volume"`
	TakerSellCount  int64           `json:"taker_sell"`
	TakerSellVolume decimal.Decimal `json:"taker_sell_volume"`
}

type TradingFee struct {
	ID       int64  `json:"id"`
	Group    string `json:"group"`
	MarketID string `json:"market_id"`

	Maker decimal.Decimal `json:"maker"`
	Taker decimal.Decimal `json:"taker"`
}

func decodeDecimal(data json.RawMessage, target *decimal.Decimal, name string) error {
	if err := target.UnmarshalJSON(data); err != nil {
		return fmt.Errorf("safetrade: %s: %w", name, err)
	}
	return nil
}
