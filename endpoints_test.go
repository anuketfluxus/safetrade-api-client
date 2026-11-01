package safetrade_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/anuketfluxus/safetrade-api-client"
)

type endpointCase struct {
	name string
	call func(ctx context.Context, c *safetrade.Client) (any, error)

	method  string
	path    string
	query   url.Values
	body    string
	private bool

	status   int
	response string
	check    func(t *testing.T, result any)
}

var (
	timeFrom = time.Unix(1759276800, 0)
	timeTo   = time.Unix(1761868800, 0)
)

func dec(t *testing.T, value string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(value)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q) error = %v", value, err)
	}
	return d
}

func expectDecimal(t *testing.T, field string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(dec(t, want)) {
		t.Errorf("%s = %s, want %s", field, got, want)
	}
}

func expectTime(t *testing.T, field string, got safetrade.Timestamp, want string) {
	t.Helper()
	wantTime, err := time.Parse(time.RFC3339Nano, want)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", want, err)
	}
	if !got.Equal(wantTime) {
		t.Errorf("%s = %v, want %v", field, got.Time, wantTime)
	}
}

func expectEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func publicEndpointCases() []endpointCase {
	return []endpointCase{
		{
			name: "ListCurrencies",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListCurrencies(ctx)
			},
			method: http.MethodGet,
			path:   "/trade/public/currencies",
			response: `[{"id":"usdt","name":"Tether","description":"Stablecoin","homepage":"https://tether.to",
				"icon_url":"https://cdn.example/usdt.png","status":"enabled","precision":6,"position":1,
				"price":"1.0001","default_network_id":7,
				"networks":[{"id":7,"currency_id":"usdt","blockchain_key":"tron-mainnet","parent_id":"trx",
					"protocol":"trc20","protocol_name":"TRC20","client":"tron","status":"active","subunits":6,
					"min_confirmations":20,"deposit_enabled":true,"deposit_fee":"0","deposit_fee_ratio":0,
					"min_deposit_amount":"1","min_collection_amount":5,"withdraw_enabled":true,
					"withdraw_fee":"1.5","withdraw_fee_ratio":"0.001","min_withdraw_amount":"10",
					"use_parent_fee":false,"explorer_address":"https://tronscan.org/#/address/#{address}",
					"explorer_transaction":"https://tronscan.org/#/transaction/#{txid}",
					"options":{"contract":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"},"system_options":{},
					"created_at":"2024-01-02T03:04:05Z","updated_at":"2026-10-01T00:00:00.123456Z"}],
				"created_at":"2024-01-02T03:04:05Z","updated_at":"2026-10-01T00:00:00Z"}]`,
			check: func(t *testing.T, result any) {
				currencies := result.([]safetrade.Currency)
				if len(currencies) != 1 {
					t.Fatalf("got %d currencies, want 1", len(currencies))
				}
				usdt := currencies[0]
				expectEqual(t, "ID", usdt.ID, "usdt")
				expectEqual(t, "Precision", usdt.Precision, 6)
				expectDecimal(t, "Price", usdt.Price, "1.0001")
				expectEqual(t, "DefaultNetworkID", usdt.DefaultNetworkID, 7)
				expectTime(t, "CreatedAt", usdt.CreatedAt, "2024-01-02T03:04:05Z")
				if len(usdt.Networks) != 1 {
					t.Fatalf("got %d networks, want 1", len(usdt.Networks))
				}
				network := usdt.Networks[0]
				expectEqual(t, "BlockchainKey", network.BlockchainKey, "tron-mainnet")
				expectEqual(t, "MinConfirmations", network.MinConfirmations, 20)
				expectEqual(t, "DepositEnabled", network.DepositEnabled, true)
				expectDecimal(t, "MinCollectionAmount", network.MinCollectionAmount, "5")
				expectDecimal(t, "WithdrawFee", network.WithdrawFee, "1.5")
				expectDecimal(t, "WithdrawFeeRatio", network.WithdrawFeeRatio, "0.001")
				expectEqual(t, "Options[contract]", network.Options["contract"], any("TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"))
				expectTime(t, "UpdatedAt", network.UpdatedAt, "2026-10-01T00:00:00.123456Z")
			},
		},
		{
			name: "GetCurrency",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetCurrency(ctx, "usdt")
			},
			method:   http.MethodGet,
			path:     "/trade/public/currencies/usdt",
			response: `{"id":"usdt","name":"Tether","precision":6,"price":1.0001,"networks":[]}`,
			check: func(t *testing.T, result any) {
				currency := result.(safetrade.Currency)
				expectEqual(t, "Name", currency.Name, "Tether")
				expectDecimal(t, "Price", currency.Price, "1.0001")
			},
		},
		{
			name: "GetPriceHistory",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetPriceHistory(ctx, "btc", safetrade.PriceHistoryWeek)
			},
			method:   http.MethodGet,
			path:     "/trade/public/currencies/btc/price-history",
			query:    url.Values{"period": {"7d"}},
			response: `[{"price":"67000.5","timestamp":1761436800},{"price":67100,"timestamp":1761440400000}]`,
			check: func(t *testing.T, result any) {
				points := result.([]safetrade.PricePoint)
				if len(points) != 2 {
					t.Fatalf("got %d points, want 2", len(points))
				}
				expectDecimal(t, "points[0].Price", points[0].Price, "67000.5")
				expectTime(t, "points[0].Time", points[0].Time, "2025-10-26T00:00:00Z")
				expectDecimal(t, "points[1].Price", points[1].Price, "67100")
				expectTime(t, "points[1].Time", points[1].Time, "2025-10-26T01:00:00Z")
			},
		},
		{
			name: "GetMarketCapHistory",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetMarketCapHistory(ctx, safetrade.MarketCapHistoryParams{TimeFrom: timeFrom, TimeTo: timeTo})
			},
			method: http.MethodGet,
			path:   "/trade/public/marketcap-historical",
			query:  url.Values{"time_from": {"1759276800"}, "time_to": {"1761868800"}},
			response: `[{"timestamp":"2025-10-25T23:59:59.999Z","market_cap":"3900000000000","volume":"98000000000",
				"btc_value":"2300000000000","eth_value":480000000000,"stable_value":"300000000000","other_value":"820000000000"}]`,
			check: func(t *testing.T, result any) {
				snapshots := result.([]safetrade.MarketCapSnapshot)
				if len(snapshots) != 1 {
					t.Fatalf("got %d snapshots, want 1", len(snapshots))
				}
				expectTime(t, "Time", snapshots[0].Time, "2025-10-25T23:59:59.999Z")
				expectDecimal(t, "MarketCap", snapshots[0].MarketCap, "3900000000000")
				expectDecimal(t, "ETHValue", snapshots[0].ETHValue, "480000000000")
			},
		},
		{
			name: "ListMarkets",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListMarkets(ctx)
			},
			method: http.MethodGet,
			path:   "/trade/public/markets",
			response: `[{"id":"btcusdt","symbol":"BTC/USDT","name":"BTC/USDT","base_unit":"btc","quote_unit":"usdt",
				"state":"enabled","position":1,"amount_precision":6,"price_precision":2,"total_precision":8,
				"min_amount":"0.0001","min_price":"0.01","max_price":"0","created_at":"2024-01-02T03:04:05Z",
				"updated_at":"2026-10-01T00:00:00Z"}]`,
			check: func(t *testing.T, result any) {
				markets := result.([]safetrade.Market)
				if len(markets) != 1 {
					t.Fatalf("got %d markets, want 1", len(markets))
				}
				expectEqual(t, "BaseUnit", markets[0].BaseUnit, "btc")
				expectEqual(t, "QuoteUnit", markets[0].QuoteUnit, "usdt")
				expectEqual(t, "AmountPrecision", markets[0].AmountPrecision, 6)
				expectDecimal(t, "MinAmount", markets[0].MinAmount, "0.0001")
			},
		},
		{
			name: "GetMarket",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetMarket(ctx, "btcusdt")
			},
			method:   http.MethodGet,
			path:     "/trade/public/markets/btcusdt",
			response: `{"id":"btcusdt","price_precision":2}`,
			check: func(t *testing.T, result any) {
				expectEqual(t, "PricePrecision", result.(safetrade.Market).PricePrecision, 2)
			},
		},
		{
			name: "GetMarket escapes the market ID",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetMarket(ctx, "we/ird id?")
			},
			method:   http.MethodGet,
			path:     "/trade/public/markets/we%2Fird%20id%3F",
			response: `{"id":"we/ird id?"}`,
			check: func(t *testing.T, result any) {
				expectEqual(t, "ID", result.(safetrade.Market).ID, "we/ird id?")
			},
		},
		{
			name: "GetDepth",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetDepth(ctx, "btcusdt", 2)
			},
			method:   http.MethodGet,
			path:     "/trade/public/markets/btcusdt/depth",
			query:    url.Values{"limit": {"2"}},
			response: `{"asks":[["67010.5","0.25"],[67020,1.5]],"bids":[["67000","0.4"]],"sequence":123456}`,
			check: func(t *testing.T, result any) {
				depth := result.(safetrade.Depth)
				if len(depth.Asks) != 2 || len(depth.Bids) != 1 {
					t.Fatalf("got %d asks and %d bids, want 2 and 1", len(depth.Asks), len(depth.Bids))
				}
				expectDecimal(t, "Asks[0].Price", depth.Asks[0].Price, "67010.5")
				expectDecimal(t, "Asks[0].Amount", depth.Asks[0].Amount, "0.25")
				expectDecimal(t, "Asks[1].Price", depth.Asks[1].Price, "67020")
				expectDecimal(t, "Asks[1].Amount", depth.Asks[1].Amount, "1.5")
				expectDecimal(t, "Bids[0].Price", depth.Bids[0].Price, "67000")
				expectEqual(t, "Sequence", depth.Sequence, 123456)
			},
		},
		{
			name: "GetDepth without limit",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetDepth(ctx, "btcusdt", 0)
			},
			method:   http.MethodGet,
			path:     "/trade/public/markets/btcusdt/depth",
			response: `{"asks":[],"bids":[],"sequence":1}`,
			check:    func(*testing.T, any) {},
		},
		{
			name: "ListKLines",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListKLines(ctx, "btcusdt", safetrade.KLineParams{
					Period: 60, TimeFrom: timeFrom, TimeTo: timeTo, Page: 2, Limit: 3,
				})
			},
			method: http.MethodGet,
			path:   "/trade/public/markets/btcusdt/k-line",
			query: url.Values{
				"period": {"60"}, "time_from": {"1759276800"}, "time_to": {"1761868800"}, "page": {"2"}, "limit": {"3"},
			},
			response: `[[1761436800,"67000","67100","66900","67050","12.5"],[1761440400,67050,67200,67000,67150,8]]`,
			check: func(t *testing.T, result any) {
				klines := result.([]safetrade.KLine)
				if len(klines) != 2 {
					t.Fatalf("got %d k-lines, want 2", len(klines))
				}
				expectTime(t, "klines[0].Time", klines[0].Time, "2025-10-26T00:00:00Z")
				expectDecimal(t, "klines[0].Open", klines[0].Open, "67000")
				expectDecimal(t, "klines[0].High", klines[0].High, "67100")
				expectDecimal(t, "klines[0].Low", klines[0].Low, "66900")
				expectDecimal(t, "klines[0].Close", klines[0].Close, "67050")
				expectDecimal(t, "klines[0].Volume", klines[0].Volume, "12.5")
				expectDecimal(t, "klines[1].Close", klines[1].Close, "67150")
				expectDecimal(t, "klines[1].Volume", klines[1].Volume, "8")
			},
		},
		{
			name: "ListMarketTrades",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListMarketTrades(ctx, "btcusdt", safetrade.ListMarketTradesParams{Page: 1, Limit: 2})
			},
			method: http.MethodGet,
			path:   "/trade/public/markets/btcusdt/trades",
			query:  url.Values{"page": {"1"}, "limit": {"2"}},
			response: `[{"id":991,"market":"btcusdt","side":"buy","price":"67000","amount":"0.01","total":"670",
				"created_at":"2026-10-26T18:00:00Z"}]`,
			check: func(t *testing.T, result any) {
				trades := result.([]safetrade.PublicTrade)
				if len(trades) != 1 {
					t.Fatalf("got %d trades, want 1", len(trades))
				}
				expectEqual(t, "ID", trades[0].ID, 991)
				expectEqual(t, "Side", trades[0].Side, safetrade.SideBuy)
				expectDecimal(t, "Total", trades[0].Total, "670")
				expectTime(t, "CreatedAt", trades[0].CreatedAt, "2026-10-26T18:00:00Z")
			},
		},
		{
			name: "GetOrderDistribution",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetOrderDistribution(ctx, safetrade.DistributionPeriod6h)
			},
			method:   http.MethodGet,
			path:     "/trade/public/order-distribution",
			query:    url.Values{"period": {"6h"}},
			response: `{"number_of_buy":120,"number_of_sell":98}`,
			check: func(t *testing.T, result any) {
				distribution := result.(safetrade.OrderDistribution)
				expectEqual(t, "BuyCount", distribution.BuyCount, 120)
				expectEqual(t, "SellCount", distribution.SellCount, 98)
			},
		},
		{
			name: "ListTickers",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListTickers(ctx, safetrade.TickerPeriod48h)
			},
			method: http.MethodGet,
			path:   "/trade/public/tickers",
			query:  url.Values{"period": {"48h"}},
			response: `{"btcusdt":{"id":"btcusdt","name":"BTC/USDT","base_unit":"btc","quote_unit":"usdt",
					"open":"66000","high":"67500","low":"65800","last":"67050.25","avg_price":"66800",
					"amount":"152.4","volume":"10180320.5","price_change_percent":"+1.59%"},
				"xtmusdt":{"id":"xtmusdt","open":0.44,"high":0.47,"low":0.43,"last":0.45,"avg_price":0.451,
					"amount":125000,"volume":56375,"price_change_percent":"+2.27%"}}`,
			check: func(t *testing.T, result any) {
				tickers := result.(map[string]safetrade.Ticker)
				if len(tickers) != 2 {
					t.Fatalf("got %d tickers, want 2", len(tickers))
				}
				btc := tickers["btcusdt"]
				expectDecimal(t, "btcusdt.Last", btc.Last, "67050.25")
				expectEqual(t, "btcusdt.PriceChangePercent", btc.PriceChangePercent, "+1.59%")
				expectDecimal(t, "xtmusdt.Last", tickers["xtmusdt"].Last, "0.45")
				expectDecimal(t, "xtmusdt.Amount", tickers["xtmusdt"].Amount, "125000")
			},
		},
		{
			name: "ListTickers with default period",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListTickers(ctx, "")
			},
			method:   http.MethodGet,
			path:     "/trade/public/tickers",
			response: `{}`,
			check:    func(*testing.T, any) {},
		},
		{
			name: "GetTicker",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetTicker(ctx, "btcusdt")
			},
			method:   http.MethodGet,
			path:     "/trade/public/tickers/btcusdt",
			response: `{"id":"btcusdt","last":"67050.25","high":67500}`,
			check: func(t *testing.T, result any) {
				ticker := result.(safetrade.Ticker)
				expectDecimal(t, "Last", ticker.Last, "67050.25")
				expectDecimal(t, "High", ticker.High, "67500")
			},
		},
		{
			name: "GetTradeDistribution",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetTradeDistribution(ctx, safetrade.DistributionPeriod24h, "btcusdt")
			},
			method:   http.MethodGet,
			path:     "/trade/public/trade-distribution",
			query:    url.Values{"period": {"24h"}, "market": {"btcusdt"}},
			response: `{"taker_buy":40,"taker_buy_volume":"125000.5","taker_sell":35,"taker_sell_volume":98000}`,
			check: func(t *testing.T, result any) {
				distribution := result.(safetrade.TradeDistribution)
				expectEqual(t, "TakerBuyCount", distribution.TakerBuyCount, 40)
				expectDecimal(t, "TakerBuyVolume", distribution.TakerBuyVolume, "125000.5")
				expectEqual(t, "TakerSellCount", distribution.TakerSellCount, 35)
				expectDecimal(t, "TakerSellVolume", distribution.TakerSellVolume, "98000")
			},
		},
		{
			name: "GetTradeDistribution exchange-wide",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.GetTradeDistribution(ctx, safetrade.DistributionPeriod12h, "")
			},
			method:   http.MethodGet,
			path:     "/trade/public/trade-distribution",
			query:    url.Values{"period": {"12h"}},
			response: `{"taker_buy":1,"taker_buy_volume":"1","taker_sell":2,"taker_sell_volume":"2"}`,
			check:    func(*testing.T, any) {},
		},
		{
			name: "ListTradingFees",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Public.ListTradingFees(ctx, safetrade.ListTradingFeesParams{
					OrderBy: "id", Ordering: safetrade.OrderingDesc, Page: 1, Limit: 50,
				})
			},
			method:   http.MethodGet,
			path:     "/trade/public/trading_fees",
			query:    url.Values{"order_by": {"id"}, "ordering": {"desc"}, "page": {"1"}, "limit": {"50"}},
			response: `[{"id":1,"group":"vip-1","market_id":"any","maker":"0.001","taker":0.002}]`,
			check: func(t *testing.T, result any) {
				fees := result.([]safetrade.TradingFee)
				if len(fees) != 1 {
					t.Fatalf("got %d fees, want 1", len(fees))
				}
				expectEqual(t, "Group", fees[0].Group, "vip-1")
				expectDecimal(t, "Maker", fees[0].Maker, "0.001")
				expectDecimal(t, "Taker", fees[0].Taker, "0.002")
			},
		},
	}
}

func accountEndpointCases() []endpointCase {
	return []endpointCase{
		{
			name: "GetMember",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.GetMember(ctx)
			},
			method:  http.MethodGet,
			path:    "/trade/account/members/me",
			private: true,
			response: `{"id":12,"uid":"ID1234567890","email":"trader@example.com","username":"trader","level":3,
				"role":"member","group":"vip-1","state":"active","withdraw_password_enabled":true,
				"created_at":"2024-05-06T07:08:09Z","updated_at":"2026-10-20T10:00:00Z"}`,
			check: func(t *testing.T, result any) {
				member := result.(safetrade.Member)
				expectEqual(t, "UID", member.UID, "ID1234567890")
				expectEqual(t, "Email", member.Email, "trader@example.com")
				expectEqual(t, "Level", member.Level, 3)
				expectEqual(t, "WithdrawPasswordEnabled", member.WithdrawPasswordEnabled, true)
				expectTime(t, "CreatedAt", member.CreatedAt, "2024-05-06T07:08:09Z")
			},
		},
		{
			name: "ListSpotBalances",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.ListSpotBalances(ctx)
			},
			method:  http.MethodGet,
			path:    "/trade/account/balances/spot",
			private: true,
			response: `[{"currency":"usdt","type":"spot","balance":"1520.75","locked":20,
				"deposit_addresses":[{"address":"TXYZ","network":"tron-mainnet","parent_address":"",
					"currencies":["usdt","trx"]}]}]`,
			check: func(t *testing.T, result any) {
				balances := result.([]safetrade.Balance)
				if len(balances) != 1 {
					t.Fatalf("got %d balances, want 1", len(balances))
				}
				expectEqual(t, "Type", balances[0].Type, safetrade.AccountTypeSpot)
				expectDecimal(t, "Balance", balances[0].Balance, "1520.75")
				expectDecimal(t, "Locked", balances[0].Locked, "20")
				if len(balances[0].DepositAddresses) != 1 {
					t.Fatalf("got %d deposit addresses, want 1", len(balances[0].DepositAddresses))
				}
				address := balances[0].DepositAddresses[0]
				expectEqual(t, "Address", address.Address, "TXYZ")
				if !reflect.DeepEqual(address.Currencies, []string{"usdt", "trx"}) {
					t.Errorf("Currencies = %v, want [usdt trx]", address.Currencies)
				}
			},
		},
		{
			name: "ListBeneficiaries",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.ListBeneficiaries(ctx, safetrade.ListBeneficiariesParams{
					State: safetrade.BeneficiaryStateActive, BlockchainKey: "tron-mainnet", CurrencyID: "usdt", Page: 1, Limit: 10,
				})
			},
			method: http.MethodGet,
			path:   "/trade/account/beneficiaries",
			query: url.Values{
				"state": {"active"}, "blockchain_key": {"tron-mainnet"}, "currency_id": {"usdt"}, "page": {"1"}, "limit": {"10"},
			},
			private: true,
			response: `[{"id":5,"currency_id":"usdt","blockchain_key":"tron-mainnet","address":"TXYZ","label":"cold wallet",
				"description":"","state":"active","options":{"memo":"42"},"created_at":"2025-01-01T00:00:00Z",
				"updated_at":"2025-01-01T00:00:00Z"}]`,
			check: func(t *testing.T, result any) {
				beneficiaries := result.([]safetrade.Beneficiary)
				if len(beneficiaries) != 1 {
					t.Fatalf("got %d beneficiaries, want 1", len(beneficiaries))
				}
				expectEqual(t, "Label", beneficiaries[0].Label, "cold wallet")
				expectEqual(t, "State", beneficiaries[0].State, safetrade.BeneficiaryStateActive)
				expectEqual(t, "Options[memo]", beneficiaries[0].Options["memo"], any("42"))
			},
		},
		{
			name: "GetDepositAddress",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.GetDepositAddress(ctx, "usdt", "tron-mainnet")
			},
			method:   http.MethodGet,
			path:     "/trade/account/deposit_address/usdt",
			query:    url.Values{"network": {"tron-mainnet"}},
			private:  true,
			response: `{"address":"TXYZ","network":"tron-mainnet","parent_address":"","currencies":["usdt"]}`,
			check: func(t *testing.T, result any) {
				address := result.(safetrade.DepositAddress)
				expectEqual(t, "Address", address.Address, "TXYZ")
				expectEqual(t, "Network", address.Network, "tron-mainnet")
			},
		},
		{
			name: "ListDeposits",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.ListDeposits(ctx, safetrade.ListDepositsParams{
					Currency: "usdt", Status: safetrade.DepositStatusCollected, BlockchainKey: "tron-mainnet",
					TimeFrom: timeFrom, TimeTo: timeTo, OrderBy: "created_at", Ordering: safetrade.OrderingAsc,
					Page: 3, Limit: 25,
				})
			},
			method: http.MethodGet,
			path:   "/trade/account/deposits",
			query: url.Values{
				"currency": {"usdt"}, "status": {"collected"}, "blockchain_key": {"tron-mainnet"},
				"time_from": {"1759276800"}, "time_to": {"1761868800"}, "order_by": {"created_at"},
				"ordering": {"asc"}, "page": {"3"}, "limit": {"25"},
			},
			private: true,
			response: `[{"id":77,"currency":"usdt","blockchain_key":"tron-mainnet","amount":"500","fee":"0",
				"fee_currency":"usdt","fee_paid":true,"credited":true,"from_address":"TABC","txid":"0xabc",
				"status":"collected","options":{},"created_at":"2025-10-02T00:00:00Z",
				"updated_at":"2025-10-02T00:10:00Z","completed_at":null}]`,
			check: func(t *testing.T, result any) {
				deposits := result.([]safetrade.Deposit)
				if len(deposits) != 1 {
					t.Fatalf("got %d deposits, want 1", len(deposits))
				}
				expectEqual(t, "TxID", deposits[0].TxID, "0xabc")
				expectEqual(t, "Status", deposits[0].Status, safetrade.DepositStatusCollected)
				expectEqual(t, "Credited", deposits[0].Credited, true)
				expectDecimal(t, "Amount", deposits[0].Amount, "500")
				if !deposits[0].CompletedAt.IsZero() {
					t.Errorf("CompletedAt = %v, want zero for null", deposits[0].CompletedAt.Time)
				}
			},
		},
		{
			name: "GetDeposit",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.GetDeposit(ctx, "0xabc")
			},
			method:   http.MethodGet,
			path:     "/trade/account/deposits/0xabc",
			private:  true,
			response: `{"id":77,"txid":"0xabc","amount":500.25,"completed_at":"2025-10-02T00:10:00Z"}`,
			check: func(t *testing.T, result any) {
				deposit := result.(safetrade.Deposit)
				expectDecimal(t, "Amount", deposit.Amount, "500.25")
				expectTime(t, "CompletedAt", deposit.CompletedAt, "2025-10-02T00:10:00Z")
			},
		},
		{
			name: "ListWithdrawals",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.ListWithdrawals(ctx, safetrade.ListWithdrawalsParams{
					Currency: "usdt", Type: safetrade.WithdrawalTypeOnchain, Status: safetrade.WithdrawalStatusSucceed,
					BlockchainKey: "tron-mainnet", RID: "TXYZ", TimeFrom: timeFrom, TimeTo: timeTo,
					OrderBy: "id", Ordering: safetrade.OrderingDesc, Page: 1, Limit: 5,
				})
			},
			method: http.MethodGet,
			path:   "/trade/account/withdraws",
			query: url.Values{
				"currency": {"usdt"}, "type": {"onchain"}, "status": {"succeed"}, "blockchain_key": {"tron-mainnet"},
				"rid": {"TXYZ"}, "time_from": {"1759276800"}, "time_to": {"1761868800"}, "order_by": {"id"},
				"ordering": {"desc"}, "page": {"1"}, "limit": {"5"},
			},
			private: true,
			response: `[{"id":9,"type":"onchain","currency_id":"usdt","blockchain_key":"tron-mainnet","beneficiary_id":5,
				"rid":"TXYZ","tid":"TID123","amount":"100","fee":"1.5","fee_currency_id":"usdt","sum":"101.5",
				"total":"98.5","txid":"0xdef","block_number":71234567,"status":"succeed","options":{},
				"created_at":"2025-10-03T00:00:00Z","updated_at":"2025-10-03T00:05:00Z",
				"completed_at":"2025-10-03T00:05:00Z"}]`,
			check: func(t *testing.T, result any) {
				withdrawals := result.([]safetrade.Withdrawal)
				if len(withdrawals) != 1 {
					t.Fatalf("got %d withdrawals, want 1", len(withdrawals))
				}
				withdrawal := withdrawals[0]
				expectEqual(t, "Type", withdrawal.Type, safetrade.WithdrawalTypeOnchain)
				expectEqual(t, "Status", withdrawal.Status, safetrade.WithdrawalStatusSucceed)
				expectEqual(t, "BlockNumber", withdrawal.BlockNumber, 71234567)
				expectDecimal(t, "Sum", withdrawal.Sum, "101.5")
				expectDecimal(t, "Total", withdrawal.Total, "98.5")
			},
		},
		{
			name: "CreateWithdrawal",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.CreateWithdrawal(ctx, safetrade.CreateWithdrawalParams{
					Currency: "usdt", Amount: decimal.RequireFromString("25.5"), BlockchainKey: "tron-mainnet",
					Address: "TXYZ", EmailCode: "123456", OTPCode: "654321", Options: map[string]any{"memo": "42"},
				})
			},
			method:   http.MethodPost,
			path:     "/trade/account/withdraws",
			body:     `{"currency":"usdt","amount":"25.5","blockchain_key":"tron-mainnet","address":"TXYZ","email_code":"123456","otp_code":"654321","options":{"memo":"42"}}`,
			private:  true,
			status:   http.StatusCreated,
			response: `{"id":10,"currency_id":"usdt","amount":"25.5","status":"prepared","type":"onchain"}`,
			check: func(t *testing.T, result any) {
				withdrawal := result.(safetrade.Withdrawal)
				expectEqual(t, "ID", withdrawal.ID, 10)
				expectEqual(t, "Status", withdrawal.Status, safetrade.WithdrawalStatusPrepared)
			},
		},
		{
			name: "CreateWithdrawal to a beneficiary",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Account.CreateWithdrawal(ctx, safetrade.CreateWithdrawalParams{
					Currency: "usdt", Amount: decimal.RequireFromString("10"), BeneficiaryID: 5,
					WithdrawPassword: "pa55",
				})
			},
			method:   http.MethodPost,
			path:     "/trade/account/withdraws",
			body:     `{"currency":"usdt","amount":"10","beneficiary_id":5,"withdraw_password":"pa55"}`,
			private:  true,
			status:   http.StatusCreated,
			response: `{"id":11,"beneficiary_id":5}`,
			check: func(t *testing.T, result any) {
				expectEqual(t, "BeneficiaryID", result.(safetrade.Withdrawal).BeneficiaryID, 5)
			},
		},
		{
			name: "GenerateWithdrawalCode",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return nil, c.Account.GenerateWithdrawalCode(ctx, safetrade.GenerateWithdrawalCodeParams{
					Currency: "usdt", BlockchainKey: "tron-mainnet", Address: "TXYZ",
					Amount: decimal.RequireFromString("25.5"), Channel: safetrade.CodeChannelEmail,
				})
			},
			method:   http.MethodPost,
			path:     "/trade/account/withdraws/generate_code",
			body:     `{"currency":"usdt","blockchain_key":"tron-mainnet","address":"TXYZ","amount":"25.5","type":"email"}`,
			private:  true,
			status:   http.StatusCreated,
			response: `{"status":201}`,
			check:    func(*testing.T, any) {},
		},
	}
}

func marketEndpointCases() []endpointCase {
	const order = `{"id":42,"market":"xtmusdt","side":"sell","type":"limit","state":"wait","ask":"xtm","bid":"usdt",
		"price":"0.45","avg_price":"0","stop_price":null,"max_price":null,"origin_amount":"12.5",
		"filled_amount":"0","maker_fee":"0.001","taker_fee":"0.002","trades_count":0,
		"created_at":"2026-10-26T18:11:41Z","updated_at":"2026-10-26T18:11:41Z","triggered_at":null}`
	checkOrder := func(t *testing.T, result any) {
		order := result.(safetrade.Order)
		expectEqual(t, "ID", order.ID, 42)
		expectEqual(t, "Side", order.Side, safetrade.SideSell)
		expectEqual(t, "Type", order.Type, safetrade.OrderTypeLimit)
		expectEqual(t, "State", order.State, safetrade.OrderStateWait)
		expectEqual(t, "Ask", order.Ask, "xtm")
		expectDecimal(t, "Price", order.Price, "0.45")
		expectDecimal(t, "OriginAmount", order.OriginAmount, "12.5")
		expectDecimal(t, "StopPrice", order.StopPrice, "0")
		if !order.TriggeredAt.IsZero() {
			t.Errorf("TriggeredAt = %v, want zero", order.TriggeredAt.Time)
		}
	}
	const trade = `{"id":7,"market":"btcusdt","order_id":42,"side":"buy","order_side":"buy","price":"67000",
		"amount":"0.01","total":"670","fee":"0.00001","fee_currency":"btc","created_at":"2026-10-26T18:00:00Z"}`
	checkTrade := func(t *testing.T, result any) {
		trade := result.(safetrade.Trade)
		expectEqual(t, "ID", trade.ID, 7)
		expectEqual(t, "OrderID", trade.OrderID, 42)
		expectEqual(t, "OrderSide", trade.OrderSide, safetrade.SideBuy)
		expectDecimal(t, "Fee", trade.Fee, "0.00001")
		expectEqual(t, "FeeCurrency", trade.FeeCurrency, "btc")
	}

	return []endpointCase{
		{
			name: "ListOrders",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.ListOrders(ctx, safetrade.ListOrdersParams{
					Market:    "xtmusdt",
					States:    []safetrade.OrderState{safetrade.OrderStateWait, safetrade.OrderStateDone},
					Types:     []safetrade.OrderType{safetrade.OrderTypeLimit, safetrade.OrderTypeStopLimit},
					Sides:     []safetrade.Side{safetrade.SideSell},
					BaseUnit:  "xtm",
					QuoteUnit: "usdt",
					TimeFrom:  timeFrom,
					TimeTo:    timeTo,
					OrderBy:   "id",
					Ordering:  safetrade.OrderingDesc,
					Page:      1,
					Limit:     100,
				})
			},
			method: http.MethodGet,
			path:   "/trade/market/orders",
			query: url.Values{
				"market": {"xtmusdt"}, "state": {"wait,done"}, "type": {"limit,stop_limit"}, "side": {"sell"},
				"base_unit": {"xtm"}, "quote_unit": {"usdt"}, "time_from": {"1759276800"}, "time_to": {"1761868800"},
				"order_by": {"id"}, "ordering": {"desc"}, "page": {"1"}, "limit": {"100"},
			},
			private:  true,
			response: "[" + order + "]",
			check: func(t *testing.T, result any) {
				orders := result.([]safetrade.Order)
				if len(orders) != 1 {
					t.Fatalf("got %d orders, want 1", len(orders))
				}
				checkOrder(t, orders[0])
			},
		},
		{
			name: "ListOrders without filters",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.ListOrders(ctx, safetrade.ListOrdersParams{})
			},
			method:   http.MethodGet,
			path:     "/trade/market/orders",
			private:  true,
			response: `[]`,
			check: func(t *testing.T, result any) {
				if orders := result.([]safetrade.Order); len(orders) != 0 {
					t.Errorf("got %d orders, want 0", len(orders))
				}
			},
		},
		{
			name: "GetOrder",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.GetOrder(ctx, 42)
			},
			method:   http.MethodGet,
			path:     "/trade/market/orders/42",
			private:  true,
			response: order,
			check:    checkOrder,
		},
		{
			name: "CreateOrder limit",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.CreateOrder(ctx, safetrade.CreateOrderParams{
					Market: "xtmusdt", Side: safetrade.SideSell, Type: safetrade.OrderTypeLimit,
					Amount: decimal.RequireFromString("12.5"), Price: decimal.RequireFromString("0.45"),
				})
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders",
			body:     `{"market":"xtmusdt","side":"sell","type":"limit","amount":"12.5","price":"0.45"}`,
			private:  true,
			status:   http.StatusCreated,
			response: order,
			check:    checkOrder,
		},
		{
			name: "CreateOrder market",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.CreateOrder(ctx, safetrade.CreateOrderParams{
					Market: "btcusdt", Side: safetrade.SideSell, Type: safetrade.OrderTypeMarket,
					Amount: decimal.RequireFromString("0.25"),
				})
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders",
			body:     `{"market":"btcusdt","side":"sell","type":"market","amount":"0.25"}`,
			private:  true,
			status:   http.StatusCreated,
			response: `{"id":45,"type":"market"}`,
			check: func(t *testing.T, result any) {
				expectEqual(t, "Type", result.(safetrade.Order).Type, safetrade.OrderTypeMarket)
			},
		},
		{

			name: "CreateOrder market for a quote total",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.CreateOrder(ctx, safetrade.CreateOrderParams{
					Market: "btcusdt", Side: safetrade.SideBuy, Type: safetrade.OrderTypeMarketQuote,
					Total: decimal.RequireFromString("100"),
				})
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders",
			body:     `{"market":"btcusdt","side":"buy","total":"100"}`,
			private:  true,
			status:   http.StatusCreated,
			response: `{"id":43,"type":"market_quote"}`,
			check: func(t *testing.T, result any) {
				expectEqual(t, "Type", result.(safetrade.Order).Type, safetrade.OrderTypeMarketQuote)
			},
		},
		{

			name: "CreateOrder stop-limit",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.CreateOrder(ctx, safetrade.CreateOrderParams{
					Market: "btcusdt", Side: safetrade.SideSell, Type: safetrade.OrderTypeStopLimit,
					Amount: decimal.RequireFromString("0.5"), Price: decimal.RequireFromString("60000"),
					StopPrice: decimal.RequireFromString("60500"), MaxPrice: decimal.RequireFromString("61000"),
				})
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders",
			body:     `{"market":"btcusdt","side":"sell","amount":"0.5","price":"60000","stop_price":"60500","max_price":"61000"}`,
			private:  true,
			status:   http.StatusCreated,
			response: `{"id":44,"type":"stop_limit","stop_price":"60500"}`,
			check: func(t *testing.T, result any) {
				expectDecimal(t, "StopPrice", result.(safetrade.Order).StopPrice, "60500")
			},
		},
		{
			name: "CancelOrder",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.CancelOrder(ctx, 42)
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders/42/cancel",
			body:     `{}`,
			private:  true,
			status:   http.StatusCreated,
			response: order,
			check:    checkOrder,
		},
		{
			name: "CancelOrders",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return nil, c.Market.CancelOrders(ctx, safetrade.CancelOrdersParams{
					Market: safetrade.AllMarkets, Side: safetrade.SideSell,
				})
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders/cancel",
			body:     `{"market":"all","side":"sell"}`,
			private:  true,
			response: `{"status":200}`,
			check:    func(*testing.T, any) {},
		},
		{
			name: "CancelOrders on one market",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return nil, c.Market.CancelOrders(ctx, safetrade.CancelOrdersParams{Market: "btcusdt"})
			},
			method:   http.MethodPost,
			path:     "/trade/market/orders/cancel",
			body:     `{"market":"btcusdt"}`,
			private:  true,
			response: `{"status":200}`,
			check:    func(*testing.T, any) {},
		},
		{
			name: "ListTrades",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.ListTrades(ctx, safetrade.ListTradesParams{
					Market: "btcusdt", Sides: []safetrade.Side{safetrade.SideBuy, safetrade.SideSell},
					TimeFrom: timeFrom, TimeTo: timeTo, OrderBy: "created_at", Ordering: safetrade.OrderingAsc,
					Page: 2, Limit: 20,
				})
			},
			method: http.MethodGet,
			path:   "/trade/market/trades",
			query: url.Values{
				"market": {"btcusdt"}, "side": {"buy,sell"}, "time_from": {"1759276800"}, "time_to": {"1761868800"},
				"order_by": {"created_at"}, "ordering": {"asc"}, "page": {"2"}, "limit": {"20"},
			},
			private:  true,
			response: "[" + trade + "]",
			check: func(t *testing.T, result any) {
				trades := result.([]safetrade.Trade)
				if len(trades) != 1 {
					t.Fatalf("got %d trades, want 1", len(trades))
				}
				checkTrade(t, trades[0])
			},
		},
		{
			name: "GetTrade",
			call: func(ctx context.Context, c *safetrade.Client) (any, error) {
				return c.Market.GetTrade(ctx, 7)
			},
			method:   http.MethodGet,
			path:     "/trade/market/trades/7",
			private:  true,
			response: trade,
			check:    checkTrade,
		},
	}
}

func TestEndpoints(t *testing.T) {
	t.Parallel()

	var cases []endpointCase
	cases = append(cases, publicEndpointCases()...)
	cases = append(cases, accountEndpointCases()...)
	cases = append(cases, marketEndpointCases()...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := newFakeAPI(t)
			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			api.respond(status, tc.response)
			client := api.newClient(t)

			result, err := tc.call(context.Background(), client)
			if err != nil {
				t.Fatalf("call error = %v", err)
			}
			if got := api.requestCount(); got != 1 {
				t.Fatalf("the API received %d requests, want 1", got)
			}
			req := api.lastRequest(t)
			verifyRequest(t, tc, req)
			tc.check(t, result)
		})
	}
}

func verifyRequest(t *testing.T, tc endpointCase, req recordedRequest) {
	t.Helper()

	if req.method != tc.method {
		t.Errorf("method = %s, want %s", req.method, tc.method)
	}
	if want := "/api/v2" + tc.path; req.path != want {
		t.Errorf("path = %s, want %s", req.path, want)
	}
	wantQuery := tc.query
	if wantQuery == nil {
		wantQuery = url.Values{}
	}
	if !reflect.DeepEqual(req.query, wantQuery) {
		t.Errorf("query = %v, want %v", req.query, wantQuery)
	}

	if got := req.header.Get("User-Agent"); got != safetrade.DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, safetrade.DefaultUserAgent)
	}
	if got := req.header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, want application/json", got)
	}

	if tc.body == "" {
		if len(req.body) != 0 {
			t.Errorf("body = %s, want none", req.body)
		}
		if got := req.header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want none for a request without body", got)
		}
	} else {
		if got := req.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		verifyJSONEqual(t, req.body, tc.body)
	}

	if tc.private {
		verifyHMACHeaders(t, req.header)
	} else {
		verifyNoAuthHeaders(t, req.header)
	}
}

func verifyJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("request body %s is not JSON: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("expected body %s is not JSON: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("body = %s, want %s", got, want)
	}
}
