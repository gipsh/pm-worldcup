package market

import "sync"

type State struct {
	mu     sync.RWMutex
	books  map[string]*Orderbook
	prices map[string]string
	trades []Trade
}

func NewState(outcomes []Outcome) *State {
	books := make(map[string]*Orderbook, len(outcomes))
	prices := make(map[string]string, len(outcomes))
	for _, o := range outcomes {
		books[o.TokenID] = &Orderbook{AssetID: o.TokenID}
		prices[o.TokenID] = ""
	}
	return &State{books: books, prices: prices}
}

func (s *State) UpdateBook(assetID string, bids, asks []PriceLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.books[assetID]; ok {
		b.Bids = bids
		b.Asks = asks
	}
}

func (s *State) UpdateBestPrices(assetID, bestBid, bestAsk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.books[assetID]; ok {
		b.BestBid = bestBid
		b.BestAsk = bestAsk
	}
}

func (s *State) AddTrade(t Trade) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices[t.AssetID] = t.Price
	s.trades = append(s.trades, t)
	if len(s.trades) > 10 {
		s.trades = s.trades[len(s.trades)-10:]
	}
}

// SnapshotOne returns a snapshot for a single outcome.
func (s *State) SnapshotOne(outcome Outcome) OutcomeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	book := s.books[outcome.TokenID]
	if book == nil {
		book = &Orderbook{AssetID: outcome.TokenID}
	}
	bookCopy := *book
	return OutcomeSnapshot{
		Outcome:   outcome,
		Book:      bookCopy,
		LastPrice: s.prices[outcome.TokenID],
	}
}

func (s *State) Snapshot(outcomes []Outcome) ([]OutcomeSnapshot, []Trade) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snaps := make([]OutcomeSnapshot, len(outcomes))
	for i, o := range outcomes {
		book := *s.books[o.TokenID]
		snaps[i] = OutcomeSnapshot{
			Outcome:   o,
			Book:      book,
			LastPrice: s.prices[o.TokenID],
		}
	}
	trades := make([]Trade, len(s.trades))
	copy(trades, s.trades)
	return snaps, trades
}
