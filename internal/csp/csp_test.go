package csp

import (
	"math"
	"testing"
	"time"
)

const epsilon = 0.01

func approxEqual(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return math.Abs(a-b) < epsilon
}

// --- Score functions ---

func TestScoreVIX(t *testing.T) {
	tests := []struct {
		name string
		vix  float64
		want float64
	}{
		{"floor", 15, 0},
		{"mid", 20, 50},
		{"three-quarter", 25, 75},
		{"ceiling", 30, 100},
		{"capped above", 35, 100},
		{"below floor", 10, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreVIX(tc.vix)
			if !approxEqual(got, tc.want) {
				t.Errorf("ScoreVIX(%v) = %v, want %v", tc.vix, got, tc.want)
			}
		})
	}
}

func TestScoreIVRank(t *testing.T) {
	tests := []struct {
		name   string
		ivRank float64
		want   float64
	}{
		{"zero", 0, 0},
		{"mid", 50, 50},
		{"three-quarter", 75, 75},
		{"max", 100, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreIVRank(tc.ivRank)
			if !approxEqual(got, tc.want) {
				t.Errorf("ScoreIVRank(%v) = %v, want %v", tc.ivRank, got, tc.want)
			}
		})
	}
}

func TestScoreRSI(t *testing.T) {
	// RSI inverted piecewise: 70→0, 40→50, 20→100
	tests := []struct {
		name string
		rsi  float64
		want float64
	}{
		{"overbought", 70, 0},
		{"mid", 40, 50},
		{"low-mid", 30, 75},
		{"oversold", 20, 100},
		{"above ceiling", 80, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreRSI(tc.rsi)
			if !approxEqual(got, tc.want) {
				t.Errorf("ScoreRSI(%v) = %v, want %v", tc.rsi, got, tc.want)
			}
		})
	}
}

func TestScorePutCallRatio(t *testing.T) {
	tests := []struct {
		name string
		pcr  float64
		want float64
	}{
		{"floor", 0.5, 0},
		{"mid", 1.0, 50},
		{"ceiling", 1.5, 100},
		{"capped", 2.0, 100},
		{"below floor", 0.3, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScorePutCallRatio(tc.pcr)
			if !approxEqual(got, tc.want) {
				t.Errorf("ScorePutCallRatio(%v) = %v, want %v", tc.pcr, got, tc.want)
			}
		})
	}
}

func TestScorePremiumYield(t *testing.T) {
	tests := []struct {
		name  string
		yield float64
		want  float64
	}{
		{"zero", 0, 0},
		{"mid", 15, 50},
		{"ceiling", 30, 100},
		{"capped", 45, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScorePremiumYield(tc.yield)
			if !approxEqual(got, tc.want) {
				t.Errorf("ScorePremiumYield(%v) = %v, want %v", tc.yield, got, tc.want)
			}
		})
	}
}

// --- Composite score ---

func TestCompositeScore(t *testing.T) {
	// VIX=25→75, IVRank=50→50, RSI=40→50, PCR=1.0→50, PremYield=15→50
	// composite = 0.20*75 + 0.25*50 + 0.20*50 + 0.15*50 + 0.20*50
	// = 15 + 12.5 + 10 + 7.5 + 10 = 55
	input := SignalInput{
		VIX:             25,
		CurrentIV:       0.30,
		IVHigh52w:       0.40,
		IVLow52w:        0.20, // IVRank = (0.30-0.20)/(0.40-0.20)*100 = 50
		ClosingPrices:   makeRSIData(40),
		TotalPutVolume:  1000,
		TotalCallVolume: 1000, // PCR = 1.0
		PutPremium:      1.23,
		StrikePrice:     100,
		DTE:             30,
	}
	out := ComputeSignals(input)
	// PremiumYield = (1.23/100)*(365/30)*100 = 14.965 → score ≈ 49.88
	// So composite ≈ 0.20*75 + 0.25*50 + 0.20*50 + 0.15*50 + 0.20*49.88
	// = 15 + 12.5 + 10 + 7.5 + 9.976 = 54.976
	if math.Abs(out.CompositeScore-55) > 2 {
		t.Errorf("CompositeScore = %v, want ~55 (±2)", out.CompositeScore)
	}
}

func TestCompositeScoreAllZero(t *testing.T) {
	input := SignalInput{
		VIX:             15,
		CurrentIV:       0.20,
		IVHigh52w:       0.20,
		IVLow52w:        0.20, // zero range → IVRank NaN
		ClosingPrices:   makeRSIData(70),
		TotalPutVolume:  50,
		TotalCallVolume: 100, // PCR = 0.5 → 0
		PutPremium:      0,
		StrikePrice:     100,
		DTE:             30,
	}
	out := ComputeSignals(input)
	// IVRank NaN → re-weighted from remaining signals all at 0
	if out.CompositeScore > 1 {
		t.Errorf("CompositeScoreAllZero = %v, want ~0", out.CompositeScore)
	}
}

func TestCompositeScorePerfect(t *testing.T) {
	input := SignalInput{
		VIX:             30,
		CurrentIV:       0.40,
		IVHigh52w:       0.40,
		IVLow52w:        0.20, // rank = 100
		ClosingPrices:   makeRSIData(20),
		TotalPutVolume:  150,
		TotalCallVolume: 100, // PCR = 1.5 → 100
		PutPremium:      2.466,
		StrikePrice:     100,
		DTE:             30,
	}
	out := ComputeSignals(input)
	if out.CompositeScore < 95 {
		t.Errorf("CompositeScorePerfect = %v, want >= 95", out.CompositeScore)
	}
	if out.Signal != "STRONG" {
		t.Errorf("Signal = %q, want STRONG", out.Signal)
	}
}

func TestCompositeScorePartialData(t *testing.T) {
	input := SignalInput{
		VIX:             25,
		CurrentIV:       0.30,
		IVHigh52w:       0.30,
		IVLow52w:        0.30, // zero range → IVRank NaN
		ClosingPrices:   makeRSIData(40),
		TotalPutVolume:  1000,
		TotalCallVolume: 1000,
		PutPremium:      1.23,
		StrikePrice:     100,
		DTE:             30,
	}
	out := ComputeSignals(input)
	// VIX=75, RSI≈50, PCR=50, PremYield≈49.88; IVRank=NaN
	// re-weighted total = 0.20+0.20+0.15+0.20 = 0.75
	// score = (0.20*75 + 0.20*50 + 0.15*50 + 0.20*49.88) / 0.75
	// = (15 + 10 + 7.5 + 9.976) / 0.75 = 42.476/0.75 = 56.63
	if out.CompositeScore < 55 || out.CompositeScore > 58 {
		t.Errorf("CompositeScorePartialData = %v, want ~56.6", out.CompositeScore)
	}
}

// --- Calculate functions ---

func TestCalculateRSI(t *testing.T) {
	// Wilder's classic example data
	prices := []float64{
		44, 44.34, 44.09, 43.61, 44.33,
		44.83, 45.10, 45.42, 45.84, 46.08,
		45.89, 46.03, 45.61, 46.28, 46.28,
	}
	rsi := CalculateRSI(prices)
	if math.IsNaN(rsi) {
		t.Fatal("CalculateRSI returned NaN for valid data")
	}
	// Expected ~70 for this data
	if rsi < 60 || rsi > 80 {
		t.Errorf("CalculateRSI = %v, expected in [60,80]", rsi)
	}
}

func TestCalculateRSIInsufficientData(t *testing.T) {
	prices := []float64{1, 2, 3}
	rsi := CalculateRSI(prices)
	if !math.IsNaN(rsi) {
		t.Errorf("CalculateRSI insufficient data = %v, want NaN", rsi)
	}
}

func TestCalculateIVRank(t *testing.T) {
	rank := CalculateIVRank(30, 20, 40)
	if !approxEqual(rank, 50) {
		t.Errorf("CalculateIVRank(30,20,40) = %v, want 50", rank)
	}
}

func TestCalculateIVRankZeroRange(t *testing.T) {
	rank := CalculateIVRank(30, 30, 30)
	if !math.IsNaN(rank) {
		t.Errorf("CalculateIVRank zero range = %v, want NaN", rank)
	}
}

func TestCalculatePremiumYield(t *testing.T) {
	yield := CalculatePremiumYield(1.0, 100, 30)
	expected := (1.0 / 100.0) * (365.0 / 30.0) * 100.0
	if !approxEqual(yield, expected) {
		t.Errorf("CalculatePremiumYield = %v, want %v", yield, expected)
	}
}

func TestCalculateDelta(t *testing.T) {
	delta := CalculateDelta(100, 95, 0.30, 30)
	if delta > -0.10 || delta < -0.50 {
		t.Errorf("CalculateDelta(100,95,0.30,30) = %v, expected in [-0.50,-0.10]", delta)
	}
	if delta > 0 {
		t.Errorf("put delta should be negative, got %v", delta)
	}
}

// --- Filter and Select ---

func TestFilterContracts(t *testing.T) {
	contracts := []OptionContract{
		{Strike: 95, Bid: 1.50, Ask: 1.60, Volume: 100, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: futureExpiry(30)},
		{Strike: 90, Bid: 0.05, Ask: 0.10, Volume: 100, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: futureExpiry(30)},
		{Strike: 85, Bid: 1.00, Ask: 1.10, Volume: 5, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: futureExpiry(30)},
		{Strike: 80, Bid: 1.00, Ask: 1.10, Volume: 100, OpenInterest: 5, ImpliedVolatility: 0.30, Expiration: futureExpiry(30)},
	}
	filtered := FilterContracts(contracts, 100)
	if len(filtered) == 0 {
		t.Fatal("FilterContracts returned 0 contracts, expected at least 1")
	}
	if filtered[0].Strike != 95 {
		t.Errorf("Expected strike 95 to survive, got %v", filtered[0].Strike)
	}
}

func TestFilterContractsAllRejected(t *testing.T) {
	contracts := []OptionContract{
		{Strike: 50, Bid: 0.01, Ask: 0.02, Volume: 1, OpenInterest: 1, ImpliedVolatility: 0.30, Expiration: futureExpiry(30)},
	}
	filtered := FilterContracts(contracts, 100)
	if len(filtered) != 0 {
		t.Errorf("Expected all rejected, got %d", len(filtered))
	}
}

func TestBidAskSpreadFilter(t *testing.T) {
	contracts := []OptionContract{
		{Strike: 95, Bid: 1.00, Ask: 2.00, Volume: 100, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: futureExpiry(30)},
	}
	filtered := FilterContracts(contracts, 100)
	if len(filtered) != 0 {
		t.Errorf("Wide spread contract should be rejected, got %d", len(filtered))
	}
}

func TestSelectTargetContract(t *testing.T) {
	now := time.Now()
	exp30 := now.AddDate(0, 0, 30).Unix()
	exp60 := now.AddDate(0, 0, 60).Unix()

	chain := OptionsData{
		UnderlyingPrice: 100,
		ExpirationDates: []int64{exp30, exp60},
		Puts: []OptionContract{
			{Strike: 95, Bid: 1.50, Ask: 1.60, Volume: 100, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: exp30},
			{Strike: 90, Bid: 2.50, Ask: 2.70, Volume: 100, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: exp30},
			{Strike: 98, Bid: 3.00, Ask: 3.10, Volume: 100, OpenInterest: 200, ImpliedVolatility: 0.30, Expiration: exp60},
		},
	}

	selected := SelectTargetContract(chain)
	if selected == nil {
		t.Fatal("SelectTargetContract returned nil")
	}
	if selected.Strike != 95 {
		t.Errorf("Expected strike 95, got %v", selected.Strike)
	}
}

// --- Helpers ---

func makeRSIData(targetRSI float64) []float64 {
	n := 30
	prices := make([]float64, n)
	prices[0] = 100

	if targetRSI >= 99 {
		for i := 1; i < n; i++ {
			prices[i] = prices[i-1] + 1
		}
		return prices
	}
	if targetRSI <= 1 {
		for i := 1; i < n; i++ {
			prices[i] = prices[i-1] - 0.5
		}
		return prices
	}

	rs := (100.0 / (100.0 - targetRSI)) - 1.0
	gain := rs * 1.0
	loss := 1.0
	for i := 1; i < n; i++ {
		if i%2 == 1 {
			prices[i] = prices[i-1] + gain
		} else {
			prices[i] = prices[i-1] - loss
		}
	}
	return prices
}

func futureExpiry(daysFromNow int) int64 {
	return time.Now().AddDate(0, 0, daysFromNow).Unix()
}
