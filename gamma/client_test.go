package gamma

import "testing"

func TestParseMarket_ThreeOutcomes(t *testing.T) {
	m := marketResponse{
		Question:     "Who wins?",
		Slug:         "fifwc-can-bih-2026-06-12",
		ClobTokenIDs: `["tokenA","tokenB","tokenC"]`,
		Outcomes:     `["Canada","Draw","Bosnia and Herzegovina"]`,
		Closed:       false,
		EndDate:      "2026-06-12T18:00:00Z",
		Volume:       "12345.67",
	}
	got, err := parseMarket(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Outcomes) != 3 {
		t.Fatalf("want 3 outcomes, got %d", len(got.Outcomes))
	}
	if got.Outcomes[0].Name != "Canada" || got.Outcomes[0].TokenID != "tokenA" {
		t.Errorf("outcome 0: got %+v", got.Outcomes[0])
	}
	if got.Outcomes[1].Name != "Draw" || got.Outcomes[1].TokenID != "tokenB" {
		t.Errorf("outcome 1: got %+v", got.Outcomes[1])
	}
	if got.Outcomes[2].Name != "Bosnia and Herzegovina" || got.Outcomes[2].TokenID != "tokenC" {
		t.Errorf("outcome 2: got %+v", got.Outcomes[2])
	}
}

func TestParseMarket_MissingOutcomes_FallsBackToDefaults(t *testing.T) {
	m := marketResponse{
		Question:     "Will Canada win?",
		Slug:         "will-canada-win",
		ClobTokenIDs: `["tokenA","tokenB"]`,
		Outcomes:     "",
	}
	got, err := parseMarket(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Outcomes) != 2 {
		t.Fatalf("want 2 outcomes, got %d", len(got.Outcomes))
	}
	if got.Outcomes[0].Name != "Outcome 1" {
		t.Errorf("want 'Outcome 1', got %q", got.Outcomes[0].Name)
	}
	if got.Outcomes[1].Name != "Outcome 2" {
		t.Errorf("want 'Outcome 2', got %q", got.Outcomes[1].Name)
	}
}

func TestParseEvent_NegRisk(t *testing.T) {
	e := eventResponse{
		Slug:    "fifwc-usa-par-2026-06-12",
		Title:   "United States vs. Paraguay",
		Closed:  false,
		EndDate: "2026-06-13T01:00:00Z",
		Volume:  9068860.89,
		Markets: []eventSubMarket{
			{Slug: "fifwc-usa-par-2026-06-12-draw", ClobTokenIDs: `["tokDraw","tokDrawNo"]`},
			{Slug: "fifwc-usa-par-2026-06-12-usa", ClobTokenIDs: `["tokUSA","tokUSANo"]`},
			{Slug: "fifwc-usa-par-2026-06-12-par", ClobTokenIDs: `["tokPAR","tokPARNo"]`},
		},
	}
	got, err := parseEvent(e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Outcomes) != 3 {
		t.Fatalf("want 3 outcomes, got %d", len(got.Outcomes))
	}
	if got.Outcomes[0].Name != "DRAW" || got.Outcomes[0].TokenID != "tokDraw" {
		t.Errorf("outcome 0: got %+v", got.Outcomes[0])
	}
	if got.Outcomes[1].Name != "USA" || got.Outcomes[1].TokenID != "tokUSA" {
		t.Errorf("outcome 1: got %+v", got.Outcomes[1])
	}
	if got.Outcomes[2].Name != "PAR" || got.Outcomes[2].TokenID != "tokPAR" {
		t.Errorf("outcome 2: got %+v", got.Outcomes[2])
	}
	if got.Slug != "fifwc-usa-par-2026-06-12" {
		t.Errorf("want slug fifwc-usa-par-2026-06-12, got %q", got.Slug)
	}
	if got.Question != "United States vs. Paraguay" {
		t.Errorf("want title 'United States vs. Paraguay', got %q", got.Question)
	}
}

func TestParseMarket_MarketMetadata(t *testing.T) {
	m := marketResponse{
		Question:     "Who wins the match?",
		Slug:         "fifwc-arg-bra-2026-07-01",
		ClobTokenIDs: `["tokA","tokB"]`,
		Outcomes:     `["Argentina","Brazil"]`,
		Volume:       "99999.00",
	}
	got, err := parseMarket(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Slug != "fifwc-arg-bra-2026-07-01" {
		t.Errorf("want slug fifwc-arg-bra-2026-07-01, got %q", got.Slug)
	}
	if got.Question != "Who wins the match?" {
		t.Errorf("want question 'Who wins the match?', got %q", got.Question)
	}
	if got.Volume != "99999.00" {
		t.Errorf("want volume 99999.00, got %q", got.Volume)
	}
}
