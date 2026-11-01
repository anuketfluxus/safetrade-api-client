package safetrade

import (
	"context"
	"time"
)

type PublicService struct {
	client *Client
}

func (s *PublicService) ListCurrencies(ctx context.Context) ([]Currency, error) {
	return call[[]Currency](ctx, s.client, publicGet("/trade/public/currencies", nil))
}

func (s *PublicService) GetCurrency(ctx context.Context, id string) (Currency, error) {
	segment, err := pathSegment("currency ID", id)
	if err != nil {
		return Currency{}, err
	}
	return call[Currency](ctx, s.client, publicGet("/trade/public/currencies/"+segment, nil))
}

func (s *PublicService) GetPriceHistory(ctx context.Context, currencyID string, period PriceHistoryPeriod) ([]PricePoint, error) {
	segment, err := pathSegment("currency ID", currencyID)
	if err != nil {
		return nil, err
	}
	q := query{}
	q.set("period", string(period))
	return call[[]PricePoint](ctx, s.client, publicGet("/trade/public/currencies/"+segment+"/price-history", q))
}

type MarketCapHistoryParams struct {
	TimeFrom time.Time
	TimeTo   time.Time
}

func (s *PublicService) GetMarketCapHistory(ctx context.Context, params MarketCapHistoryParams) ([]MarketCapSnapshot, error) {
	q := query{}
	q.setUnix("time_from", params.TimeFrom)
	q.setUnix("time_to", params.TimeTo)
	return call[[]MarketCapSnapshot](ctx, s.client, publicGet("/trade/public/marketcap-historical", q))
}

func (s *PublicService) ListMarkets(ctx context.Context) ([]Market, error) {
	return call[[]Market](ctx, s.client, publicGet("/trade/public/markets", nil))
}

func (s *PublicService) GetMarket(ctx context.Context, id string) (Market, error) {
	segment, err := pathSegment("market ID", id)
	if err != nil {
		return Market{}, err
	}
	return call[Market](ctx, s.client, publicGet("/trade/public/markets/"+segment, nil))
}

func (s *PublicService) GetDepth(ctx context.Context, market string, limit int) (Depth, error) {
	segment, err := pathSegment("market ID", market)
	if err != nil {
		return Depth{}, err
	}
	q := query{}
	q.setInt("limit", limit)
	return call[Depth](ctx, s.client, publicGet("/trade/public/markets/"+segment+"/depth", q))
}

type KLineParams struct {
	Period   int
	TimeFrom time.Time
	TimeTo   time.Time

	Page  int
	Limit int
}

func (s *PublicService) ListKLines(ctx context.Context, market string, params KLineParams) ([]KLine, error) {
	segment, err := pathSegment("market ID", market)
	if err != nil {
		return nil, err
	}
	q := query{}
	q.setInt("period", params.Period)
	q.setUnix("time_from", params.TimeFrom)
	q.setUnix("time_to", params.TimeTo)
	q.setPage(params.Page, params.Limit)
	return call[[]KLine](ctx, s.client, publicGet("/trade/public/markets/"+segment+"/k-line", q))
}

type ListMarketTradesParams struct {
	Page  int
	Limit int
}

func (s *PublicService) ListMarketTrades(ctx context.Context, market string, params ListMarketTradesParams) ([]PublicTrade, error) {
	segment, err := pathSegment("market ID", market)
	if err != nil {
		return nil, err
	}
	q := query{}
	q.setPage(params.Page, params.Limit)
	return call[[]PublicTrade](ctx, s.client, publicGet("/trade/public/markets/"+segment+"/trades", q))
}

func (s *PublicService) GetOrderDistribution(ctx context.Context, period DistributionPeriod) (OrderDistribution, error) {
	if period == "" {
		return OrderDistribution{}, invalidArgument("period must not be empty")
	}
	q := query{}
	q.set("period", string(period))
	return call[OrderDistribution](ctx, s.client, publicGet("/trade/public/order-distribution", q))
}

func (s *PublicService) ListTickers(ctx context.Context, period TickerPeriod) (map[string]Ticker, error) {
	q := query{}
	q.set("period", string(period))
	return call[map[string]Ticker](ctx, s.client, publicGet("/trade/public/tickers", q))
}

func (s *PublicService) GetTicker(ctx context.Context, market string) (Ticker, error) {
	segment, err := pathSegment("market ID", market)
	if err != nil {
		return Ticker{}, err
	}
	return call[Ticker](ctx, s.client, publicGet("/trade/public/tickers/"+segment, nil))
}

func (s *PublicService) GetTradeDistribution(ctx context.Context, period DistributionPeriod, market string) (TradeDistribution, error) {
	if period == "" {
		return TradeDistribution{}, invalidArgument("period must not be empty")
	}
	q := query{}
	q.set("period", string(period))
	q.set("market", market)
	return call[TradeDistribution](ctx, s.client, publicGet("/trade/public/trade-distribution", q))
}

type ListTradingFeesParams struct {
	OrderBy  string
	Ordering Ordering

	Page  int
	Limit int
}

func (s *PublicService) ListTradingFees(ctx context.Context, params ListTradingFeesParams) ([]TradingFee, error) {
	q := query{}
	q.setSort(params.OrderBy, params.Ordering)
	q.setPage(params.Page, params.Limit)
	return call[[]TradingFee](ctx, s.client, publicGet("/trade/public/trading_fees", q))
}
