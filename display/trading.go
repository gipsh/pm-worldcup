package display

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pm-worldcup/market"
	"pm-worldcup/trade"
)

type FormState int

const (
	FormEditing FormState = iota
	FormSubmitting
	FormSuccess
	FormError
)

type TradeForm struct {
	info        market.Market
	outcomeIdx  int
	isMarket    bool
	isBuy       bool
	price       string
	qty         string
	activeField int // 0=tipo, 1=lado, 2=precio, 3=qty, 4=enviar
	state       FormState
	statusMsg   string
	successTick int
	negRisk     bool
}

func newTradeForm(info market.Market) TradeForm {
	return TradeForm{info: info, isBuy: true}
}

func (f TradeForm) setOutcome(idx int, isBuy bool, prefillPrice string) TradeForm {
	f.outcomeIdx = idx
	f.isBuy = isBuy
	f.price = prefillPrice
	f.qty = ""
	f.activeField = 3
	f.state = FormEditing
	f.statusMsg = ""
	return f
}

func (f TradeForm) onTick() (TradeForm, tea.Cmd) {
	if f.state == FormSuccess {
		f.successTick--
		if f.successTick <= 0 {
			f.state = FormEditing
			f.qty = ""
			f.statusMsg = ""
		}
	}
	return f, nil
}

func (f TradeForm) update(msg tea.KeyMsg, client *trade.Client, state *market.State, outcomes []market.Outcome) (TradeForm, tea.Cmd) {
	if f.state == FormSubmitting {
		return f, nil
	}
	if f.state == FormError || f.state == FormSuccess {
		f.state = FormEditing
		return f, nil
	}

	switch msg.String() {
	case "tab", "down":
		maxField := 4
		if f.isMarket {
			maxField = 3
		}
		f.activeField = (f.activeField + 1) % (maxField + 1)
		if f.isMarket && f.activeField == 2 {
			f.activeField = 3
		}
	case "shift+tab", "up":
		f.activeField--
		if f.isMarket && f.activeField == 2 {
			f.activeField = 1
		}
		if f.activeField < 0 {
			f.activeField = 4
		}
	case "left", "right":
		switch f.activeField {
		case 0:
			f.isMarket = !f.isMarket
			if f.isMarket && f.activeField == 2 {
				f.activeField = 3
			}
		case 1:
			f.isBuy = !f.isBuy
			if len(outcomes) > f.outcomeIdx {
				snap := state.SnapshotOne(outcomes[f.outcomeIdx])
				if f.isBuy {
					f.price = snap.Book.BestAsk
				} else {
					f.price = snap.Book.BestBid
				}
			}
		}
	case "enter":
		if f.activeField == 4 {
			f.state = FormSubmitting
			return f, f.submitOrder(client, state, outcomes)
		}
		f.activeField++
	case "backspace":
		switch f.activeField {
		case 2:
			if len(f.price) > 0 {
				f.price = f.price[:len(f.price)-1]
			}
		case 3:
			if len(f.qty) > 0 {
				f.qty = f.qty[:len(f.qty)-1]
			}
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && (ch[0] >= '0' && ch[0] <= '9' || ch[0] == '.') {
			switch f.activeField {
			case 2:
				f.price += ch
			case 3:
				f.qty += ch
			}
		}
	}
	return f, nil
}

type orderSubmitMsg struct {
	orderID string
	err     error
}

func (f TradeForm) submitOrder(client *trade.Client, state *market.State, outcomes []market.Outcome) tea.Cmd {
	if client == nil {
		return nil
	}
	outcomeIdx := f.outcomeIdx
	if outcomeIdx >= len(outcomes) {
		outcomeIdx = 0
	}
	tokenID := outcomes[outcomeIdx].TokenID
	side := "BUY"
	if !f.isBuy {
		side = "SELL"
	}
	orderType := "LIMIT"
	price := f.price
	if f.isMarket {
		orderType = "MARKET"
		snap := state.SnapshotOne(outcomes[outcomeIdx])
		if f.isBuy {
			price = snap.Book.BestAsk
		} else {
			price = snap.Book.BestBid
		}
	}
	qty := f.qty
	negRisk := f.negRisk

	return func() tea.Msg {
		orderID, err := client.PlaceOrder(tokenID, side, orderType, price, qty, negRisk)
		return orderSubmitMsg{orderID: orderID, err: err}
	}
}

type negRiskFetchedMsg struct {
	negRisk bool
}

func fetchNegRisk(client *trade.Client, tokenID string) tea.Cmd {
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		nr, err := client.GetNegRisk(tokenID)
		if err != nil {
			return negRiskFetchedMsg{negRisk: false}
		}
		return negRiskFetchedMsg{negRisk: nr}
	}
}

func renderTrading(m Model) string {
	f := m.Form
	outcomes := m.Info.Outcomes

	if m.TradeClient == nil {
		return "\n\n  " + styleWarn.Render("✗ Credenciales no configuradas.") +
			"\n\n  " + styleDim.Render("Exportá POLY_PRIVATE_KEY y POLY_PROXY_ADDRESS y reiniciá.")
	}

	outcomeIdx := f.outcomeIdx
	if outcomeIdx >= len(outcomes) {
		outcomeIdx = 0
	}
	outcome := outcomes[outcomeIdx]
	snap := m.State.SnapshotOne(outcome)
	mid := calcMid(snap.Book.BestBid, snap.Book.BestAsk)

	sep := strings.Repeat("═", 64)
	header := fmt.Sprintf("%s\n  %s    Bid: %s  Ask: %s  (%s)\n%s",
		sep,
		styleBold.Render("TRADING — "+outcome.Name),
		styleBid.Render(formatPrice(snap.Book.BestBid)),
		styleAsk.Render(formatPrice(snap.Book.BestAsk)),
		styleProb(mid).Render(fmt.Sprintf("%.1f%%", mid*100)),
		sep,
	)

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(header)
	sb.WriteString("\n\n")

	if f.state == FormSubmitting {
		sb.WriteString(styleWarn.Render("  Enviando orden..."))
		return sb.String()
	}

	if f.state == FormSuccess {
		sb.WriteString(styleBid.Render("  ✓ " + f.statusMsg))
		return sb.String()
	}

	if f.state == FormError {
		sb.WriteString(styleAsk.Render("  ✗ " + f.statusMsg))
		sb.WriteString("\n\n  " + styleDim.Render("Presioná cualquier tecla para continuar."))
		return sb.String()
	}

	// Tipo field
	tipoStyle := styleDim
	if f.activeField == 0 {
		tipoStyle = styleBold
	}
	limitLabel := "LIMIT"
	marketLabel := "MARKET"
	if !f.isMarket {
		limitLabel = lipgloss.NewStyle().Background(lipgloss.Color("82")).Foreground(lipgloss.Color("0")).Render("LIMIT")
	} else {
		marketLabel = lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("0")).Render("MARKET")
	}
	sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", tipoStyle.Render("Tipo:  "), limitLabel, marketLabel))

	// Lado field
	ladoStyle := styleDim
	if f.activeField == 1 {
		ladoStyle = styleBold
	}
	buyLabel := "BUY"
	sellLabel := "SELL"
	if f.isBuy {
		buyLabel = lipgloss.NewStyle().Background(lipgloss.Color("82")).Foreground(lipgloss.Color("0")).Render("BUY")
	} else {
		sellLabel = lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("0")).Render("SELL")
	}
	sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", ladoStyle.Render("Lado:  "), buyLabel, sellLabel))

	// Precio field (hidden for MARKET)
	if !f.isMarket {
		precioStyle := styleDim
		if f.activeField == 2 {
			precioStyle = styleBold
		}
		cursor := ""
		if f.activeField == 2 {
			cursor = "█"
		}
		sb.WriteString(fmt.Sprintf("  %s  %s%s\n", precioStyle.Render("Precio:"), f.price, styleDim.Render(cursor)))
	}

	// Qty field
	qtyStyle := styleDim
	if f.activeField == 3 {
		qtyStyle = styleBold
	}
	qtyCursor := ""
	if f.activeField == 3 {
		qtyCursor = "█"
	}
	sb.WriteString(fmt.Sprintf("  %s  %s%s  USDC\n", qtyStyle.Render("Qty:   "), f.qty, styleDim.Render(qtyCursor)))

	// Total estimado
	if qty, err := strconv.ParseFloat(f.qty, 64); err == nil && qty > 0 {
		price := f.price
		if f.isMarket {
			if f.isBuy {
				price = snap.Book.BestAsk
			} else {
				price = snap.Book.BestBid
			}
		}
		if p, err2 := strconv.ParseFloat(price, 64); err2 == nil {
			totalStr := fmt.Sprintf("$%.2f", qty*p)
			if f.isMarket {
				totalStr += " (estimado)"
			}
			sb.WriteString(fmt.Sprintf("\n  %s  %s\n", styleDim.Render("Total: "), totalStr))
		}
	}

	// Enviar button
	sb.WriteString("\n")
	var enviarStyle lipgloss.Style
	if f.activeField == 4 {
		enviarStyle = lipgloss.NewStyle().Background(lipgloss.Color("82")).Foreground(lipgloss.Color("0")).Bold(true)
	} else {
		enviarStyle = styleDim
	}
	sb.WriteString("  " + enviarStyle.Render("  Enviar orden  "))
	sb.WriteString("\n\n")
	sb.WriteString(styleDim.Render("  Tab/↑↓=navegar  ←→=cambiar opción  Enter=seleccionar  1=monitor"))

	return sb.String()
}
