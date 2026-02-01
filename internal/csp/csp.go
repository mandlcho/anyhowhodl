package csp

import (
	"math"
	"time"
)

// Signal weights for composite CSP score.
const (
	WeightVIX          = 0.20
	WeightIVRank       = 0.25
	WeightRSI          = 0.20
	WeightPutCallRatio = 0.15
	WeightPremiumYield = 0.20
)

// Contract quality filter constants (from repo analysis).
const (
	MinVolume       = 10
	MinOpenInterest = 10
	MaxBidAskSpread = 0.15
	MinBidPrice     = 0.10
	MaxDelta        = -0.20
	MinDelta        = -0.50
	RiskFreeRate    = 0.05
)

// SignalInput holds raw data for CSP score computation.
type SignalInput struct {
	VIX             float64
	CurrentIV       float64
	IVHigh52w       float64
	IVLow52w        float64
	ClosingPrices   []float64
	TotalPutVolume  float64
	TotalCallVolume float64
	PutPremium      float64
	StrikePrice     float64
	DTE             int
}

// SignalOutput holds computed signals and composite score.
type SignalOutput struct {
	VIXScore          float64
	IVRankScore       float64
	RSIScore          float64
	PutCallRatioScore float64
	PremiumYieldScore float64
	CompositeScore    float64
	RawVIX            float64
	RawIVRank         float64
	RawRSI            float64
	RawPutCallRatio   float64
	RawPremiumYield   float64
	Signal            string
}

// OptionContract represents a single option from the chain.
type OptionContract struct {
	Strike            float64
	LastPrice         float64
	Bid               float64
	Ask               float64
	Volume            int
	OpenInterest      int
	ImpliedVolatility float64
	Expiration        int64
	Delta             float64
}

// OptionsData holds the parsed options chain for a ticker.
type OptionsData struct {
	UnderlyingPrice float64
	Puts            []OptionContract
	Calls           []OptionContract
	ExpirationDates []int64
}

// linearInterp does piecewise linear interpolation through three points,
// clamped to [0, 100].
func linearInterp(x, x0, x1, x2, y0, y1, y2 float64) float64 {
	var score float64
	if x <= x0 {
		score = y0
	} else if x <= x1 {
		score = y0 + (x-x0)/(x1-x0)*(y1-y0)
	} else if x <= x2 {
		score = y1 + (x-x1)/(x2-x1)*(y2-y1)
	} else {
		score = y2
	}
	return math.Max(0, math.Min(100, score))
}

// ScoreVIX scores VIX level: 15→0, 20→50, 30→100 (capped).
func ScoreVIX(vix float64) float64 {
	return linearInterp(vix, 15, 20, 30, 0, 50, 100)
}

// ScoreIVRank scores IV Rank (0-100): 0→0, 50→50, 100→100 (linear).
func ScoreIVRank(ivRank float64) float64 {
	return linearInterp(ivRank, 0, 50, 100, 0, 50, 100)
}

// ScoreRSI scores RSI inverted: 70→0, 40→50, 20→100.
func ScoreRSI(rsi float64) float64 {
	// Inverted: higher RSI = lower score
	// Map: rsi=20→100, rsi=40→50, rsi=70→0
	if rsi >= 70 {
		return 0
	}
	if rsi <= 20 {
		return 100
	}
	if rsi >= 40 {
		// [40,70] → [50,0]
		return 50 - (rsi-40)/(70-40)*50
	}
	// [20,40] → [100,50]
	return 100 - (rsi-20)/(40-20)*50
}

// ScorePutCallRatio scores P/C ratio: 0.5→0, 1.0→50, 1.5→100 (capped).
func ScorePutCallRatio(pcr float64) float64 {
	return linearInterp(pcr, 0.5, 1.0, 1.5, 0, 50, 100)
}

// ScorePremiumYield scores annualized yield: 0→0, 15→50, 30→100 (capped).
func ScorePremiumYield(annualizedYield float64) float64 {
	return linearInterp(annualizedYield, 0, 15, 30, 0, 50, 100)
}

// CalculateRSI computes 14-period RSI from closing prices (newest last).
// Returns NaN if fewer than 15 data points.
func CalculateRSI(closes []float64) float64 {
	period := 14
	if len(closes) < period+1 {
		return math.NaN()
	}

	var gainSum, lossSum float64
	for i := 1; i <= period; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			gainSum += change
		} else {
			lossSum -= change
		}
	}

	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)

	// Smooth with remaining data using Wilder's method
	for i := period + 1; i < len(closes); i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - change) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// CalculateIVRank computes IV Rank as percentage.
// Returns NaN if high == low.
func CalculateIVRank(current, low52w, high52w float64) float64 {
	r := high52w - low52w
	if r == 0 {
		return math.NaN()
	}
	return (current - low52w) / r * 100
}

// CalculatePremiumYield computes annualized premium yield percentage.
func CalculatePremiumYield(premium, strike float64, dte int) float64 {
	if strike == 0 || dte == 0 {
		return 0
	}
	return (premium / strike) * (365.0 / float64(dte)) * 100
}

// CalculateDelta computes Black-Scholes put delta: -N(-d1).
func CalculateDelta(S, K, iv float64, dte int) float64 {
	if iv <= 0 || dte <= 0 || S <= 0 || K <= 0 {
		return 0
	}
	t := float64(dte) / 365.0
	sqrtT := math.Sqrt(t)
	d1 := (math.Log(S/K) + (RiskFreeRate+iv*iv/2)*t) / (iv * sqrtT)
	// Put delta = -N(-d1) = -(1 - N(d1)) = N(d1) - 1
	return normCDF(d1) - 1
}

// normCDF computes the standard normal cumulative distribution function.
func normCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// FilterContracts applies quality filters and returns surviving contracts.
// Delta is computed for each contract using the underlying price.
func FilterContracts(contracts []OptionContract, underlyingPrice float64) []OptionContract {
	var result []OptionContract
	for _, c := range contracts {
		if c.Volume < MinVolume {
			continue
		}
		if c.OpenInterest < MinOpenInterest {
			continue
		}
		if c.Bid < MinBidPrice {
			continue
		}
		mid := (c.Bid + c.Ask) / 2
		if mid <= 0 {
			continue
		}
		spread := (c.Ask - c.Bid) / mid
		if spread > MaxBidAskSpread {
			continue
		}
		dte := daysUntil(c.Expiration)
		delta := CalculateDelta(underlyingPrice, c.Strike, c.ImpliedVolatility, dte)
		if delta < MinDelta || delta > MaxDelta {
			continue
		}
		c.Delta = delta
		result = append(result, c)
	}
	return result
}

// SelectTargetContract picks the best contract from the chain:
// 1. Find expiry closest to 30 DTE within 21-45 window
// 2. Filter contracts for that expiry
// 3. Pick the one closest to ATM (nearest strike to underlying)
func SelectTargetContract(chain OptionsData) *OptionContract {
	now := time.Now()

	// Find best expiry: closest to 30 DTE within [21, 45]
	bestExpiry := int64(0)
	bestDist := math.MaxFloat64
	for _, exp := range chain.ExpirationDates {
		expTime := time.Unix(exp, 0)
		dte := expTime.Sub(now).Hours() / 24
		if dte < 21 || dte > 45 {
			continue
		}
		dist := math.Abs(dte - 30)
		if dist < bestDist {
			bestDist = dist
			bestExpiry = exp
		}
	}
	if bestExpiry == 0 {
		return nil
	}

	// Get puts for this expiry
	var expiryPuts []OptionContract
	for _, p := range chain.Puts {
		if p.Expiration == bestExpiry {
			expiryPuts = append(expiryPuts, p)
		}
	}

	// Apply quality filters
	filtered := FilterContracts(expiryPuts, chain.UnderlyingPrice)
	if len(filtered) == 0 {
		return nil
	}

	// Pick nearest ATM
	var best *OptionContract
	bestStrikeDist := math.MaxFloat64
	for i := range filtered {
		dist := math.Abs(filtered[i].Strike - chain.UnderlyingPrice)
		if dist < bestStrikeDist {
			bestStrikeDist = dist
			best = &filtered[i]
		}
	}
	return best
}

// ComputeSignals calculates all signal scores and the composite score.
// If a signal is NaN, remaining signals are re-weighted proportionally.
func ComputeSignals(input SignalInput) SignalOutput {
	out := SignalOutput{
		RawVIX: input.VIX,
	}

	// Raw calculations
	ivRank := CalculateIVRank(input.CurrentIV, input.IVLow52w, input.IVHigh52w)
	rsi := CalculateRSI(input.ClosingPrices)
	pcr := 0.0
	if input.TotalCallVolume > 0 {
		pcr = input.TotalPutVolume / input.TotalCallVolume
	}
	premYield := CalculatePremiumYield(input.PutPremium, input.StrikePrice, input.DTE)

	out.RawIVRank = ivRank
	out.RawRSI = rsi
	out.RawPutCallRatio = pcr
	out.RawPremiumYield = premYield

	// Score each signal
	vixScore := ScoreVIX(input.VIX)
	out.VIXScore = vixScore

	var ivRankScore float64
	if math.IsNaN(ivRank) {
		ivRankScore = math.NaN()
	} else {
		ivRankScore = ScoreIVRank(ivRank)
	}
	out.IVRankScore = ivRankScore

	var rsiScore float64
	if math.IsNaN(rsi) {
		rsiScore = math.NaN()
	} else {
		rsiScore = ScoreRSI(rsi)
	}
	out.RSIScore = rsiScore

	pcrScore := ScorePutCallRatio(pcr)
	out.PutCallRatioScore = pcrScore

	premScore := ScorePremiumYield(premYield)
	out.PremiumYieldScore = premScore

	// Composite with NaN re-weighting
	type weightedSignal struct {
		weight float64
		score  float64
	}
	signals := []weightedSignal{
		{WeightVIX, vixScore},
		{WeightIVRank, ivRankScore},
		{WeightRSI, rsiScore},
		{WeightPutCallRatio, pcrScore},
		{WeightPremiumYield, premScore},
	}

	totalWeight := 0.0
	weightedSum := 0.0
	for _, s := range signals {
		if math.IsNaN(s.score) {
			continue
		}
		totalWeight += s.weight
		weightedSum += s.weight * s.score
	}

	if totalWeight > 0 {
		out.CompositeScore = weightedSum / totalWeight
	}

	// Signal string
	switch {
	case out.CompositeScore > 70:
		out.Signal = "STRONG"
	case out.CompositeScore >= 50:
		out.Signal = "MODERATE"
	default:
		out.Signal = "WEAK"
	}

	return out
}

func daysUntil(unixTimestamp int64) int {
	expTime := time.Unix(unixTimestamp, 0)
	d := time.Until(expTime).Hours() / 24
	if d < 0 {
		return 0
	}
	return int(d)
}
