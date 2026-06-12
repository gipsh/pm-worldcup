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
