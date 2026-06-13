package trade

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymarket/go-order-utils/pkg/model"
)

const clobHost = "https://clob.polymarket.com"

type Client struct {
	host          string
	chainID       int
	privateKey    *ecdsa.PrivateKey
	signerAddress string
	makerAddress  string
	signatureType model.SignatureType
	httpClient    *http.Client
	apiKey        string
	apiSecret     string
	apiPassphrase string
	orderBuilder  builder.ExchangeOrderBuilder
}

type Order struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	TokenID      string `json:"asset_id"`
	Side         string `json:"side"`
	Type         string `json:"type"`
	OriginalSize string `json:"original_size"`
	SizeMatched  string `json:"size_matched"`
	Price        string `json:"price"`
	CreatedAt    string `json:"created_at"`
}

type postOrderResponse struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	OrderID  string `json:"orderID"`
}

type apiCredsResponse struct {
	ApiKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

type negRiskResponse struct {
	NegRisk bool `json:"neg_risk"`
}

// NewClient parses the private key and proxy address, derives L2 API credentials,
// and returns a ready-to-use Client.
func NewClient(privateKeyHex, proxyAddress string) (*Client, error) {
	pk, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	signerAddress := crypto.PubkeyToAddress(pk.PublicKey).Hex()
	makerAddress := signerAddress
	if proxyAddress != "" {
		makerAddress = proxyAddress
	}

	c := &Client{
		host:          clobHost,
		chainID:       137,
		privateKey:    pk,
		signerAddress: signerAddress,
		makerAddress:  makerAddress,
		signatureType: model.SignatureType(0),
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		orderBuilder:  builder.NewExchangeOrderBuilderImpl(big.NewInt(137), nil),
	}

	if err := c.deriveApiCreds(); err != nil {
		if err2 := c.createApiCreds(); err2 != nil {
			return nil, fmt.Errorf("failed to get API credentials (derive: %v, create: %v)", err, err2)
		}
	}

	return c, nil
}

// GetNegRisk returns whether the token uses negRisk settlement.
func (c *Client) GetNegRisk(tokenID string) (bool, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/neg-risk?token_id=%s", c.host, tokenID))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result negRiskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.NegRisk, nil
}

// PlaceOrder places a limit or market order.
// orderType: "LIMIT" or "MARKET". For MARKET, pass current bid/ask as price.
// negRisk must be fetched via GetNegRisk() before calling.
func (c *Client) PlaceOrder(tokenID, side, orderType, price, size string, negRisk bool) (string, error) {
	priceF, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return "", fmt.Errorf("invalid price: %w", err)
	}
	sizeF, err := strconv.ParseFloat(size, 64)
	if err != nil {
		return "", fmt.Errorf("invalid size: %w", err)
	}
	if sizeF < 5 {
		return "", fmt.Errorf("minimum order size is 5 USDC, got %.2f", sizeF)
	}

	priceF = math.Round(priceF*100) / 100
	sizeF = math.Round(sizeF)

	var modelSide model.Side
	if strings.ToUpper(side) == "BUY" {
		modelSide = model.BUY
	} else {
		modelSide = model.SELL
	}

	makerAmt, takerAmt := calcOrderAmounts(priceF, sizeF, modelSide == model.BUY)

	orderData := &model.OrderData{
		Maker:         c.makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenId:       tokenID,
		MakerAmount:   strconv.FormatInt(int64(makerAmt), 10),
		TakerAmount:   strconv.FormatInt(int64(takerAmt), 10),
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        c.signerAddress,
		Expiration:    "0",
		Side:          modelSide,
		SignatureType: c.signatureType,
	}

	contract := model.CTFExchange
	if negRisk {
		contract = model.NegRiskCTFExchange
	}

	signedOrder, err := c.orderBuilder.BuildSignedOrder(c.privateKey, orderData, contract)
	if err != nil {
		return "", fmt.Errorf("failed to build order: %w", err)
	}

	sideStr := "BUY"
	if modelSide == model.SELL {
		sideStr = "SELL"
	}

	polyOrderType := "GTC"
	if strings.ToUpper(orderType) == "MARKET" {
		polyOrderType = "FOK"
	}

	payload := map[string]any{
		"order": map[string]any{
			"salt":          signedOrder.Salt.Int64(),
			"maker":         signedOrder.Maker.Hex(),
			"signer":        signedOrder.Signer.Hex(),
			"taker":         signedOrder.Taker.Hex(),
			"tokenId":       signedOrder.TokenId.String(),
			"makerAmount":   signedOrder.MakerAmount.String(),
			"takerAmount":   signedOrder.TakerAmount.String(),
			"expiration":    signedOrder.Expiration.String(),
			"nonce":         signedOrder.Nonce.String(),
			"feeRateBps":    signedOrder.FeeRateBps.String(),
			"side":          sideStr,
			"signatureType": int(signedOrder.SignatureType.Int64()),
			"signature":     "0x" + hex.EncodeToString(signedOrder.Signature),
		},
		"owner":     c.apiKey,
		"orderType": polyOrderType,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal order: %w", err)
	}

	path := "/order"
	headers, err := c.buildL2Headers("POST", path, bodyBytes)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.host+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to post order: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("order rejected (status %d): %s", resp.StatusCode, string(respBody))
	}

	var orderResp postOrderResponse
	if err := json.Unmarshal(respBody, &orderResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if !orderResp.Success {
		msg := orderResp.ErrorMsg
		if msg == "" {
			msg = string(respBody)
		}
		return "", fmt.Errorf("order rejected: %s", msg)
	}

	return orderResp.OrderID, nil
}

// GetOpenOrders returns open/pending orders filtered to the given token IDs.
func (c *Client) GetOpenOrders(tokenIDs []string) ([]Order, error) {
	path := "/orders"
	headers, err := c.buildL2Headers("GET", path, nil)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", c.host+path, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get orders failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var orders []Order
	if err := json.Unmarshal(respBody, &orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %w", err)
	}

	allowed := make(map[string]bool, len(tokenIDs))
	for _, id := range tokenIDs {
		allowed[id] = true
	}
	return filterByTokenIDs(orders, allowed), nil
}

// GetFilledOrders returns matched/filled orders by querying the maker's trading history.
func (c *Client) GetFilledOrders(tokenIDs []string) ([]Order, error) {
	path := "/data/trading-history"
	headers, err := c.buildL2Headers("GET", path, nil)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", c.host+path, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("maker_address", c.makerAddress)
	q.Set("limit", "50")
	req.URL.RawQuery = q.Encode()
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get trade history: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get trade history failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		History []Order `json:"history"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		var orders []Order
		if err2 := json.Unmarshal(respBody, &orders); err2 != nil {
			return nil, fmt.Errorf("failed to decode trade history: %w", err)
		}
		result.History = orders
	}

	allowed := make(map[string]bool, len(tokenIDs))
	for _, id := range tokenIDs {
		allowed[id] = true
	}
	return filterByTokenIDs(result.History, allowed), nil
}

// CancelOrder cancels a single order by ID.
func (c *Client) CancelOrder(orderID string) error {
	payload := map[string]string{"orderID": orderID}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	path := "/order"
	headers, err := c.buildL2Headers("DELETE", path, bodyBytes)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", c.host+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// CancelAll cancels all open orders.
func (c *Client) CancelAll() error {
	path := "/cancel-all"
	headers, err := c.buildL2Headers("DELETE", path, nil)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", c.host+path, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to cancel all: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel-all failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// MakerAddress returns the address used as order maker.
func (c *Client) MakerAddress() string { return c.makerAddress }

// --- internal helpers ---

func (c *Client) deriveApiCreds() error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "0"

	sig, err := c.buildEIP712Signature(timestamp, nonce)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", c.host+"/auth/derive-api-key", nil)
	if err != nil {
		return err
	}
	req.Header.Set("POLY_ADDRESS", c.signerAddress)
	req.Header.Set("POLY_SIGNATURE", sig)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_NONCE", nonce)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("derive-api-key status %d: %s", resp.StatusCode, string(body))
	}

	var creds apiCredsResponse
	if err := json.Unmarshal(body, &creds); err != nil {
		return err
	}
	c.apiKey = creds.ApiKey
	c.apiSecret = creds.Secret
	c.apiPassphrase = creds.Passphrase
	return nil
}

func (c *Client) createApiCreds() error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "0"

	sig, err := c.buildEIP712Signature(timestamp, nonce)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.host+"/auth/api-key", nil)
	if err != nil {
		return err
	}
	req.Header.Set("POLY_ADDRESS", c.signerAddress)
	req.Header.Set("POLY_SIGNATURE", sig)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_NONCE", nonce)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create-api-key status %d: %s", resp.StatusCode, string(body))
	}

	var creds apiCredsResponse
	if err := json.Unmarshal(body, &creds); err != nil {
		return err
	}
	c.apiKey = creds.ApiKey
	c.apiSecret = creds.Secret
	c.apiPassphrase = creds.Passphrase
	return nil
}

func (c *Client) buildEIP712Signature(timestamp, nonce string) (string, error) {
	domainSep := buildDomainSeparator(c.chainID)
	structHash := buildClobAuthStructHash(c.signerAddress, timestamp, nonce)

	digestInput := append([]byte("\x19\x01"), domainSep[:]...)
	digestInput = append(digestInput, structHash[:]...)
	digest := crypto.Keccak256Hash(digestInput)

	sig, err := crypto.Sign(digest.Bytes(), c.privateKey)
	if err != nil {
		return "", err
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return "0x" + hex.EncodeToString(sig), nil
}

func buildDomainSeparator(chainID int) [32]byte {
	typeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId)"))
	nameHash := crypto.Keccak256Hash([]byte("ClobAuthDomain"))
	versionHash := crypto.Keccak256Hash([]byte("1"))
	chainIDBytes := make([]byte, 32)
	big.NewInt(int64(chainID)).FillBytes(chainIDBytes)
	encoded := append(typeHash.Bytes(), nameHash.Bytes()...)
	encoded = append(encoded, versionHash.Bytes()...)
	encoded = append(encoded, chainIDBytes...)
	return crypto.Keccak256Hash(encoded)
}

func buildClobAuthStructHash(address, timestamp, nonce string) [32]byte {
	typeHash := crypto.Keccak256Hash([]byte("ClobAuth(address address,string timestamp,uint256 nonce,string message)"))
	addressBytes := make([]byte, 32)
	addrBytes, _ := hex.DecodeString(strings.TrimPrefix(address, "0x"))
	copy(addressBytes[12:], addrBytes)
	timestampHash := crypto.Keccak256Hash([]byte(timestamp))
	nonceBig := new(big.Int)
	nonceBig.SetString(nonce, 10)
	nonceBytes := make([]byte, 32)
	nonceBig.FillBytes(nonceBytes)
	messageHash := crypto.Keccak256Hash([]byte("This message attests that I control the given wallet"))
	encoded := append(typeHash.Bytes(), addressBytes...)
	encoded = append(encoded, timestampHash.Bytes()...)
	encoded = append(encoded, nonceBytes...)
	encoded = append(encoded, messageHash.Bytes()...)
	return crypto.Keccak256Hash(encoded)
}

func (c *Client) buildL2Headers(method, path string, body []byte) (map[string]string, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	message := timestamp + method + path
	if len(body) > 0 {
		message += string(body)
	}

	secretBytes, err := base64.URLEncoding.DecodeString(c.apiSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decode API secret: %w", err)
	}

	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(message))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	return map[string]string{
		"POLY_ADDRESS":    c.signerAddress,
		"POLY_API_KEY":    c.apiKey,
		"POLY_PASSPHRASE": c.apiPassphrase,
		"POLY_TIMESTAMP":  timestamp,
		"POLY_SIGNATURE":  signature,
	}, nil
}

func calcOrderAmounts(price, size float64, isBuy bool) (makerAmt, takerAmt int64) {
	if isBuy {
		return int64(math.Round(size * price * 1e6)), int64(math.Round(size * 1e6))
	}
	return int64(math.Round(size * 1e6)), int64(math.Round(size * price * 1e6))
}

func filterByTokenIDs(orders []Order, allowed map[string]bool) []Order {
	out := make([]Order, 0, len(orders))
	for _, o := range orders {
		if allowed[o.TokenID] {
			out = append(out, o)
		}
	}
	return out
}

func parsePrivateKey(key string) (*ecdsa.PrivateKey, error) {
	return crypto.HexToECDSA(trimHexPrefix(key))
}

func trimHexPrefix(s string) string {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return s[2:]
	}
	return s
}
