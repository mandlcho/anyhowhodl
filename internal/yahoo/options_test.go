package yahoo

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseOptionsResponse(t *testing.T) {
	data, err := os.ReadFile("testdata/yahoo-options-response.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	var or optionsResponse
	if err := json.Unmarshal(data, &or); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}

	opts, err := parseOptionsResponse(&or)
	if err != nil {
		t.Fatalf("parseOptionsResponse: %v", err)
	}

	// Verify underlying price
	if opts.UnderlyingPrice != 259.48 {
		t.Errorf("UnderlyingPrice = %v, want 259.48", opts.UnderlyingPrice)
	}

	// Verify expiration dates are populated
	if len(opts.ExpirationDates) == 0 {
		t.Fatal("ExpirationDates is empty")
	}
	if opts.ExpirationDates[0] != 1770336000 {
		t.Errorf("first expiration = %d, want 1770336000", opts.ExpirationDates[0])
	}

	// Verify puts are parsed
	if len(opts.Puts) == 0 {
		t.Fatal("Puts is empty")
	}

	// Check first put contract
	firstPut := opts.Puts[0]
	if firstPut.Strike != 130.0 {
		t.Errorf("first put strike = %v, want 130.0", firstPut.Strike)
	}
	if firstPut.LastPrice != 0.29 {
		t.Errorf("first put lastPrice = %v, want 0.29", firstPut.LastPrice)
	}
	if firstPut.Expiration != 1770336000 {
		t.Errorf("first put expiration = %d, want 1770336000", firstPut.Expiration)
	}

	// Verify calls are parsed
	if len(opts.Calls) == 0 {
		t.Fatal("Calls is empty")
	}

	// Check a call with non-trivial IV
	// AAPL260206C00255000 has IV 0.19947089599609374
	found := false
	for _, c := range opts.Calls {
		if c.Strike == 255.0 && c.ImpliedVolatility > 0.19 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find call at strike 255.0 with IV > 0.19")
	}

	t.Logf("Parsed %d puts, %d calls, %d expiration dates",
		len(opts.Puts), len(opts.Calls), len(opts.ExpirationDates))
}

func TestParseChartResponse(t *testing.T) {
	data, err := os.ReadFile("testdata/yahoo-chart-1y-response.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	var cr chartHistoryResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}

	closes, err := parseChartHistoryResponse(&cr)
	if err != nil {
		t.Fatalf("parseChartHistoryResponse: %v", err)
	}

	// The fixture has 20 values with 1 null, so 19 closes
	if len(closes) != 19 {
		t.Errorf("got %d closes, want 19", len(closes))
	}

	// First close should be 170.12
	if len(closes) > 0 && closes[0] != 170.12 {
		t.Errorf("first close = %v, want 170.12", closes[0])
	}

	// Last close should be 188.50
	if len(closes) > 0 && closes[len(closes)-1] != 188.50 {
		t.Errorf("last close = %v, want 188.50", closes[len(closes)-1])
	}

	t.Logf("Parsed %d closing prices (nils filtered)", len(closes))
}
