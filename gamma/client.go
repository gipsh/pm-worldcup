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
