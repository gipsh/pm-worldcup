package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pm-worldcup/clob"
	"pm-worldcup/display"
	"pm-worldcup/gamma"
	"pm-worldcup/market"
)

func main() {
	slug := flag.String("slug", "", "Market slug from Polymarket URL (required)")
	obOnly := flag.Bool("ob", false, "Show order book charts only")
	flag.Parse()

	if *slug == "" {
		fmt.Fprintln(os.Stderr, "Error: -slug is required")
		flag.Usage()
		os.Exit(1)
	}

	gammaClient := gamma.NewClient()
	marketInfo, err := gammaClient.FetchMarketBySlug(*slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch market: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Market: %s\n", marketInfo.Question)
	for i, o := range marketInfo.Outcomes {
		fmt.Printf("  Outcome %d: %s\n", i+1, o.Name)
	}
	fmt.Println("Connecting to WebSocket...")

	tokenIDs := make([]string, len(marketInfo.Outcomes))
	for i, o := range marketInfo.Outcomes {
		tokenIDs[i] = o.TokenID
	}

	state := market.NewState(marketInfo.Outcomes)
	term := display.NewTerminal(*marketInfo, *obOnly)

	wsClient := clob.NewWSClient(func(eventType string, data json.RawMessage) {
		handleEvent(eventType, data, state)
	})

	if err := wsClient.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "WebSocket connection failed: %v\n", err)
		os.Exit(1)
	}
	if err := wsClient.Subscribe(tokenIDs); err != nil {
		fmt.Fprintf(os.Stderr, "Subscribe failed: %v\n", err)
		os.Exit(1)
	}
	wsClient.Start()

	fmt.Println("Connected! Waiting for data...")
	time.Sleep(time.Second)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			wsClient.Close()
			return
		case <-ticker.C:
			snaps, trades := state.Snapshot(marketInfo.Outcomes)
			term.Render(snaps, trades)
		}
	}
}

func handleEvent(eventType string, data json.RawMessage, state *market.State) {
	switch eventType {
	case "book":
		msg, err := clob.ParseBookMessage(data)
		if err != nil {
			return
		}
		bids := make([]market.PriceLevel, len(msg.Bids))
		for i, b := range msg.Bids {
			bids[i] = market.PriceLevel{Price: b.Price, Size: b.Size}
		}
		asks := make([]market.PriceLevel, len(msg.Asks))
		for i, a := range msg.Asks {
			asks[i] = market.PriceLevel{Price: a.Price, Size: a.Size}
		}
		state.UpdateBook(msg.AssetID, bids, asks)

	case "price_change":
		msg, err := clob.ParsePriceChangeMessage(data)
		if err != nil {
			return
		}
		for _, pc := range msg.PriceChanges {
			state.UpdateBestPrices(pc.AssetID, pc.BestBid, pc.BestAsk)
		}

	case "last_trade_price":
		trade, err := clob.ParseTradeMessage(data)
		if err != nil {
			return
		}
		state.AddTrade(*trade)
	}
}
