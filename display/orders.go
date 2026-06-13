package display

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pm-worldcup/trade"
)

type OrdersView struct {
	TokenIDs     []string
	OpenOrders   []trade.Order
	FilledOrders []trade.Order
	cursor       int
	confirmMode  string // "" | "one" | "all"
	RefreshMsg   string
	ticksSince   int
}

type orderRefreshMsg struct {
	open   []trade.Order
	filled []trade.Order
}

func newOrdersView(client *trade.Client, tokenIDs []string) OrdersView {
	return OrdersView{TokenIDs: tokenIDs}
}

func (ov OrdersView) onTick(client *trade.Client) (OrdersView, tea.Cmd) {
	ov.ticksSince++
	if client != nil && ov.ticksSince >= 50 { // every 5s (50 * 100ms ticks)
		ov.ticksSince = 0
		return ov, fetchOrders(client, ov.TokenIDs)
	}
	return ov, nil
}

func (ov OrdersView) update(msg tea.KeyMsg, client *trade.Client) (OrdersView, tea.Cmd) {
	if ov.confirmMode == "one" {
		switch msg.String() {
		case "s", "S":
			ov.confirmMode = ""
			if client != nil && ov.cursor < len(ov.OpenOrders) {
				orderID := ov.OpenOrders[ov.cursor].ID
				return ov, cancelOneOrder(client, orderID, ov.TokenIDs)
			}
		default:
			ov.confirmMode = ""
		}
		return ov, nil
	}
	if ov.confirmMode == "all" {
		switch msg.String() {
		case "s", "S":
			ov.confirmMode = ""
			return ov, cancelAllOrders(client, ov.TokenIDs)
		default:
			ov.confirmMode = ""
		}
		return ov, nil
	}

	switch msg.String() {
	case "up", "k":
		if ov.cursor > 0 {
			ov.cursor--
		}
	case "down", "j":
		if ov.cursor < len(ov.OpenOrders)-1 {
			ov.cursor++
		}
	case "c":
		if len(ov.OpenOrders) > 0 {
			ov.confirmMode = "one"
		}
	case "C":
		if len(ov.OpenOrders) > 0 {
			ov.confirmMode = "all"
		}
	case "r":
		ov.RefreshMsg = "Actualizando..."
		return ov, fetchOrders(client, ov.TokenIDs)
	}
	return ov, nil
}

func fetchOrders(client *trade.Client, tokenIDs []string) tea.Cmd {
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		open, _ := client.GetOpenOrders(tokenIDs)
		filled, _ := client.GetFilledOrders(tokenIDs)
		return orderRefreshMsg{open: open, filled: filled}
	}
}

func cancelOneOrder(client *trade.Client, orderID string, tokenIDs []string) tea.Cmd {
	return func() tea.Msg {
		_ = client.CancelOrder(orderID)
		open, _ := client.GetOpenOrders(tokenIDs)
		filled, _ := client.GetFilledOrders(tokenIDs)
		return orderRefreshMsg{open: open, filled: filled}
	}
}

func cancelAllOrders(client *trade.Client, tokenIDs []string) tea.Cmd {
	return func() tea.Msg {
		_ = client.CancelAll()
		open, _ := client.GetOpenOrders(tokenIDs)
		filled, _ := client.GetFilledOrders(tokenIDs)
		return orderRefreshMsg{open: open, filled: filled}
	}
}

func renderOrders(m Model) string {
	ov := m.Orders
	nameByToken := make(map[string]string, len(m.Info.Outcomes))
	for _, o := range m.Info.Outcomes {
		nameByToken[o.TokenID] = o.Name
	}

	if m.TradeClient == nil {
		return "\n\n  " + styleWarn.Render("✗ Credenciales no configuradas.") +
			"\n\n  " + styleDim.Render("Exportá POLY_PRIVATE_KEY y POLY_PROXY_ADDRESS y reiniciá.")
	}

	sep := strings.Repeat("═", 64)
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s\n  %s\n%s\n",
		sep,
		styleBold.Render("ÓRDENES — "+m.Info.Slug),
		sep,
	))

	if ov.RefreshMsg != "" {
		sb.WriteString("\n  " + styleWarn.Render(ov.RefreshMsg) + "\n")
	}

	hint := "c=cancelar  C=cancelar todo"
	sb.WriteString(fmt.Sprintf("\n  %s  (%d)  %s\n\n",
		styleBold.Render("Pendientes"),
		len(ov.OpenOrders),
		styleDim.Render(hint),
	))

	if len(ov.OpenOrders) == 0 {
		sb.WriteString(styleDim.Render("    (sin órdenes pendientes)") + "\n")
	} else {
		sb.WriteString(renderOrderTable(ov.OpenOrders, nameByToken, ov.cursor, true))
	}

	if ov.confirmMode == "one" && len(ov.OpenOrders) > 0 {
		sb.WriteString("\n  " + styleWarn.Render(fmt.Sprintf("¿Cancelar orden %s? [s/N]",
			shortID(ov.OpenOrders[ov.cursor].ID))))
	}
	if ov.confirmMode == "all" {
		sb.WriteString("\n  " + styleWarn.Render(fmt.Sprintf("¿Cancelar TODAS las órdenes (%d)? [s/N]",
			len(ov.OpenOrders))))
	}

	sb.WriteString(fmt.Sprintf("\n\n  %s  (%d)\n\n",
		styleBold.Render("Ejecutadas"),
		len(ov.FilledOrders),
	))

	if len(ov.FilledOrders) == 0 {
		sb.WriteString(styleDim.Render("    (sin órdenes ejecutadas)") + "\n")
	} else {
		sb.WriteString(renderOrderTable(ov.FilledOrders, nameByToken, -1, false))
	}

	sb.WriteString("\n")
	sb.WriteString(styleDim.Render("  r=refresh  ↑↓=mover  c=cancelar  C=cancelar todo  q=salir"))
	return sb.String()
}

func renderOrderTable(orders []trade.Order, nameByToken map[string]string, cursorRow int, showCursor bool) string {
	var sb strings.Builder
	sb.WriteString(styleDim.Render("  ┌──────┬──────┬────────┬────────┬──────────┬───────────┐") + "\n")
	sb.WriteString(fmt.Sprintf("  │ %s │ %s │ %s │ %s │ %s │ %s │\n",
		styleBold.Render(fmt.Sprintf("%-4s", "Out")),
		styleBold.Render(fmt.Sprintf("%-4s", "Lado")),
		styleBold.Render(fmt.Sprintf("%-6s", "Tipo")),
		styleBold.Render(fmt.Sprintf("%-6s", "Precio")),
		styleBold.Render(fmt.Sprintf("%-8s", "Qty")),
		styleBold.Render(fmt.Sprintf("%-9s", "Enviada")),
	))
	sb.WriteString(styleDim.Render("  ├──────┼──────┼────────┼────────┼──────────┼───────────┤") + "\n")

	for i, o := range orders {
		name := nameByToken[o.TokenID]
		if name == "" {
			name = shortID(o.TokenID)
		}

		var sideStyled string
		if o.Side == "BUY" {
			sideStyled = styleBuy.Render(fmt.Sprintf("%-4s", o.Side))
		} else {
			sideStyled = styleSell.Render(fmt.Sprintf("%-4s", o.Side))
		}

		qty := fmt.Sprintf("%s/%s", formatSize(o.SizeMatched), formatSize(o.OriginalSize))
		ts := formatOrderTime(o.CreatedAt)
		orderType := o.Type
		if orderType == "" {
			orderType = "LIMIT"
		}

		cursor := " "
		if showCursor && i == cursorRow {
			cursor = styleWarn.Render("▶")
		}

		sb.WriteString(fmt.Sprintf("  │%s%-4s│ %s │ %-6s │ %-6s │ %-8s │ %-9s │\n",
			cursor,
			fmt.Sprintf("%-3s", name),
			sideStyled,
			orderType,
			formatPrice(o.Price),
			qty,
			ts,
		))
	}
	sb.WriteString(styleDim.Render("  └──────┴──────┴────────┴────────┴──────────┴───────────┘") + "\n")
	return sb.String()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func formatOrderTime(ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		if len(ts) > 9 {
			return ts[:9]
		}
		return ts
	}
	return t.Format("15:04:05")
}
