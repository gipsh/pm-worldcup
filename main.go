package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pm-worldcup/clob"
	"pm-worldcup/display"
	"pm-worldcup/gamma"
	"pm-worldcup/market"
	"pm-worldcup/trade"
)

func main() {
	slug := flag.String("slug", "", "Market slug from Polymarket URL (required)")
	showOB := flag.Bool("ob", false, "Show order book charts (toggle with o key)")
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

	// Load trading credentials from env (optional)
	var tradeClient *trade.Client
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	proxyAddress := os.Getenv("POLY_PROXY_ADDRESS")
	if privateKey != "" {
		fmt.Println("Authenticating with Polymarket CLOB...")
		tc, err := trade.NewClient(privateKey, proxyAddress)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: trading disabled — %v\n", err)
		} else {
			tradeClient = tc
			fmt.Println("Trading enabled.")
		}
	} else {
		fmt.Println("No POLY_PRIVATE_KEY set — trading tabs disabled.")
	}

	fmt.Println("Connecting to WebSocket...")

	tokenIDs := make([]string, len(marketInfo.Outcomes))
	for i, o := range marketInfo.Outcomes {
		tokenIDs[i] = o.TokenID
	}

	state := market.NewState(marketInfo.Outcomes)

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

	m := display.NewModel(*marketInfo, state, *showOB, tradeClient)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	wsClient.Close()
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
		tr, err := clob.ParseTradeMessage(data)
		if err != nil {
			return
		}
		state.AddTrade(*tr)
	}
}
