package display

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pm-worldcup/market"
)

var (
	styleBold = lipgloss.NewStyle().Bold(true)
	styleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleBid  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleAsk  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleBuy  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	styleSell = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	// pre-rendered colored chart bars
	barBid  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("█")
	barAsk  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("░")
	barBoth = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("▓")
)

func styleProb(p float64) lipgloss.Style {
	switch {
	case p >= 0.5:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	case p >= 0.25:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	default:
		return styleDim
	}
}

type tickMsg time.Time

// Model is the bubbletea model for the market monitor.
type Model struct {
	Info   market.Market
	State  *market.State
	ShowOB bool
	width  int
}

// NewModel creates a new display model.
func NewModel(info market.Market, state *market.State, showOB bool) Model {
	return Model{Info: info, State: state, ShowOB: showOB}
}

func (m Model) Init() tea.Cmd {
	return doTick()
}

func doTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, doTick()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "o":
			m.ShowOB = !m.ShowOB
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	}
	return m, nil
}

func (m Model) View() string {
	snaps, trades := m.State.Snapshot(m.Info.Outcomes)
	var sb strings.Builder
	sb.WriteString(renderHeader(m.Info))
	sb.WriteString("\n\n")
	sb.WriteString(renderPriceTable(snaps))
	if m.ShowOB {
		sb.WriteString("\n\n")
		sb.WriteString(renderCharts(snaps))
	}
	sb.WriteString("\n\n")
	sb.WriteString(renderTrades(snaps, trades))
	sb.WriteString("\n\n")
	sb.WriteString(styleDim.Render("  q quit  o toggle order book"))
	return sb.String()
}

func renderHeader(info market.Market) string {
	remaining := time.Until(info.EndDate)
	var timeStr string
	if info.Closed || remaining <= 0 {
		timeStr = styleWarn.Render("CLOSED")
	} else {
		timeStr = formatDuration(remaining)
	}
	sep := strings.Repeat("═", 64)
	line1 := fmt.Sprintf("  %s    Time: %s    Vol: %s",
		styleBold.Render(info.Slug), timeStr, formatVolume(info.Volume))
	line2 := styleDim.Render("  " + info.Question)
	return sep + "\n" + line1 + "\n" + line2 + "\n" + sep
}

func renderPriceTable(snaps []market.OutcomeSnapshot) string {
	maxName := 7
	for _, s := range snaps {
		if len(s.Outcome.Name) > maxName {
			maxName = len(s.Outcome.Name)
		}
	}
	pad := strings.Repeat("─", maxName+2)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("┌─%s─┬────────┬────────┬────────┬────────┬────────┐\n", pad))
	sb.WriteString(fmt.Sprintf("│ %s │  Price │  Bid   │  Ask   │ Spread │  Prob  │\n",
		styleBold.Render(fmt.Sprintf("%-*s", maxName, "Outcome"))))
	sb.WriteString(fmt.Sprintf("├─%s─┼────────┼────────┼────────┼────────┼────────┤\n", pad))

	totalMid := 0.0
	for _, s := range snaps {
		mid := calcMid(s.Book.BestBid, s.Book.BestAsk)
		totalMid += mid
		paddedName := fmt.Sprintf("%-*s", maxName, s.Outcome.Name)
		sb.WriteString(fmt.Sprintf("│ %s │ %s │ %s │ %s │ %s │ %s │\n",
			styleBold.Render(paddedName),
			formatPrice(s.LastPrice),
			styleBid.Render(formatPrice(s.Book.BestBid)),
			styleAsk.Render(formatPrice(s.Book.BestAsk)),
			calcSpread(s.Book.BestBid, s.Book.BestAsk),
			styleProb(mid).Render(fmt.Sprintf("%5.1f%%", mid*100)),
		))
	}
	sb.WriteString(fmt.Sprintf("└─%s─┴────────┴────────┴────────┴────────┴────────┘\n", pad))
	sb.WriteString(styleDim.Render(fmt.Sprintf("  Total implied prob: %.1f%%", totalMid*100)))
	return sb.String()
}

func renderCharts(snaps []market.OutcomeSnapshot) string {
	var sb strings.Builder
	for _, s := range snaps {
		sb.WriteString(styleBold.Render(fmt.Sprintf("  ── %s Order Book ──", s.Outcome.Name)) + "\n")
		sb.WriteString(renderBookChart(s.Book))
	}
	return sb.String()
}

func renderBookChart(book market.Orderbook) string {
	bidVols := make(map[int]float64)
	askVols := make(map[int]float64)

	for _, level := range book.Bids {
		price, _ := strconv.ParseFloat(level.Price, 64)
		size, _ := strconv.ParseFloat(level.Size, 64)
		bidVols[int(math.Round(price*100))] += size
	}
	for _, level := range book.Asks {
		price, _ := strconv.ParseFloat(level.Price, 64)
		size, _ := strconv.ParseFloat(level.Size, 64)
		askVols[int(math.Round(price*100))] += size
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
		return styleDim.Render("    (no data)") + "\n"
	}
	if minBucket > 2 {
		minBucket -= 2
	}
	if maxBucket < 98 {
		maxBucket += 2
	}

	chartHeight := 12
	bucketRange := maxBucket - minBucket + 1
	var sb strings.Builder

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
		line := "    " + styleDim.Render(label)
		for bucket := minBucket; bucket <= maxBucket; bucket++ {
			switch {
			case bidVols[bucket] >= threshold && askVols[bucket] >= threshold:
				line += barBoth
			case bidVols[bucket] >= threshold:
				line += barBid
			case askVols[bucket] >= threshold:
				line += barAsk
			default:
				line += " "
			}
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString(styleDim.Render(fmt.Sprintf("           └%s", strings.Repeat("─", bucketRange))) + "\n")
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
	sb.WriteString(styleDim.Render(labelLine) + "\n")
	sb.WriteString(
		styleDim.Render("    Legend: ") +
			barBid + styleDim.Render(" Bids  ") +
			barAsk + styleDim.Render(" Asks  ") +
			barBoth + styleDim.Render(" Both") + "\n")
	bidTotal, askTotal := calcBookTotals(book)
	sb.WriteString(styleDim.Render(fmt.Sprintf("    Bids: %.0f  Asks: %.0f  Spread: %s",
		bidTotal, askTotal, calcSpread(book.BestBid, book.BestAsk))) + "\n")
	return sb.String()
}

func renderTrades(snaps []market.OutcomeSnapshot, trades []market.Trade) string {
	nameByToken := make(map[string]string, len(snaps))
	for _, s := range snaps {
		nameByToken[s.Outcome.TokenID] = s.Outcome.Name
	}

	var sb strings.Builder
	sb.WriteString(styleBold.Render("  Last Trades:") + "\n")
	if len(trades) == 0 {
		sb.WriteString(styleDim.Render("    No trades yet..."))
		return sb.String()
	}
	for i := len(trades) - 1; i >= 0; i-- {
		tr := trades[i]
		ts := time.UnixMilli(tr.Timestamp).Format("15:04:05")
		name := nameByToken[tr.AssetID]
		if name == "" && len(tr.AssetID) >= 8 {
			name = tr.AssetID[:8] + "..."
		}

		side := strings.ToUpper(tr.Side)
		paddedSide := fmt.Sprintf("%-4s", side)
		var sideStyled string
		if side == "BUY" || side == "ADD" {
			sideStyled = styleBuy.Render(paddedSide)
		} else {
			sideStyled = styleSell.Render(paddedSide)
		}

		paddedName := fmt.Sprintf("%-24s", name)
		sb.WriteString(fmt.Sprintf("    [%s] %s %s %s x %s\n",
			styleDim.Render(ts),
			styleBold.Render(paddedName),
			sideStyled,
			formatPrice(tr.Price),
			formatSize(tr.Size),
		))
	}
	return sb.String()
}

// --- helpers ---

func calcMid(bid, ask string) float64 {
	b, e1 := strconv.ParseFloat(bid, 64)
	a, e2 := strconv.ParseFloat(ask, 64)
	if e1 != nil || e2 != nil {
		return 0
	}
	return (b + a) / 2
}

func calcSpread(bid, ask string) string {
	b, e1 := strconv.ParseFloat(bid, 64)
	a, e2 := strconv.ParseFloat(ask, 64)
	if e1 != nil || e2 != nil {
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
