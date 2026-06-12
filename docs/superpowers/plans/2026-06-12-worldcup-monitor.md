# pm-worldcup CLI Monitor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI tool that monitors any Polymarket World Cup market in real-time via WebSocket, displaying a probability table and order book chart per outcome.

**Architecture:** Standalone Go module `pm-worldcup` with four packages: `clob` (WebSocket client, copied from `go-polymarket-watcher`), `gamma` (Gamma API client), `market` (types + thread-safe state), and `display` (ASCII terminal renderer). `main.go` wires them together with a 100ms render loop.

**Tech Stack:** Go 1.21, `github.com/gorilla/websocket v1.5.3`, standard library only (no TUI framework).

---

## File Map

| File | Status | Responsibility |
|---|---|---|
| `go.mod` | Create | Module declaration + gorilla/websocket dep |
| `clob/websocket.go` | Copy + edit | WebSocket client (change import path only) |
| `market/types.go` | Create | Outcome, Market, Orderbook, PriceLevel, Trade, OutcomeSnapshot |
| `market/state.go` | Create | Thread-safe state: map[tokenID]→Orderbook, ring buffer trades |
| `market/state_test.go` | Create | Unit tests for state mutations and snapshot |
| `gamma/client.go` | Create | FetchMarketBySlug + parseMarket (N outcomes) |
| `gamma/client_test.go` | Create | Unit tests for JSON parsing |
| `display/terminal.go` | Create | Render: header + price table + N charts + trades |
| `main.go` | Create | Flags, wiring, event handler, render loop |

---

## Task 1: Initialize module and copy WebSocket client

**Files:**
- Create: `go.mod`
- Create: `clob/websocket.go` (copy + single import path edit)

- [ ] **Step 1: Create go.mod**

```
module pm-worldcup

go 1.21

require github.com/gorilla/websocket v1.5.3
```

Save to `/Users/hernangips/workspace/pm-worldcup/go.mod`.

- [ ] **Step 2: Copy websocket.go from the existing project**

```bash
cp /Users/hernangips/workspace/go-polymarket-watcher/clob/websocket.go \
   /Users/hernangips/workspace/pm-worldcup/clob/websocket.go
```

- [ ] **Step 3: Fix the import path in clob/websocket.go**

Change line 12 from:
```go
	"polymarket-watcher/market"
```
to:
```go
	"pm-worldcup/market"
```

- [ ] **Step 4: Fetch dependencies**

Run from `/Users/hernangips/workspace/pm-worldcup`:
```bash
go mod tidy
```
Expected: creates `go.sum`, no errors.

- [ ] **Step 5: Verify clob package compiles (will fail on missing market package — that's fine)**

```bash
go build ./clob/... 2>&1 | head -5
```
Expected: error about `pm-worldcup/market` not found. That's correct — we haven't written it yet.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum clob/
git commit -m "feat: init module and copy websocket client"
```

---

## Task 2: Market types

**Files:**
- Create: `market/types.go`

- [ ] **Step 1: Write market/types.go**

```go
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
```

- [ ] **Step 2: Verify clob now compiles**

```bash
go build ./clob/...
```
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add market/types.go
git commit -m "feat: add market types"
```

---

## Task 3: Market state (TDD)

**Files:**
- Create: `market/state.go`
- Create: `market/state_test.go`

- [ ] **Step 1: Write the failing tests**

Create `market/state_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./market/... -v 2>&1 | head -20
```
Expected: compilation error — `NewState`, `UpdateBook`, etc. undefined.

- [ ] **Step 3: Write market/state.go**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./market/... -v
```
Expected output:
```
=== RUN   TestState_UpdateBook
--- PASS: TestState_UpdateBook
=== RUN   TestState_UpdateBestPrices
--- PASS: TestState_UpdateBestPrices
=== RUN   TestState_AddTrade_UpdatesLastPrice
--- PASS: TestState_AddTrade_UpdatesLastPrice
=== RUN   TestState_AddTrade_RingBuffer
--- PASS: TestState_AddTrade_RingBuffer
=== RUN   TestState_SnapshotOrderMatchesOutcomes
--- PASS: TestState_SnapshotOrderMatchesOutcomes
PASS
```

- [ ] **Step 5: Commit**

```bash
git add market/state.go market/state_test.go
git commit -m "feat: add thread-safe market state with ring buffer"
```

---

## Task 4: Gamma client (TDD)

**Files:**
- Create: `gamma/client.go`
- Create: `gamma/client_test.go`

- [ ] **Step 1: Write the failing tests**

Create `gamma/client_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./gamma/... -v 2>&1 | head -10
```
Expected: compilation error — `marketResponse`, `parseMarket` undefined.

- [ ] **Step 3: Write gamma/client.go**

```go
package gamma

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pm-worldcup/market"
)

const baseURL = "https://gamma-api.polymarket.com"

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type marketResponse struct {
	Question     string `json:"question"`
	Slug         string `json:"slug"`
	ClobTokenIDs string `json:"clobTokenIds"`
	Outcomes     string `json:"outcomes"`
	Closed       bool   `json:"closed"`
	EndDate      string `json:"endDate"`
	Volume       string `json:"volume"`
}

func (c *Client) FetchMarketBySlug(slug string) (*market.Market, error) {
	url := fmt.Sprintf("%s/markets?slug=%s", baseURL, slug)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	var markets []marketResponse
	if err := json.NewDecoder(resp.Body).Decode(&markets); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}
	if len(markets) == 0 {
		return nil, fmt.Errorf("market not found: %s", slug)
	}
	return parseMarket(markets[0])
}

func parseMarket(m marketResponse) (*market.Market, error) {
	var tokenIDs []string
	if err := json.Unmarshal([]byte(m.ClobTokenIDs), &tokenIDs); err != nil {
		return nil, fmt.Errorf("parse token IDs: %w", err)
	}

	var names []string
	if m.Outcomes != "" {
		_ = json.Unmarshal([]byte(m.Outcomes), &names)
	}

	outcomes := make([]market.Outcome, len(tokenIDs))
	for i, id := range tokenIDs {
		name := fmt.Sprintf("Outcome %d", i+1)
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		outcomes[i] = market.Outcome{Name: name, TokenID: id}
	}

	endDate, _ := time.Parse(time.RFC3339, m.EndDate)

	return &market.Market{
		Slug:     m.Slug,
		Question: m.Question,
		Outcomes: outcomes,
		Closed:   m.Closed,
		EndDate:  endDate,
		Volume:   m.Volume,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./gamma/... -v
```
Expected output:
```
=== RUN   TestParseMarket_ThreeOutcomes
--- PASS: TestParseMarket_ThreeOutcomes
=== RUN   TestParseMarket_MissingOutcomes_FallsBackToDefaults
--- PASS: TestParseMarket_MissingOutcomes_FallsBackToDefaults
=== RUN   TestParseMarket_MarketMetadata
--- PASS: TestParseMarket_MarketMetadata
PASS
```

- [ ] **Step 5: Run all tests**

```bash
go test ./...
```
Expected: all packages pass.

- [ ] **Step 6: Commit**

```bash
git add gamma/client.go gamma/client_test.go
git commit -m "feat: add gamma client with N-outcome parsing"
```

---

## Task 5: Display terminal

**Files:**
- Create: `display/terminal.go`

No unit tests — rendering is verified visually in Task 6 end-to-end.

- [ ] **Step 1: Write display/terminal.go**

```go
package display

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"pm-worldcup/market"
)

type Terminal struct {
	info   market.Market
	obOnly bool
}

func NewTerminal(m market.Market, obOnly bool) *Terminal {
	return &Terminal{info: m, obOnly: obOnly}
}

func (t *Terminal) Render(snaps []market.OutcomeSnapshot, trades []market.Trade) {
	clearScreen()
	if t.obOnly {
		t.renderCharts(snaps)
		return
	}
	t.renderHeader()
	fmt.Println()
	t.renderPriceTable(snaps)
	fmt.Println()
	t.renderCharts(snaps)
	fmt.Println()
	t.renderTrades(snaps, trades)
}

func (t *Terminal) renderHeader() {
	fmt.Println(strings.Repeat("═", 64))
	remaining := time.Until(t.info.EndDate)
	status := formatDuration(remaining)
	if t.info.Closed || remaining <= 0 {
		status = "CLOSED"
	}
	fmt.Printf("  %-36s  Time: %-10s  Vol: %s\n", t.info.Slug, status, formatVolume(t.info.Volume))
	fmt.Printf("  %s\n", t.info.Question)
	fmt.Println(strings.Repeat("═", 64))
}

func (t *Terminal) renderPriceTable(snaps []market.OutcomeSnapshot) {
	maxName := 7
	for _, s := range snaps {
		if len(s.Outcome.Name) > maxName {
			maxName = len(s.Outcome.Name)
		}
	}
	pad := strings.Repeat("─", maxName+2)
	fmt.Printf("┌─%s─┬────────┬────────┬────────┬────────┬────────┐\n", pad)
	fmt.Printf("│ %-*s │  Price │  Bid   │  Ask   │ Spread │  Prob  │\n", maxName, "Outcome")
	fmt.Printf("├─%s─┼────────┼────────┼────────┼────────┼────────┤\n", pad)

	totalMid := 0.0
	for _, s := range snaps {
		mid := calcMid(s.Book.BestBid, s.Book.BestAsk)
		totalMid += mid
		fmt.Printf("│ %-*s │ %s │ %s │ %s │ %s │ %5.1f%% │\n",
			maxName, s.Outcome.Name,
			formatPrice(s.LastPrice),
			formatPrice(s.Book.BestBid),
			formatPrice(s.Book.BestAsk),
			calcSpread(s.Book.BestBid, s.Book.BestAsk),
			mid*100,
		)
	}
	fmt.Printf("└─%s─┴────────┴────────┴────────┴────────┴────────┘\n", pad)
	fmt.Printf("  Total implied prob: %.1f%%\n", totalMid*100)
}

func (t *Terminal) renderCharts(snaps []market.OutcomeSnapshot) {
	for _, s := range snaps {
		fmt.Printf("  ── %s Order Book ──\n", s.Outcome.Name)
		renderBookChart(s.Book)
		fmt.Println()
	}
}

func renderBookChart(book market.Orderbook) {
	bidVols := make(map[int]float64)
	askVols := make(map[int]float64)

	for _, level := range book.Bids {
		price, _ := strconv.ParseFloat(level.Price, 64)
		size, _ := strconv.ParseFloat(level.Size, 64)
		bucket := int(math.Round(price * 100))
		bidVols[bucket] += size
	}
	for _, level := range book.Asks {
		price, _ := strconv.ParseFloat(level.Price, 64)
		size, _ := strconv.ParseFloat(level.Size, 64)
		bucket := int(math.Round(price * 100))
		askVols[bucket] += size
	}

	maxVol := 1.0
	for _, v := range bidVols {
		if v > maxVol {
			maxVol = v
		}
	}
	for _, v := range askVols {
		if v > maxVol {
			maxVol = v
		}
	}

	minBucket, maxBucket := 100, 0
	for b := range bidVols {
		if b < minBucket {
			minBucket = b
		}
		if b > maxBucket {
			maxBucket = b
		}
	}
	for b := range askVols {
		if b < minBucket {
			minBucket = b
		}
		if b > maxBucket {
			maxBucket = b
		}
	}

	if minBucket > maxBucket {
		fmt.Println("    (no data)")
		return
	}
	if minBucket > 2 {
		minBucket -= 2
	}
	if maxBucket < 98 {
		maxBucket += 2
	}

	chartHeight := 12
	bucketRange := maxBucket - minBucket + 1

	for row := chartHeight; row >= 1; row-- {
		threshold := (float64(row) / float64(chartHeight)) * maxVol
		var label string
		switch row {
		case chartHeight:
			label = fmt.Sprintf("%6.0f │", maxVol)
		case chartHeight / 2:
			label = fmt.Sprintf("%6.0f │", maxVol/2)
		case 1:
			label = fmt.Sprintf("%6.0f │", 0.0)
		default:
			label = "       │"
		}
		line := "    " + label
		for bucket := minBucket; bucket <= maxBucket; bucket++ {
			bid := bidVols[bucket]
			ask := askVols[bucket]
			switch {
			case bid >= threshold && ask >= threshold:
				line += "▓"
			case bid >= threshold:
				line += "█"
			case ask >= threshold:
				line += "░"
			default:
				line += " "
			}
		}
		fmt.Println(line)
	}

	fmt.Printf("           └%s\n", strings.Repeat("─", bucketRange))
	labelLine := "            "
	skip := 0
	for bucket := minBucket; bucket <= maxBucket; bucket++ {
		if skip > 0 {
			skip--
			continue
		}
		if bucket%10 == 0 {
			lbl := fmt.Sprintf("%.1f", float64(bucket)/100)
			labelLine += lbl
			skip = len(lbl) - 1
		} else {
			labelLine += " "
		}
	}
	fmt.Println(labelLine)
	fmt.Println("    Legend: █ Bids  ░ Asks  ▓ Both")

	bidTotal, askTotal := calcBookTotals(book)
	fmt.Printf("    Bids: %.0f  Asks: %.0f  Spread: %s\n",
		bidTotal, askTotal, calcSpread(book.BestBid, book.BestAsk))
}

func (t *Terminal) renderTrades(snaps []market.OutcomeSnapshot, trades []market.Trade) {
	nameByToken := make(map[string]string, len(snaps))
	for _, s := range snaps {
		nameByToken[s.Outcome.TokenID] = s.Outcome.Name
	}

	fmt.Println("  Last Trades:")
	if len(trades) == 0 {
		fmt.Println("    No trades yet...")
		return
	}
	for i := len(trades) - 1; i >= 0; i-- {
		tr := trades[i]
		ts := time.UnixMilli(tr.Timestamp).Format("15:04:05")
		name := nameByToken[tr.AssetID]
		if name == "" && len(tr.AssetID) >= 8 {
			name = tr.AssetID[:8] + "..."
		}
		side := strings.ToUpper(tr.Side)
		if len(side) > 4 {
			side = side[:4]
		}
		fmt.Printf("    [%s] %-24s %-4s %s x %s\n",
			ts, name, side, formatPrice(tr.Price), formatSize(tr.Size))
	}
}

func clearScreen() { fmt.Print("\033[H\033[2J") }

func calcMid(bid, ask string) float64 {
	b, err1 := strconv.ParseFloat(bid, 64)
	a, err2 := strconv.ParseFloat(ask, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	return (b + a) / 2
}

func calcSpread(bid, ask string) string {
	b, err1 := strconv.ParseFloat(bid, 64)
	a, err2 := strconv.ParseFloat(ask, 64)
	if err1 != nil || err2 != nil {
		return "  -   "
	}
	return fmt.Sprintf("%.4f", a-b)
}

func calcBookTotals(book market.Orderbook) (bid, ask float64) {
	for _, l := range book.Bids {
		if v, err := strconv.ParseFloat(l.Size, 64); err == nil {
			bid += v
		}
	}
	for _, l := range book.Asks {
		if v, err := strconv.ParseFloat(l.Size, 64); err == nil {
			ask += v
		}
	}
	return
}

func formatPrice(p string) string {
	if p == "" {
		return "  -   "
	}
	f, err := strconv.ParseFloat(p, 64)
	if err != nil {
		return p
	}
	return fmt.Sprintf("%.4f", f)
}

func formatSize(s string) string {
	if s == "" {
		return "-"
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%.2f", f)
}

func formatVolume(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	if f >= 1_000_000 {
		return fmt.Sprintf("$%.2fM", f/1_000_000)
	}
	if f >= 1_000 {
		return fmt.Sprintf("$%.1fK", f/1_000)
	}
	return fmt.Sprintf("$%.2f", f)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "ENDED"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
```

- [ ] **Step 2: Verify display compiles**

```bash
go build ./display/...
```
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add display/terminal.go
git commit -m "feat: add terminal display with N-outcome price table and charts"
```

---

## Task 6: Main wiring

**Files:**
- Create: `main.go`

- [ ] **Step 1: Write main.go**

```go
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
```

- [ ] **Step 2: Build the binary**

```bash
go build -o pm-worldcup .
```
Expected: creates `pm-worldcup` binary, no errors.

- [ ] **Step 3: Run all tests one final time**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add main wiring — CLI is functional"
```

---

## Task 7: End-to-end smoke test

**Files:** none — manual verification only.

- [ ] **Step 1: Verify -slug flag is required**

```bash
./pm-worldcup
```
Expected output:
```
Error: -slug is required
Usage of ./pm-worldcup:
  -ob
        Show order book charts only
  -slug string
        Market slug from Polymarket URL (required)
```

- [ ] **Step 2: Test with a real World Cup market slug**

Find a live or recent market slug from `https://polymarket.com/sports/world-cup`. Extract the slug from the URL path (last segment, e.g. `fifwc-can-bih-2026-06-12`).

```bash
./pm-worldcup -slug fifwc-can-bih-2026-06-12
```

Expected:
1. Prints market question and outcome names
2. Prints "Connecting to WebSocket..." and "Connected!"
3. Terminal clears and shows the live display with:
   - Header line with slug, time remaining, volume
   - Price table with one row per outcome showing bid/ask/prob
   - Order book chart per outcome (may show "no data" briefly until WS data arrives)
   - Last trades section

- [ ] **Step 3: Test order-book-only mode**

```bash
./pm-worldcup -slug fifwc-can-bih-2026-06-12 -ob
```
Expected: only the order book charts render, no price table or trade feed.

- [ ] **Step 4: Test Ctrl+C shutdown**

Press `Ctrl+C` while running.
Expected: prints "Shutting down..." and exits cleanly.

- [ ] **Step 5: Test with a binary (YES/NO) market to verify N=2 works**

Find a binary World Cup market slug (e.g. `will-argentina-win-world-cup-2026`).

```bash
./pm-worldcup -slug will-argentina-win-world-cup-2026
```
Expected: price table shows exactly 2 rows.

- [ ] **Step 6: Add binary to .gitignore and final commit**

Create `.gitignore`:
```
pm-worldcup
```

```bash
git add .gitignore
git commit -m "chore: add .gitignore"
```
