package stream_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/anuketfluxus/safetrade-api-client/stream"
)

func Example() {
	ctx := context.Background()
	client, err := stream.Dial(ctx, stream.Public)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.Subscribe(ctx, stream.GlobalTickers, stream.MarketTrades("btcusdt")); err != nil {
		log.Print(err)
		return
	}
	for {
		msg, err := client.Read(ctx)
		if err != nil {
			log.Print(err)
			return
		}
		switch msg := msg.(type) {
		case stream.Update:
			switch msg.Kind() {
			case stream.KindTickers:
				tickers, err := msg.Tickers()
				if err != nil {
					log.Print(err)
					continue
				}
				fmt.Println("BTC/USDT last price:", tickers["btcusdt"].Last)
			case stream.KindMarketTrades:
				trades, err := msg.MarketTrades()
				if err != nil {
					log.Print(err)
					continue
				}
				fmt.Println(len(trades), "new trades on", msg.Market())
			default:

			}
		case stream.Disconnected:
			log.Println("disconnected:", msg.Err)
		case stream.Reconnected:
			log.Println("reconnected; updates were missed in between")
		}
	}
}

func ExampleDial_private() {
	ctx := context.Background()
	client, err := stream.Dial(ctx, stream.Private,
		stream.WithAPIKey(os.Getenv("SAFETRADE_API_KEY"), os.Getenv("SAFETRADE_API_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.Subscribe(ctx, stream.Orders, stream.Trades, stream.Balances); err != nil {
		log.Print(err)
		return
	}
	for {
		msg, err := client.Read(ctx)
		if err != nil {
			log.Print(err)
			return
		}
		update, ok := msg.(stream.Update)
		if !ok || update.Kind() != stream.KindOrder {
			continue
		}
		order, err := update.Order()
		if err != nil {
			log.Print(err)
			continue
		}
		fmt.Printf("order %d is now %s\n", order.ID, order.State)
	}
}
