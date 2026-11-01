package safetrade_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/shopspring/decimal"

	"github.com/anuketfluxus/safetrade-api-client"
)

func Example() {
	ctx := context.Background()
	client, err := safetrade.NewClient(
		safetrade.WithAPIKey(os.Getenv("SAFETRADE_API_KEY"), os.Getenv("SAFETRADE_API_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}

	member, err := client.Account.GetMember(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("UID:", member.UID)

	balances, err := client.Account.ListSpotBalances(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, balance := range balances {
		fmt.Printf("%s: %s available, %s locked\n", balance.Currency, balance.Balance, balance.Locked)
	}
}

func ExamplePublicService_ListTickers() {
	client, err := safetrade.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	tickers, err := client.Public.ListTickers(context.Background(), safetrade.TickerPeriod24h)
	if err != nil {
		log.Fatal(err)
	}
	for market, ticker := range tickers {
		fmt.Printf("%s: last %s (%s)\n", market, ticker.Last, ticker.PriceChangePercent)
	}
}

func ExampleMarketService_CreateOrder() {
	client, err := safetrade.NewClient(
		safetrade.WithAPIKey(os.Getenv("SAFETRADE_API_KEY"), os.Getenv("SAFETRADE_API_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}

	order, err := client.Market.CreateOrder(context.Background(), safetrade.CreateOrderParams{
		Market: "xtmusdt",
		Side:   safetrade.SideSell,
		Type:   safetrade.OrderTypeLimit,
		Amount: decimal.RequireFromString("12.5"),
		Price:  decimal.RequireFromString("0.45"),
	})
	switch {
	case err == nil:
		fmt.Printf("order %d is %s\n", order.ID, order.State)
	case safetrade.MayHaveTakenEffect(err):

		log.Fatalf("the order may have been placed: %v", err)
	default:
		log.Fatalf("the order was not placed: %v", err)
	}
}

func ExampleMarketService_ListOrders() {
	client, err := safetrade.NewClient(
		safetrade.WithAPIKey(os.Getenv("SAFETRADE_API_KEY"), os.Getenv("SAFETRADE_API_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}

	orders, err := client.Market.ListOrders(context.Background(), safetrade.ListOrdersParams{
		Market: "btcusdt",
		States: []safetrade.OrderState{safetrade.OrderStateWait},
		Limit:  50,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, order := range orders {
		fmt.Printf("[%d] %s %s @ %s (filled %s)\n", order.ID, order.Side, order.OriginAmount, order.Price, order.FilledAmount)
	}
}

func ExampleAPIError() {
	client, err := safetrade.NewClient(
		safetrade.WithAPIKey(os.Getenv("SAFETRADE_API_KEY"), os.Getenv("SAFETRADE_API_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Market.GetOrder(context.Background(), 42)
	var apiErr *safetrade.APIError
	switch {
	case err == nil:
		fmt.Println("found")
	case errors.Is(err, safetrade.ErrNotFound):
		fmt.Println("no such order")
	case errors.As(err, &apiErr):
		fmt.Println("API error", apiErr.StatusCode, apiErr.Messages())
	default:
		fmt.Println("request failed:", err)
	}
}
