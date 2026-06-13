package trade

import (
	"math"
	"strings"
	"testing"
)

func TestParsePrivateKey_Valid(t *testing.T) {
	key := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	pk, err := parsePrivateKey(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pk == nil {
		t.Fatal("expected non-nil private key")
	}
}

func TestParsePrivateKey_WithPrefix(t *testing.T) {
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	pk, err := parsePrivateKey(key)
	if err != nil {
		t.Fatalf("unexpected error with 0x prefix: %v", err)
	}
	if pk == nil {
		t.Fatal("expected non-nil private key")
	}
}

func TestParsePrivateKey_Invalid(t *testing.T) {
	_, err := parsePrivateKey("notahexkey")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestCalcOrderAmounts_Buy(t *testing.T) {
	price := 0.47
	size := 100.0
	makerAmt, takerAmt := calcOrderAmounts(price, size, true)
	if makerAmt != 47_000_000 {
		t.Errorf("BUY makerAmt: want 47000000 got %d", makerAmt)
	}
	if takerAmt != 100_000_000 {
		t.Errorf("BUY takerAmt: want 100000000 got %d", takerAmt)
	}
}

func TestCalcOrderAmounts_Sell(t *testing.T) {
	price := 0.46
	size := 100.0
	makerAmt, takerAmt := calcOrderAmounts(price, size, false)
	if makerAmt != 100_000_000 {
		t.Errorf("SELL makerAmt: want 100000000 got %d", makerAmt)
	}
	if takerAmt != 46_000_000 {
		t.Errorf("SELL takerAmt: want 46000000 got %d", takerAmt)
	}
}

func TestTrimHexPrefix(t *testing.T) {
	if trimHexPrefix("0xabcd") != "abcd" {
		t.Error("should strip 0x prefix")
	}
	if trimHexPrefix("abcd") != "abcd" {
		t.Error("should leave non-prefixed string unchanged")
	}
}

func TestFilterByTokenIDs(t *testing.T) {
	orders := []Order{
		{TokenID: "tokenA", Status: "OPEN"},
		{TokenID: "tokenB", Status: "OPEN"},
		{TokenID: "tokenC", Status: "OPEN"},
	}
	allowed := map[string]bool{"tokenA": true, "tokenC": true}
	result := filterByTokenIDs(orders, allowed)
	if len(result) != 2 {
		t.Fatalf("want 2 orders, got %d", len(result))
	}
	if result[0].TokenID != "tokenA" || result[1].TokenID != "tokenC" {
		t.Error("wrong orders returned")
	}
}

func TestCalcOrderAmounts_Rounding(t *testing.T) {
	price := 0.4755
	size := 99.9
	roundedPrice := math.Round(price*100) / 100
	roundedSize := math.Round(size)
	makerAmt, _ := calcOrderAmounts(roundedPrice, roundedSize, true)
	if makerAmt != 48_000_000 {
		t.Errorf("want 48000000 got %d", makerAmt)
	}
	_ = strings.TrimSpace("ok")
}
