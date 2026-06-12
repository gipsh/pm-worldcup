package market

import "testing"

func testOutcomes() []Outcome {
	return []Outcome{
		{Name: "Canada", TokenID: "tokA"},
		{Name: "Draw", TokenID: "tokB"},
	}
}

func TestState_UpdateBook(t *testing.T) {
	s := NewState(testOutcomes())
	bids := []PriceLevel{{Price: "0.42", Size: "100"}}
	asks := []PriceLevel{{Price: "0.43", Size: "150"}}
	s.UpdateBook("tokA", bids, asks)

	snaps, _ := s.Snapshot(testOutcomes())
	if len(snaps[0].Book.Bids) != 1 || snaps[0].Book.Bids[0].Price != "0.42" {
		t.Errorf("unexpected bids: %+v", snaps[0].Book.Bids)
	}
	if len(snaps[0].Book.Asks) != 1 || snaps[0].Book.Asks[0].Price != "0.43" {
		t.Errorf("unexpected asks: %+v", snaps[0].Book.Asks)
	}
}

func TestState_UpdateBestPrices(t *testing.T) {
	s := NewState(testOutcomes())
	s.UpdateBestPrices("tokB", "0.22", "0.24")

	snaps, _ := s.Snapshot(testOutcomes())
	if snaps[1].Book.BestBid != "0.22" {
		t.Errorf("want BestBid 0.22, got %q", snaps[1].Book.BestBid)
	}
	if snaps[1].Book.BestAsk != "0.24" {
		t.Errorf("want BestAsk 0.24, got %q", snaps[1].Book.BestAsk)
	}
}

func TestState_AddTrade_UpdatesLastPrice(t *testing.T) {
	s := NewState(testOutcomes())
	s.AddTrade(Trade{Timestamp: 1000, AssetID: "tokA", Price: "0.42", Size: "50", Side: "buy"})

	snaps, _ := s.Snapshot(testOutcomes())
	if snaps[0].LastPrice != "0.42" {
		t.Errorf("want LastPrice 0.42, got %q", snaps[0].LastPrice)
	}
}

func TestState_AddTrade_RingBuffer(t *testing.T) {
	s := NewState(testOutcomes())
	for i := 0; i < 15; i++ {
		s.AddTrade(Trade{Timestamp: int64(i), AssetID: "tokA", Price: "0.42", Size: "1", Side: "buy"})
	}
	_, trades := s.Snapshot(testOutcomes())
	if len(trades) != 10 {
		t.Errorf("want 10 trades in ring buffer, got %d", len(trades))
	}
	if trades[0].Timestamp != 5 {
		t.Errorf("want oldest timestamp 5, got %d", trades[0].Timestamp)
	}
}

func TestState_SnapshotOrderMatchesOutcomes(t *testing.T) {
	s := NewState(testOutcomes())
	s.UpdateBestPrices("tokB", "0.30", "0.32")
	s.UpdateBestPrices("tokA", "0.60", "0.62")

	snaps, _ := s.Snapshot(testOutcomes())
	if snaps[0].Outcome.Name != "Canada" {
		t.Errorf("want snaps[0] = Canada, got %q", snaps[0].Outcome.Name)
	}
	if snaps[1].Outcome.Name != "Draw" {
		t.Errorf("want snaps[1] = Draw, got %q", snaps[1].Outcome.Name)
	}
}
