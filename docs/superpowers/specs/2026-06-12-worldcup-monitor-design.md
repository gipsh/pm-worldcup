# pm-worldcup CLI Monitor — Design Spec

**Date:** 2026-06-12  
**Status:** Approved

## Overview

A Go CLI tool that monitors a Polymarket World Cup market in real-time via WebSocket. It displays probabilities/prices for each outcome and a visual order book chart per outcome. Built as a standalone module inspired by `go-polymarket-watcher`, generalized for N outcomes instead of binary YES/NO.

## Architecture

```
pm-worldcup/
├── main.go              # Entry point: flags, wiring, render loop
├── go.mod               # module pm-worldcup
├── clob/
│   └── websocket.go     # Copied from go-polymarket-watcher (unchanged)
├── gamma/
│   └── client.go        # Extended: parses N outcomes + their names
├── market/
│   ├── types.go         # Generalized types for N outcomes
│   └── state.go         # Thread-safe state: map[tokenID]→Orderbook
└── display/
    └── terminal.go      # Display: price table + N order book charts
```

## Data Model

### `market/types.go`

```go
type Outcome struct {
    Name    string  // e.g. "Canada", "Draw", "Bosnia and Herzegovina"
    TokenID string  // CLOB token ID
}

type Market struct {
    Slug     string
    Question string
    Outcomes []Outcome
    Closed   bool
    EndDate  time.Time
    Volume   string
}

type Orderbook struct {
    AssetID string
    BestBid string
    BestAsk string
    Bids    []PriceLevel
    Asks    []PriceLevel
}

type PriceLevel struct{ Price, Size string }

type Trade struct {
    Timestamp int64
    AssetID, Price, Size, Side string
}
```

### `market/state.go`

Thread-safe state container:
- `books map[string]*Orderbook` — tokenID → current orderbook
- `prices map[string]string` — tokenID → last trade price
- `trades []Trade` — ring buffer, last 10 trades across all outcomes
- `Snapshot() []OutcomeSnapshot` — returns immutable snapshot for rendering

`OutcomeSnapshot` pairs an `Outcome` (name + tokenID) with its current `Orderbook` and `LastPrice`.

### `gamma/client.go`

Fetches market by slug from `https://gamma-api.polymarket.com/markets?slug=<slug>`.

Extended parsing: reads both `clobTokenIds` (JSON string array of token IDs) and `outcomes` (JSON string array of outcome names), zips them into `[]Outcome`. Index `i` of outcomes matches index `i` of token IDs.

## WebSocket Protocol

Connects to `wss://ws-subscriptions-clob.polymarket.com/ws/market`.  
Subscribes with all N tokenIDs in a single message:
```json
{"assets_ids": ["tokenA", "tokenB", "tokenC"], "type": "market"}
```

Ping/pong every 10 seconds (sends `"PING"`, expects `"PONG"`).

**Event handling:**
| Event type | Action |
|---|---|
| `book` | Full orderbook snapshot for one tokenID — replaces existing book |
| `price_change` | Updates BestBid/BestAsk for one or more tokenIDs |
| `last_trade_price` | Appends trade to ring buffer |

WebSocket writes to `State` under mutex. Render loop reads an immutable snapshot every 100ms.

## Display Layout

```
════════════════════════════════════════════════════════════
  fifwc-can-bih-2026-06-12    Time: 87m 23s    Vol: $124.3K
  Will Canada win FIFA World Cup match vs Bosnia?
════════════════════════════════════════════════════════════

┌─────────────────────────┬────────┬────────┬────────┬────────┬────────┐
│ Outcome                 │  Price │  Bid   │  Ask   │ Spread │  Prob  │
├─────────────────────────┼────────┼────────┼────────┼────────┼────────┤
│ Canada                  │ 0.4200 │ 0.4150 │ 0.4250 │ 0.0100 │ 42.0%  │
│ Draw                    │ 0.2300 │ 0.2280 │ 0.2320 │ 0.0040 │ 23.0%  │
│ Bosnia and Herzegovina  │ 0.3500 │ 0.3480 │ 0.3520 │ 0.0040 │ 35.0%  │
└─────────────────────────┴────────┴────────┴────────┴────────┴────────┘
  Total implied prob: 100.0%

  ── Canada Order Book ──
   500 │                 ░░░░░
   250 │       ██████░░░░░░░░░░░
     0 │
        └──────────────────────────
         0.0  0.2  0.4  0.6  0.8  1.0
  Bids: 1823  Asks: 2104  Spread: 0.0100

  ── Draw Order Book ──
  (same format)

  ── Bosnia Order Book ──
  (same format)

  Last Trades:
  [14:23:01] Canada  BUY  0.4200 x 150.00
  [14:22:58] Bosnia  SELL 0.3500 x  80.00
```

- **Prob** column = mid-price expressed as percentage `(bid+ask)/2 * 100`
- **Total implied prob** = sum of all mid-prices; should be ~100% in a well-arbitraged market
- Order book chart: same ASCII bar chart as `go-polymarket-watcher` (`█` bids, `░` asks, `▓` both)
- Screen clears on each render tick (100ms)

## CLI Flags

```
pm-worldcup -slug <slug>          # required: market slug from Polymarket URL
pm-worldcup -slug <slug> -ob      # order book charts only (no price table or trades)
```

## Error Handling

- Market not found → print error and exit
- WebSocket disconnect → log warning, attempt reconnect (3 retries with 2s backoff)
- Missing `outcomes` field in gamma response → fall back to labeling outcomes as "Outcome 1", "Outcome 2", etc.

## Out of Scope

- Authentication / trading
- Stop-loss analysis
- BTC price feeds
- Metrics (VWAP, imbalance, etc.)
- Saving historical data
