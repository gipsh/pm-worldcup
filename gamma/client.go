package gamma

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// eventSubMarket is one binary market within a negRisk event.
type eventSubMarket struct {
	Slug         string `json:"slug"`
	ClobTokenIDs string `json:"clobTokenIds"`
}

type eventResponse struct {
	Slug    string           `json:"slug"`
	Title   string           `json:"title"`
	Closed  bool             `json:"closed"`
	EndDate string           `json:"endDate"`
	Volume  float64          `json:"volume"`
	Markets []eventSubMarket `json:"markets"`
}

func (c *Client) FetchMarketBySlug(slug string) (*market.Market, error) {
	// Try direct market lookup first.
	m, err := c.fetchSingleMarket(slug)
	if err == nil {
		return m, nil
	}
	// Fall back to negRisk event lookup (e.g. fifwc-usa-par-2026-06-12).
	return c.fetchEventMarket(slug)
}

func (c *Client) fetchSingleMarket(slug string) (*market.Market, error) {
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

func (c *Client) fetchEventMarket(slug string) (*market.Market, error) {
	url := fmt.Sprintf("%s/events?slug=%s", baseURL, slug)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("event request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	var events []eventResponse
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("event decode failed: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("market or event not found: %s", slug)
	}
	return parseEvent(events[0])
}

func parseEvent(e eventResponse) (*market.Market, error) {
	outcomes := make([]market.Outcome, 0, len(e.Markets))
	for _, m := range e.Markets {
		var tokenIDs []string
		if err := json.Unmarshal([]byte(m.ClobTokenIDs), &tokenIDs); err != nil || len(tokenIDs) == 0 {
			continue
		}
		// YES token is index 0; it represents the outcome probability.
		name := outcomeNameFromSlug(e.Slug, m.Slug)
		outcomes = append(outcomes, market.Outcome{Name: name, TokenID: tokenIDs[0]})
	}
	if len(outcomes) == 0 {
		return nil, fmt.Errorf("event has no parseable markets: %s", e.Slug)
	}
	endDate, _ := time.Parse(time.RFC3339, e.EndDate)
	return &market.Market{
		Slug:     e.Slug,
		Question: e.Title,
		Outcomes: outcomes,
		Closed:   e.Closed,
		EndDate:  endDate,
		Volume:   fmt.Sprintf("%.2f", e.Volume),
	}, nil
}

// outcomeNameFromSlug derives a short outcome label by stripping the event slug prefix.
// e.g. eventSlug="fifwc-usa-par-2026-06-12", marketSlug="fifwc-usa-par-2026-06-12-draw" → "DRAW"
func outcomeNameFromSlug(eventSlug, marketSlug string) string {
	suffix := strings.TrimPrefix(marketSlug, eventSlug+"-")
	if suffix == marketSlug {
		return strings.ToUpper(marketSlug)
	}
	return strings.ToUpper(suffix)
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
