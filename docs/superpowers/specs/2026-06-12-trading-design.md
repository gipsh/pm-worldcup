# pm-worldcup Trading Feature — Design Spec

**Date:** 2026-06-12  
**Status:** Approved

## Overview

Add buy/sell order placement (market and limit) and order management (pending + filled) to the existing pm-worldcup TUI. The monitor continues to work without credentials; trading features activate only when credentials are present.

## Architecture

```
pm-worldcup/
├── main.go                    # Add credential loading, pass to display
├── trade/
│   ├── auth.go                # Read env vars, derive L2 API credentials via EIP712
│   └── client.go              # CLOB HTTP client: place, list, cancel orders
└── display/
    ├── terminal.go            # Refactor: add tab model, route to sub-views
    ├── monitor.go             # Extracted from terminal.go: existing monitor view
    ├── trading.go             # New: Tab 2 — order form
    └── orders.go              # New: Tab 3 — pending + filled orders table
```

### Key constraints

- **Credentials are optional.** If `POLY_PRIVATE_KEY` / `POLY_PROXY_ADDRESS` are missing, Tab 2 and 3 show a "credenciales no configuradas" message. The monitor (Tab 1) is unaffected.
- **No new CLI flags** for credentials — env vars only (`POLY_PRIVATE_KEY`, `POLY_PROXY_ADDRESS`).
- Code reuse from `go-polymarket-tools/trade-console` for auth and HTTP signing.

## Authentication

**`trade/auth.go`**

Reads two environment variables:
- `POLY_PRIVATE_KEY` — Ethereum private key (hex, with or without `0x` prefix)
- `POLY_PROXY_ADDRESS` — Proxy wallet address

Derives L2 API credentials by calling `GET /auth/derive-api-key` on the CLOB API with an EIP712 signature. Returns `ApiCreds{Key, Secret, Passphrase}` or an error.

Called once at startup in `main.go`. If it fails, `trade.Client` is nil and the display shows a degraded state.

## Trade Client

**`trade/client.go`**

```go
type Client struct { /* host, apiCreds, httpClient, privateKey, proxyAddress */ }

func NewClient(privateKey, proxyAddress string) (*Client, error)

func (c *Client) PlaceOrder(tokenID, side, orderType, price, size string) (orderID string, err error)
// side: "BUY" | "SELL"
// orderType: "LIMIT" | "MARKET"
// price: ignored for MARKET orders

func (c *Client) GetOpenOrders(tokenIDs []string) ([]Order, error)
func (c *Client) GetFilledOrders(tokenIDs []string) ([]Order, error)
func (c *Client) CancelOrder(orderID string) error
func (c *Client) CancelAll(tokenIDs []string) error
```

```go
type Order struct {
    ID          string
    TokenID     string
    Side        string  // "BUY" | "SELL"
    Type        string  // "LIMIT" | "MARKET"
    Price       string
    OriginalQty string
    FilledQty   string
    Status      string  // "OPEN" | "MATCHED" | "CANCELLED"
    CreatedAt   int64   // unix ms
}
```

HTTP signing follows trade-console's HMAC-SHA256 L2 auth pattern (timestamp + method + path + body).

NegRisk flag is determined per-token via `GET /neg-risk?token_id={id}`, called once per token at startup and cached.

## Display — Tab Navigation

**`display/terminal.go`** is refactored to own tab state. `Model` gains:

```go
type Tab int
const (TabMonitor Tab = iota; TabTrading; TabOrders)

type Model struct {
    // existing fields
    activeTab   Tab
    tradeClient *trade.Client   // nil if no credentials
    form        TradeForm
    ordersView  OrdersView
}
```

Navigation:
- `1` → Tab 1 (Monitor)
- `2` → Tab 2 (Trading)
- `3` → Tab 3 (Orders)
- `Tab` / `Shift+Tab` → cycle tabs
- From Monitor: `b` on a focused row → open Tab 2 in BUY mode for that outcome
- From Monitor: `s` on a focused row → open Tab 2 in SELL mode for that outcome
- In Monitor, `↑`/`↓` move row cursor (no cursor shown today — new addition)

Tab bar shown at top of every view:
```
[ 1 MONITOR ]  [ 2 TRADING ]  [ 3 ÓRDENES ]
```
Active tab is highlighted with lipgloss bold + color.

## Tab 2 — Trading View

**`display/trading.go`**

If `tradeClient == nil`, show:
```
  ✗ Credenciales no configuradas.
  Exportá POLY_PRIVATE_KEY y POLY_PROXY_ADDRESS y reiniciá.
```

Otherwise, show the order form:

```
══════════════════════════════════════════════════════════
  TRADING — USA (46.5%)   Bid: 0.4600  Ask: 0.4700
══════════════════════════════════════════════════════════

  Tipo:    [LIMIT]  MARKET
  Lado:    [BUY]    SELL
  Precio:  0.4700
  Qty:     _____    USDC

  Total estimado: $47.00

  [Enviar orden]

  Esc / 1 = monitor   Tab = siguiente campo
```

**Form state machine:**
- `EDITING` — user filling fields
- `CONFIRMING` — not used (no extra confirm step — send on Enter at Enviar button)
- `SUBMITTING` — showing "Enviando..."
- `SUCCESS` — showing "✓ Orden enviada (#id)" for 2s then back to EDITING with cleared form
- `ERROR` — showing "✗ Error: <msg>" until user presses any key

**Field navigation:** Tab / Shift+Tab cycle through Tipo → Lado → Precio → Qty → Enviar. Left/Right on Tipo and Lado toggle between options. Precio and Qty accept numeric input with Backspace support.

**Price pre-fill logic:**
- BUY → pre-fill with current best ask of selected outcome
- SELL → pre-fill with current best bid
- Outcome selected from Monitor (`b`/`s`) is remembered in `form.outcomeIndex`

**MARKET orders:** Precio field is hidden. Total estimado shows "~$X.XX (estimado)" based on best ask/bid.

## Tab 3 — Orders View

**`display/orders.go`**

If `tradeClient == nil`, same "credenciales no configuradas" message.

Otherwise:

```
══════════════════════════════════════════════════════════
  ÓRDENES — fifwc-usa-par-2026-06-12
══════════════════════════════════════════════════════════

  Pendientes (2)                        c=cancelar  C=cancelar todo

  ┌──────┬──────┬────────┬────────┬──────────┬───────────┐
  │ Out  │ Lado │  Tipo  │ Precio │   Qty    │  Enviada  │
  ├──────┼──────┼────────┼────────┼──────────┼───────────┤
  │ USA  │ BUY  │ LIMIT  │ 0.4500 │ 100/100  │ 14:21:03  │ ← cursor
  │ DRAW │ BUY  │ LIMIT  │ 0.2800 │  50/50   │ 14:19:44  │
  └──────┴──────┴────────┴────────┴──────────┴───────────┘

  Ejecutadas (3)

  ┌──────┬──────┬────────┬────────┬──────────┬───────────┐
  │ USA  │ BUY  │ MARKET │ 0.4700 │ 200/200  │ 14:18:01  │
  │ ...                                                   │
  └──────┴──────┴────────┴────────┴──────────┴───────────┘

  r=refresh  ↑↓=mover  c=cancelar  C=cancelar todo  q=salir
```

- `↑`/`↓` move cursor over pending orders only
- `c` → inline confirmation "¿Cancelar esta orden? [s/N]" on same line; `s` calls `CancelOrder`, `N`/Esc dismisses
- `C` → inline confirmation "¿Cancelar TODAS (%d)? [s/N]"
- `r` → manual refresh
- Orders auto-refresh every 5 seconds via a `tea.Tick` cmd
- Filtered to token IDs of current market only
- Outcome name resolved from tokenID → outcome name map (from market info)
- BUY rows colored green, SELL rows colored red (lipgloss, matching existing style)

## Error Handling

- API errors shown inline in the view where they occur, never crash the program
- WebSocket and trading are independent goroutines — a trading error doesn't affect the live market data
- Malformed private key → error at startup printed to stderr before entering TUI
- Network timeout on order placement → ERROR state in form with message

## Out of Scope

- Position tracking / P&L calculation
- Order history across multiple sessions
- Multiple markets simultaneously
- Stop-loss or algorithmic orders
- Wallet balance display
