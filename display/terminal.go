package display

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pm-worldcup/market"
	"pm-worldcup/trade"
)

var (
	styleBold = lipgloss.NewStyle().Bold(true)
	styleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleBid  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleAsk  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleBuy  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	styleSell = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	barBid  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("█")
	barAsk  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("░")
	barBoth = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("▓")
)

type Tab int

const (
	TabMonitor Tab = iota
	TabTrading
	TabOrders
)

type tickMsg time.Time

type Model struct {
	Info        market.Market
	State       *market.State
	ShowOB      bool
	width       int
	activeTab   Tab
	cursorRow   int
	TradeClient *trade.Client
	Form        TradeForm
	Orders      OrdersView
}

func NewModel(info market.Market, state *market.State, showOB bool, tradeClient *trade.Client) Model {
	tokenIDs := make([]string, len(info.Outcomes))
	for i, o := range info.Outcomes {
		tokenIDs[i] = o.TokenID
	}
	return Model{
		Info:        info,
		State:       state,
		ShowOB:      showOB,
		TradeClient: tradeClient,
		Form:        newTradeForm(info),
		Orders:      newOrdersView(tradeClient, tokenIDs),
	}
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
		m.Form, _ = m.Form.onTick()
		m.Orders, _ = m.Orders.onTick(m.TradeClient)
		return m, doTick()

	case orderRefreshMsg:
		m.Orders.OpenOrders = msg.open
		m.Orders.FilledOrders = msg.filled
		m.Orders.RefreshMsg = ""
		return m, nil

	case orderSubmitMsg:
		if msg.err != nil {
			m.Form.state = FormError
			m.Form.statusMsg = msg.err.Error()
		} else {
			m.Form.state = FormSuccess
			m.Form.statusMsg = fmt.Sprintf("Orden enviada: %s", msg.orderID)
			m.Form.successTick = 20
		}
		return m, nil

	case negRiskFetchedMsg:
		m.Form.negRisk = msg.negRisk
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.activeTab == TabMonitor {
				return m, tea.Quit
			}
		case "1":
			m.activeTab = TabMonitor
			return m, nil
		case "2":
			m.activeTab = TabTrading
			return m, nil
		case "3":
			m.activeTab = TabOrders
			return m, fetchOrders(m.TradeClient, m.Orders.TokenIDs)
		case "tab":
			m.activeTab = (m.activeTab + 1) % 3
			if m.activeTab == TabOrders {
				return m, fetchOrders(m.TradeClient, m.Orders.TokenIDs)
			}
			return m, nil
		}

		switch m.activeTab {
		case TabMonitor:
			switch msg.String() {
			case "up", "k":
				if m.cursorRow > 0 {
					m.cursorRow--
				}
			case "down", "j":
				if m.cursorRow < len(m.Info.Outcomes)-1 {
					m.cursorRow++
				}
			case "o":
				m.ShowOB = !m.ShowOB
			case "b":
				snap := m.State.SnapshotOne(m.Info.Outcomes[m.cursorRow])
				m.Form = m.Form.setOutcome(m.cursorRow, true, snap.Book.BestAsk)
				m.activeTab = TabTrading
				return m, fetchNegRisk(m.TradeClient, m.Info.Outcomes[m.cursorRow].TokenID)
			case "s":
				snap := m.State.SnapshotOne(m.Info.Outcomes[m.cursorRow])
				m.Form = m.Form.setOutcome(m.cursorRow, false, snap.Book.BestBid)
				m.activeTab = TabTrading
				return m, fetchNegRisk(m.TradeClient, m.Info.Outcomes[m.cursorRow].TokenID)
			}
		case TabTrading:
			var cmd tea.Cmd
			m.Form, cmd = m.Form.update(msg, m.TradeClient, m.State, m.Info.Outcomes)
			return m, cmd
		case TabOrders:
			var cmd tea.Cmd
			m.Orders, cmd = m.Orders.update(msg, m.TradeClient)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
	}
	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString(renderTabBar(m.activeTab, m.TradeClient != nil))
	sb.WriteString("\n")
	switch m.activeTab {
	case TabMonitor:
		sb.WriteString(renderMonitor(m))
	case TabTrading:
		sb.WriteString(renderTrading(m))
	case TabOrders:
		sb.WriteString(renderOrders(m))
	}
	return sb.String()
}

func renderTabBar(active Tab, hasCredentials bool) string {
	type tabDef struct {
		key   string
		label string
		tab   Tab
	}
	tabs := []tabDef{
		{"1", "MONITOR", TabMonitor},
		{"2", "TRADING", TabTrading},
		{"3", "ÓRDENES", TabOrders},
	}

	var parts []string
	for _, t := range tabs {
		text := "[ " + t.key + " " + t.label + " ]"
		if t.tab == active {
			parts = append(parts, styleBold.Foreground(lipgloss.Color("82")).Render(text))
		} else if (t.tab == TabTrading || t.tab == TabOrders) && !hasCredentials {
			parts = append(parts, styleDim.Render(text))
		} else {
			parts = append(parts, styleDim.Render(text))
		}
	}
	return "  " + strings.Join(parts, "  ")
}
