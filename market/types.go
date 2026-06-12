package market

import "time"

type Outcome struct {
	Name    string
	TokenID string
}

type Market struct {
	Slug     string
	Question string
	Outcomes []Outcome
	Closed   bool
	EndDate  time.Time
	Volume   string
}

type PriceLevel struct {
	Price string
	Size  string
}

type Orderbook struct {
	AssetID string
	BestBid string
	BestAsk string
	Bids    []PriceLevel
	Asks    []PriceLevel
}

type Trade struct {
	Timestamp int64
	AssetID   string
	Price     string
	Size      string
	Side      string
}

type OutcomeSnapshot struct {
	Outcome   Outcome
	Book      Orderbook
	LastPrice string
}
