package clob

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"pm-worldcup/market"
)

const wsURL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"

type MessageHandler func(eventType string, data json.RawMessage)

type WSClient struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	running    bool
	assetIDs   []string
	onMessage  MessageHandler
	pingTicker *time.Ticker
	done       chan struct{}
}

func NewWSClient(handler MessageHandler) *WSClient {
	return &WSClient{
		onMessage: handler,
		done:      make(chan struct{}),
	}
}

func (c *WSClient) Connect() error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	c.conn = conn
	c.running = true
	return nil
}

func (c *WSClient) Subscribe(assetIDs []string) error {
	c.assetIDs = assetIDs

	msg := map[string]interface{}{
		"assets_ids": assetIDs,
		"type":       "market",
	}

	c.mu.Lock()
	err := c.conn.WriteJSON(msg)
	c.mu.Unlock()

	if err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}
	return nil
}

func (c *WSClient) Start() {
	go c.readLoop()
	go c.pingLoop()
}

func (c *WSClient) readLoop() {
	for c.running {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if c.running {
				continue
			}
			return
		}

		if string(message) == "PONG" {
			continue
		}

		c.handleMessage(message)
	}
}

func (c *WSClient) handleMessage(data []byte) {
	var msgs []json.RawMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		var single json.RawMessage
		if err := json.Unmarshal(data, &single); err != nil {
			return
		}
		msgs = []json.RawMessage{single}
	}

	for _, raw := range msgs {
		var envelope struct {
			EventType string `json:"event_type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if c.onMessage != nil {
			c.onMessage(envelope.EventType, raw)
		}
	}
}

func (c *WSClient) pingLoop() {
	c.pingTicker = time.NewTicker(10 * time.Second)
	defer c.pingTicker.Stop()

	for {
		select {
		case <-c.pingTicker.C:
			c.mu.Lock()
			if c.conn != nil && c.running {
				c.conn.WriteMessage(websocket.TextMessage, []byte("PING"))
			}
			c.mu.Unlock()
		case <-c.done:
			return
		}
	}
}

func (c *WSClient) Close() {
	c.running = false
	close(c.done)
	if c.conn != nil {
		c.conn.Close()
	}
}

type BookMessage struct {
	EventType string       `json:"event_type"`
	AssetID   string       `json:"asset_id"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp string       `json:"timestamp"`
}

type PriceLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type PriceChangeMessage struct {
	EventType    string        `json:"event_type"`
	Timestamp    string        `json:"timestamp"`
	PriceChanges []PriceChange `json:"price_changes"`
}

type PriceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`
	Size    string `json:"size"`
	Side    string `json:"side"`
	BestBid string `json:"best_bid"`
	BestAsk string `json:"best_ask"`
}

type TradeMessage struct {
	EventType string `json:"event_type"`
	AssetID   string `json:"asset_id"`
	Price     string `json:"price"`
	Size      string `json:"size"`
	Side      string `json:"side"`
	Timestamp string `json:"timestamp"`
}

func ParseBookMessage(data json.RawMessage) (*BookMessage, error) {
	var msg BookMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func ParsePriceChangeMessage(data json.RawMessage) (*PriceChangeMessage, error) {
	var msg PriceChangeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func ParseTradeMessage(data json.RawMessage) (*market.Trade, error) {
	var msg TradeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	ts, _ := strconv.ParseInt(msg.Timestamp, 10, 64)

	return &market.Trade{
		Timestamp: ts,
		AssetID:   msg.AssetID,
		Price:     msg.Price,
		Size:      msg.Size,
		Side:      msg.Side,
	}, nil
}
