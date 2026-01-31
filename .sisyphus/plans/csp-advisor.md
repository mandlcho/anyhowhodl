# Cash-Secured Put Advisor

## TL;DR

> **Quick Summary**: Add a CSP Advisor tab to the terminal UI that computes a composite "CSP Score" (0-100) for watchlisted tickers, combining VIX, IV Rank, RSI, Put/Call Ratio, and Premium Yield signals from Yahoo Finance data.
> 
> **Deliverables**:
> - `internal/csp/` package — pure signal calculation engine with full test coverage
> - Extended `internal/yahoo/` — options chain + price history fetching
> - Extended `internal/db/` — CSP watchlist table + CRUD
> - `csp_view.go` — new TUI tab wired into main app via `p` keybinding
> - `schema_csp.sql` — database migration for new tables
> 
> **Estimated Effort**: Large
> **Parallel Execution**: YES - 2 waves
> **Critical Path**: Task 1 (API probe) → Task 3 (yahoo ext) → Task 5 (TUI)

---

## Context

### Original Request
Build a "Cash-Secured Put Advisor" — a separate TUI tab that analyzes whether it's a good time to sell cash-secured puts on stocks the user is watching. Uses Yahoo Finance data to compute VIX, IV Rank, RSI, Put/Call Ratio, and Premium Yield into a composite 0-100 score.

### Interview Summary
**Key Discussions**:
- Composite CSP Score from exactly 5 weighted signals
- All data from Yahoo Finance REST API (no API keys needed)
- TDD approach with `go test`
- Separate watchlist from existing holdings

**Research Findings**:
- Yahoo `/v7/finance/options/{ticker}` provides options chains with `impliedVolatility` per contract
- Yahoo `/v8/finance/chart/{ticker}?range=1y&interval=1d` provides price history for RSI
- VIX available via `^VIX` ticker through existing fetchQuote
- Existing yahoo package uses `float64` (not decimal) — follow this pattern for calculations

### Metis Review
**Identified Gaps** (addressed):
- **Strike/expiry selection**: Fixed to nearest ATM put, 30-45 DTE. No user selection V1.
- **CSP watchlist vs holdings**: Watchlist is independent — user can watch tickers they don't own.
- **IV computation**: Use Yahoo's `impliedVolatility` field from options chain (no Black-Scholes needed). Task 1 verifies this field exists.
- **Data staleness**: Manual refresh only (triggered with existing `r` key when on CSP tab). No separate auto-refresh V1.
- **float64 vs decimal**: Signal calculations use `float64` (matching yahoo package). Only DB storage uses decimal.
- **main.go bloat**: All CSP TUI code goes in separate `csp_view.go` file.

---

## Work Objectives

### Core Objective
Add a CSP Advisor tab that shows a scored recommendation (0-100) for selling cash-secured puts on watchlisted tickers.

### Concrete Deliverables
- `internal/csp/csp.go` + `internal/csp/csp_test.go` — signal engine
- `internal/yahoo/options.go` + `internal/yahoo/options_test.go` — options chain + history fetch
- `internal/db/csp.go` + `internal/db/csp_test.go` — watchlist CRUD
- `csp_view.go` — TUI tab (CSP table + score display)
- `schema_csp.sql` — migration SQL
- Wiring in `main.go` — `p` keybinding, App struct fields

### Definition of Done
- [ ] `go build .` succeeds
- [ ] `go test ./...` passes with all new tests
- [ ] `go vet ./...` reports zero warnings
- [ ] Pressing `p` in the app toggles the CSP advisor view
- [ ] Adding a ticker to watchlist persists it in DB
- [ ] CSP score computes for tickers with available options data
- [ ] Tickers without options data show "N/A" gracefully

### Must Have
- 5-signal composite score: VIX (20%), IV Rank (25%), RSI (20%), P/C Ratio (15%), Premium Yield (20%)
- CSP watchlist stored in PostgreSQL (independent of holdings)
- Add/remove tickers from watchlist via TUI form
- Score color coding: >70 green (Strong), 50-70 yellow (Moderate), <50 red (Weak)
- Graceful degradation when Yahoo data unavailable

### Must NOT Have (Guardrails)
- NO more than 5 signals in the composite score
- NO Black-Scholes implementation (use Yahoo's IV field)
- NO historical signal storage or backtesting
- NO full options chain display (only computed metrics)
- NO strike/expiry selection UI (fixed logic: nearest ATM, 30-45 DTE)
- NO alerts or notifications
- NO refactoring of existing main.go structure, App struct, or keybindings
- NO new Go dependencies beyond what's already in go.mod
- NO auto-refresh for CSP data (manual `r` key only)

---

## Verification Strategy (MANDATORY)

### Test Decision
- **Infrastructure exists**: NO (zero test files currently)
- **User wants tests**: TDD
- **Framework**: `go test` (stdlib)

### TDD Flow Per Task
1. **RED**: Write failing test
2. **GREEN**: Implement minimum code to pass
3. **REFACTOR**: Clean up while tests stay green

### Signal Calculation Constants (locked at plan time)

```go
// Weights for composite CSP score
const (
    WeightVIX          = 0.20
    WeightIVRank       = 0.25
    WeightRSI          = 0.20
    WeightPutCallRatio = 0.15
    WeightPremiumYield = 0.20
)

// Signal scoring thresholds
// VIX: 15→0pts, 20→50pts, 30→100pts (linear interpolation)
// IV Rank: 0%→0pts, 50%→50pts, 100%→100pts (linear)
// RSI: 70→0pts, 40→50pts, 20→100pts (inverted linear)
// P/C Ratio: 0.5→0pts, 1.0→50pts, 1.5→100pts (linear, capped)
// Premium Yield: 0%→0pts, 15%→50pts, 30%→100pts (annualized, linear)
```

### Strike/Expiry Selection Logic (locked)
1. Get options chain for ticker
2. Filter expiry dates to 30-45 DTE window (pick closest to 35 DTE)
3. Find put strike closest to current price (nearest ATM)
4. Use that contract's premium and IV for calculations

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately):
├── Task 1: Yahoo API endpoint probe (curl verification)
├── Task 2: CSP signal engine (internal/csp) — pure math, TDD
└── Task 4: DB schema + watchlist CRUD (internal/db)

Wave 2 (After Wave 1):
├── Task 3: Yahoo options/history fetchers (depends: Task 1 findings)
└── Task 5: TUI integration (depends: Tasks 2, 3, 4)
```

### Dependency Matrix

| Task | Depends On | Blocks | Can Parallelize With |
|------|------------|--------|---------------------|
| 1 | None | 3 | 2, 4 |
| 2 | None | 5 | 1, 4 |
| 3 | 1 | 5 | 4 |
| 4 | None | 5 | 1, 2 |
| 5 | 2, 3, 4 | None | None (final) |

### Agent Dispatch Summary

| Wave | Tasks | Recommended Agents |
|------|-------|-------------------|
| 1 | 1, 2, 4 | 3 parallel background tasks |
| 2 | 3, 5 | Sequential after Wave 1 |

---

## TODOs

- [ ] 1. Verify Yahoo Options API Endpoint

  **What to do**:
  - Probe `https://query1.finance.yahoo.com/v7/finance/options/AAPL` with curl
  - Document the response JSON structure
  - Confirm these fields exist in the response:
    - `optionChain.result[0].options[0].puts[]` — array of put contracts
    - Each put contract has: `strike`, `lastPrice`, `bid`, `ask`, `volume`, `openInterest`, `impliedVolatility`
    - `optionChain.result[0].expirationDates[]` — available expiry timestamps
    - `optionChain.result[0].quote.regularMarketPrice` — current underlying price
  - Probe `https://query2.finance.yahoo.com/v8/finance/chart/AAPL?range=1y&interval=1d`
    - Confirm `chart.result[0].indicators.quote[0].close[]` returns ~252 daily closing prices
  - Probe `https://query1.finance.yahoo.com/v8/finance/chart/%5EVIX?range=1d&interval=1d`
    - Confirm VIX quote works through existing chart endpoint
  - If `/v7/finance/options/` is broken or returns 403, document the failure and flag it — the plan needs adjustment
  - Save response samples to `.sisyphus/evidence/yahoo-options-response.json` and `.sisyphus/evidence/yahoo-chart-1y-response.json`

  **Must NOT do**:
  - Do not write any Go code in this task
  - Do not modify any files except evidence output

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: [`playwright`]
    - `playwright`: Can use browser-based fetch if curl gets blocked by Yahoo (cookie/redirect issues)

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 4)
  - **Blocks**: Task 3
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/yahoo/yahoo.go:76-84` — existing User-Agent header pattern for Yahoo requests

  **External References**:
  - Yahoo Finance options endpoint: `https://query1.finance.yahoo.com/v7/finance/options/{ticker}`
  - Yahoo Finance chart endpoint: `https://query2.finance.yahoo.com/v8/finance/chart/{ticker}?range=1y&interval=1d`

  **Acceptance Criteria**:

  ```bash
  # Verify options endpoint returns valid JSON with puts data
  curl -s -H "User-Agent: Mozilla/5.0" "https://query1.finance.yahoo.com/v7/finance/options/AAPL" | jq '.optionChain.result[0].options[0].puts[0] | keys'
  # Assert: output includes "strike", "impliedVolatility", "lastPrice", "volume", "openInterest"

  # Verify chart endpoint returns 1 year of daily closes
  curl -s -H "User-Agent: Mozilla/5.0" "https://query2.finance.yahoo.com/v8/finance/chart/AAPL?range=1y&interval=1d" | jq '.chart.result[0].indicators.quote[0].close | length'
  # Assert: output is >= 200 (roughly 252 trading days)

  # Verify VIX works
  curl -s -H "User-Agent: Mozilla/5.0" "https://query1.finance.yahoo.com/v8/finance/chart/%5EVIX?range=1d&interval=1d" | jq '.chart.result[0].meta.regularMarketPrice'
  # Assert: returns a number (VIX level)
  ```

  **Evidence to Capture:**
  - [ ] Full JSON response samples saved to `.sisyphus/evidence/`
  - [ ] Confirmation that `impliedVolatility` field exists per contract (determines if B-S is needed)

  **Commit**: NO (no code changes)

---

- [ ] 2. CSP Signal Calculation Engine (`internal/csp`)

  **What to do**:

  **RED phase** — write tests first in `internal/csp/csp_test.go`:
  - `TestScoreVIX`: VIX 15→0, 20→50, 25→75, 30→100, 35→100 (capped)
  - `TestScoreIVRank`: 0→0, 50→50, 75→75, 100→100
  - `TestScoreRSI`: 70→0, 40→50, 30→75, 20→100 (inverted)
  - `TestScorePutCallRatio`: 0.5→0, 1.0→50, 1.5→100, 2.0→100 (capped)
  - `TestScorePremiumYield`: 0→0, 15→50, 30→100, 45→100 (capped)
  - `TestCompositeScore`: known inputs → expected weighted result
  - `TestCompositeScoreAllZero`: all zero inputs → score 0
  - `TestCompositeScorePerfect`: all max inputs → score 100
  - `TestCompositeScorePartialData`: some signals NaN/missing → score computed from available signals only (re-weight)
  - `TestCalculateRSI`: known 14-period close prices → expected RSI value
  - `TestCalculateRSIInsufficientData`: < 15 data points → returns NaN
  - `TestCalculateIVRank`: currentIV=30, lowIV=20, highIV=40 → rank 50
  - `TestCalculateIVRankZeroRange`: highIV == lowIV → returns NaN
  - `TestSelectTargetContract`: given options chain data, selects nearest ATM put in 30-45 DTE window

  **GREEN phase** — implement in `internal/csp/csp.go`:
  ```go
  package csp

  import "math"

  // Signal weights
  const (
      WeightVIX          = 0.20
      WeightIVRank       = 0.25
      WeightRSI          = 0.20
      WeightPutCallRatio = 0.15
      WeightPremiumYield = 0.20
  )

  // SignalInput holds raw data for CSP score computation
  type SignalInput struct {
      VIX            float64   // Current VIX level
      CurrentIV      float64   // Current implied volatility of target contract
      IVHigh52w      float64   // 52-week high IV
      IVLow52w       float64   // 52-week low IV
      ClosingPrices  []float64 // Recent daily closes (newest last), need 15+ for RSI
      TotalPutVolume float64   // Total put volume from options chain
      TotalCallVolume float64  // Total call volume from options chain
      PutPremium     float64   // Selected put contract premium (mid price)
      StrikePrice    float64   // Selected put strike
      DTE            int       // Days to expiration
  }

  // SignalOutput holds computed signals and composite score
  type SignalOutput struct {
      VIXScore          float64 // 0-100
      IVRankScore       float64 // 0-100
      RSIScore          float64 // 0-100
      PutCallRatioScore float64 // 0-100
      PremiumYieldScore float64 // 0-100
      CompositeScore    float64 // 0-100 weighted
      RawVIX            float64
      RawIVRank         float64 // 0-100%
      RawRSI            float64
      RawPutCallRatio   float64
      RawPremiumYield   float64 // annualized %
      Signal            string  // "STRONG", "MODERATE", "WEAK"
  }

  // OptionContract represents a single option from the chain
  type OptionContract struct {
      Strike           float64
      LastPrice        float64
      Bid              float64
      Ask              float64
      Volume           int
      OpenInterest     int
      ImpliedVolatility float64
      Expiration       int64 // Unix timestamp
  }

  // OptionsData holds the parsed options chain for a ticker
  type OptionsData struct {
      UnderlyingPrice float64
      Puts            []OptionContract
      Calls           []OptionContract
      ExpirationDates []int64
  }

  func ComputeSignals(input SignalInput) SignalOutput { ... }
  func ScoreVIX(vix float64) float64 { ... }
  func ScoreIVRank(ivRank float64) float64 { ... }
  func ScoreRSI(rsi float64) float64 { ... }
  func ScorePutCallRatio(pcr float64) float64 { ... }
  func ScorePremiumYield(annualizedYield float64) float64 { ... }
  func CalculateRSI(closes []float64) float64 { ... }
  func CalculateIVRank(current, low52w, high52w float64) float64 { ... }
  func CalculatePremiumYield(premium, strike float64, dte int) float64 { ... }
  func SelectTargetContract(chain OptionsData) *OptionContract { ... }
  ```

  **REFACTOR phase**: Ensure all scoring functions use consistent linear interpolation helper.

  **Must NOT do**:
  - No database imports
  - No HTTP/network calls
  - No tview/UI imports
  - No more than 5 signals

  **Recommended Agent Profile**:
  - **Category**: `ultrabrain`
  - **Skills**: []
    - Pure Go math + TDD — no specialized skills needed

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 4)
  - **Blocks**: Task 5
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/yahoo/yahoo.go:11-19` — struct definition pattern (Quote struct)
  - `internal/db/db.go:12-22` — struct field naming conventions

  **Documentation References**:
  - RSI formula: RSI = 100 - (100 / (1 + RS)), where RS = avg gain / avg loss over 14 periods
  - IV Rank: (Current IV - 52w Low) / (52w High - 52w Low) × 100
  - Premium Yield: (premium / strike) × (365 / DTE) × 100

  **Acceptance Criteria**:

  ```bash
  # RED: Tests exist and fail (no implementation yet)
  go test -v ./internal/csp/ 2>&1 | grep -c "FAIL"
  # Assert: > 0 (tests fail because functions not implemented)

  # GREEN: All tests pass after implementation
  go test -v ./internal/csp/
  # Assert: all PASS, 0 failures

  # Verify score boundaries
  go test -v -run TestCompositeScoreAllZero ./internal/csp/
  # Assert: PASS (returns 0)

  go test -v -run TestCompositeScorePerfect ./internal/csp/
  # Assert: PASS (returns 100)

  # Build still works
  go build .
  # Assert: exit code 0

  go vet ./internal/csp/
  # Assert: no warnings
  ```

  **Commit**: YES
  - Message: `feat(csp): add signal calculation engine with TDD tests`
  - Files: `internal/csp/csp.go`, `internal/csp/csp_test.go`
  - Pre-commit: `go test ./internal/csp/ && go vet ./internal/csp/`

---

- [ ] 3. Extend Yahoo Client — Options Chain + Price History

  **What to do**:

  **RED phase** — write tests in `internal/yahoo/options_test.go`:
  - `TestParseOptionsResponse`: parse a saved JSON fixture → correct OptionsData struct
  - `TestParseChartResponse`: parse saved chart JSON → correct []float64 closes
  - `TestFetchOptionsChain`: integration test (can be skipped in CI with build tag)
  - `TestFetchPriceHistory`: integration test

  **GREEN phase** — implement in `internal/yahoo/options.go`:
  ```go
  package yahoo

  import "anyhowhodl/internal/csp"

  // Response structs for Yahoo options endpoint
  type optionsResponse struct { ... } // maps /v7/finance/options/ JSON

  // Response structs for chart endpoint (extended range)
  type chartHistoryResponse struct { ... } // maps /v8/finance/chart/ with range=1y

  // FetchOptionsChain fetches the full options chain for a ticker
  func (c *Client) FetchOptionsChain(ticker string) (*csp.OptionsData, error) { ... }

  // FetchOptionsChainForExpiry fetches chain for a specific expiry timestamp
  func (c *Client) FetchOptionsChainForExpiry(ticker string, expiry int64) (*csp.OptionsData, error) { ... }

  // FetchPriceHistory fetches 1 year of daily closing prices
  func (c *Client) FetchPriceHistory(ticker string) ([]float64, error) { ... }
  ```

  Key implementation notes:
  - URL: `https://query1.finance.yahoo.com/v7/finance/options/{ticker}` (default expiry)
  - URL: `https://query1.finance.yahoo.com/v7/finance/options/{ticker}?date={unixTimestamp}` (specific expiry)
  - URL: `https://query2.finance.yahoo.com/v8/finance/chart/{ticker}?range=1y&interval=1d`
  - Reuse existing `c.httpClient` and User-Agent header pattern from `fetchQuote`
  - Add 200ms delay between requests to avoid rate limiting (use a simple `time.Sleep`)
  - Return `csp.OptionsData` type directly (defined in Task 2)
  - For price history, extract `chart.result[0].indicators.quote[0].close` array, filter out nil values

  **Must NOT do**:
  - Do not modify existing `fetchQuote` or `GetQuotes` functions
  - Do not add caching
  - Do not add new dependencies

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []
    - Standard Go HTTP + JSON work

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 2 (sequential, needs Task 1 findings)
  - **Blocks**: Task 5
  - **Blocked By**: Task 1 (need confirmed API response structure)

  **References**:

  **Pattern References**:
  - `internal/yahoo/yahoo.go:21-36` — `chartResponse` struct pattern for JSON unmarshaling
  - `internal/yahoo/yahoo.go:76-84` — HTTP request pattern (User-Agent, error handling)
  - `internal/yahoo/yahoo.go:50-73` — concurrent fetching pattern with sync.Mutex

  **API/Type References**:
  - `internal/csp/csp.go:OptionsData` — return type for FetchOptionsChain (from Task 2)
  - `internal/csp/csp.go:OptionContract` — struct for individual contracts (from Task 2)

  **External References**:
  - `.sisyphus/evidence/yahoo-options-response.json` — actual API response structure (from Task 1)
  - `.sisyphus/evidence/yahoo-chart-1y-response.json` — chart response structure (from Task 1)

  **Acceptance Criteria**:

  ```bash
  # Tests pass (including fixture-based parsing tests)
  go test -v ./internal/yahoo/ -run "TestParse"
  # Assert: all PASS

  # Build succeeds
  go build .
  # Assert: exit code 0

  go vet ./internal/yahoo/
  # Assert: no warnings

  # Integration smoke test (if network available)
  go test -v ./internal/yahoo/ -run "TestFetchOptionsChain" -count=1 2>&1 | head -5
  # Assert: PASS or SKIP (not FAIL)
  ```

  **Commit**: YES
  - Message: `feat(yahoo): add options chain and price history fetchers`
  - Files: `internal/yahoo/options.go`, `internal/yahoo/options_test.go`, `internal/yahoo/testdata/*.json`
  - Pre-commit: `go test ./internal/yahoo/ -run "TestParse" && go vet ./internal/yahoo/`

---

- [ ] 4. Database Schema + Watchlist CRUD

  **What to do**:

  **Schema** — create `schema_csp.sql`:
  ```sql
  -- CSP Advisor watchlist
  CREATE TABLE IF NOT EXISTS csp_watchlist (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      ticker VARCHAR(10) NOT NULL UNIQUE,
      notes TEXT,
      created_at TIMESTAMPTZ DEFAULT NOW(),
      updated_at TIMESTAMPTZ DEFAULT NOW()
  );

  CREATE INDEX IF NOT EXISTS idx_csp_watchlist_ticker ON csp_watchlist(ticker);

  DROP TRIGGER IF EXISTS update_csp_watchlist_updated_at ON csp_watchlist;
  CREATE TRIGGER update_csp_watchlist_updated_at
      BEFORE UPDATE ON csp_watchlist
      FOR EACH ROW
      EXECUTE FUNCTION update_updated_at_column();
  ```

  **RED phase** — write tests in `internal/db/csp_test.go`:
  - `TestAddCSPWatchlistTicker`: add ticker, verify it's returned by GetCSPWatchlist
  - `TestAddCSPWatchlistDuplicate`: adding same ticker twice returns error (UNIQUE constraint)
  - `TestRemoveCSPWatchlistTicker`: add then remove, verify empty list
  - `TestGetCSPWatchlist`: add multiple tickers, verify all returned in order

  Note: Tests need a real PostgreSQL connection. Use `DATABASE_URL` env var. Skip if not available.

  **GREEN phase** — implement in `internal/db/csp.go`:
  ```go
  package db

  type CSPWatchItem struct {
      ID        string
      Ticker    string
      Notes     string
      CreatedAt time.Time
      UpdatedAt time.Time
  }

  func (d *DB) AddCSPWatchTicker(ctx context.Context, ticker, notes string) error { ... }
  func (d *DB) RemoveCSPWatchTicker(ctx context.Context, ticker string) error { ... }
  func (d *DB) GetCSPWatchlist(ctx context.Context) ([]CSPWatchItem, error) { ... }
  ```

  **Must NOT do**:
  - No modifications to existing db.go functions
  - No signal/score storage tables (V1 is compute-on-demand)
  - No migration tooling (raw SQL file, same as existing schema.sql)

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []
    - Simple CRUD, follows existing pattern exactly

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Task 5
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/db/db.go:110-136` — `GetHoldings` pattern (Query, rows.Next, Scan, defer rows.Close)
  - `internal/db/db.go:65-108` — `AddHolding` pattern (INSERT with Exec)
  - `internal/db/db.go:145-148` — `DeleteHolding` pattern (DELETE by ID)
  - `internal/db/db.go:42-59` — `DB` struct and `New()` constructor pattern

  **Documentation References**:
  - `schema.sql:1-82` — existing table creation patterns (UUID PK, IF NOT EXISTS, triggers, indexes)

  **Acceptance Criteria**:

  ```bash
  # Schema SQL is valid (syntax check via psql dry run)
  # Agent verifies by reading the file and confirming it follows schema.sql patterns

  # Tests pass (requires DATABASE_URL)
  go test -v ./internal/db/ -run "TestCSP"
  # Assert: all PASS (or SKIP if no DB connection)

  # Build succeeds
  go build .
  # Assert: exit code 0

  go vet ./internal/db/
  # Assert: no warnings
  ```

  **Commit**: YES
  - Message: `feat(db): add CSP watchlist schema and CRUD operations`
  - Files: `schema_csp.sql`, `internal/db/csp.go`, `internal/db/csp_test.go`
  - Pre-commit: `go build . && go vet ./internal/db/`

---

- [ ] 5. TUI Integration — CSP Advisor Tab

  **What to do**:

  Create `csp_view.go` in the root package with all CSP tab UI logic. Wire it into main.go with minimal changes.

  **RED phase** — No TDD for TUI rendering (tview is hard to unit test). Instead, verify via build + manual verification commands.

  **Implementation** — `csp_view.go`:
  ```go
  package main

  // CSP Advisor tab UI code

  // initCSPView sets up the CSP table and related UI components
  func (a *App) initCSPView() { ... }

  // updateCSPTable refreshes the CSP advisor table with latest data
  func (a *App) updateCSPTable() { ... }

  // refreshCSPData fetches options data and computes scores for all watchlist tickers
  func (a *App) refreshCSPData() { ... }

  // showAddCSPWatchForm shows the form to add a ticker to CSP watchlist
  func (a *App) showAddCSPWatchForm() { ... }

  // showRemoveCSPWatchConfirm confirms removal of a ticker from watchlist
  func (a *App) showRemoveCSPWatchConfirm(index int) { ... }
  ```

  **CSP Table columns**:
  `| TICKER | PRICE | CSP SCORE | VIX | IV RANK | RSI | P/C RATIO | YIELD | SIGNAL |`

  - TICKER: magenta (matching existing style)
  - PRICE: cyan
  - CSP SCORE: color-coded (>70 lime, 50-70 yellow, <50 red)
  - VIX/IV RANK/RSI/P/C RATIO/YIELD: white, raw values
  - SIGNAL: "STRONG" (lime), "MODERATE" (yellow), "WEAK" (red)

  **Changes to `main.go`** (minimal):
  1. Add fields to `App` struct:
     ```go
     cspTable     *tview.Table
     cspStatusBar *tview.TextView
     cspSection   *tview.Flex
     cspWatchlist []db.CSPWatchItem
     cspScores    map[string]csp.SignalOutput
     showCSP      bool // toggles CSP view visibility
     ```
  2. In `run()`: call `a.initCSPView()` after existing UI setup
  3. Add keybinding `case 'p':` to toggle CSP view
  4. In CSP view, `+` or `a` adds ticker, `d` removes ticker, `r` refreshes
  5. When `showCSP` is true, replace main layout with CSP layout. When false, show normal layout.

  **Layout when CSP active**:
  ```
  ┌─────────────────────────────────────────┐
  │             ANYHOWHODL (header)         │
  ├─────────────────────────────────────────┤
  │  CSP Advisor                            │
  │  TICKER  PRICE  SCORE  VIX  IV  RSI .. │
  │  AAPL    $185   72     22   65  35  .. │
  │  MSFT    $420   58     22   45  42  .. │
  ├─────────────────────────────────────────┤
  │  Status: p:Portfolio  +:Add  d:Remove  │
  └─────────────────────────────────────────┘
  ```

  **Data flow**:
  1. `refreshCSPData()` gets watchlist from DB
  2. For each ticker: fetch options chain + price history (sequential, 200ms delay)
  3. Fetch VIX once (shared across all tickers)
  4. For each ticker: `csp.SelectTargetContract()` → find target put
  5. Collect IV values across all available expiries for IV Rank calculation
  6. `csp.ComputeSignals()` → store in `a.cspScores` map
  7. `updateCSPTable()` renders the table

  **Must NOT do**:
  - Do not restructure existing App struct layout
  - Do not change existing keybindings
  - Do not add CSP auto-refresh
  - Do not display full options chain
  - Keep main.go changes under 30 lines (all logic in csp_view.go)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
  - **Skills**: [`frontend-ui-ux`]
    - `frontend-ui-ux`: TUI layout, color coding, table styling consistent with existing design

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 2 (final integration)
  - **Blocks**: None
  - **Blocked By**: Tasks 2, 3, 4

  **References**:

  **Pattern References**:
  - `main.go:94-99` — Table creation pattern (SetBorders, SetSelectable, SetFixed, SetSelectedStyle)
  - `main.go:430-444` — Table header creation pattern (cell styling, color scheme)
  - `main.go:486-648` — Table row population pattern (data display, color coding, alignment)
  - `main.go:134-136` — Status bar pattern
  - `main.go:683-765` — Form creation pattern (AddInputField, auto-uppercase, validation)
  - `main.go:166-167` — Pages usage pattern (AddPage)
  - `main.go:170-256` — Input capture pattern (keybinding handler)
  - `main.go:302-367` — refreshData pattern (status bar, ctx, error handling, sequential data load)
  - `main.go:1700-1717` — createModalPage pattern for dialogs
  - `main.go:1720-1731` — styleForm helper pattern

  **API/Type References**:
  - `internal/csp/csp.go:SignalInput` — input struct for ComputeSignals
  - `internal/csp/csp.go:SignalOutput` — output struct with scores and signal string
  - `internal/csp/csp.go:OptionsData` — parsed options chain data
  - `internal/db/csp.go:CSPWatchItem` — watchlist item from DB
  - `internal/yahoo/options.go:FetchOptionsChain` — options chain fetcher
  - `internal/yahoo/options.go:FetchPriceHistory` — price history fetcher
  - `internal/yahoo/yahoo.go:131-133` — GetQuote for VIX fetch

  **Acceptance Criteria**:

  ```bash
  # Build succeeds with all new code integrated
  go build -o anyhowhodl .
  # Assert: exit code 0

  # Vet passes
  go vet ./...
  # Assert: no warnings

  # All tests still pass
  go test ./...
  # Assert: all PASS

  # Verify keybinding 'p' exists exactly once in input handler
  grep -n "case 'p'" main.go | wc -l
  # Assert: 1

  # Verify csp_view.go exists and has expected functions
  grep -c "func (a \*App)" csp_view.go
  # Assert: >= 5 (initCSPView, updateCSPTable, refreshCSPData, showAddCSPWatchForm, showRemoveCSPWatchConfirm)

  # Verify no new dependencies added
  go mod tidy && git diff go.mod go.sum
  # Assert: no changes (no new deps)
  ```

  **Evidence to Capture:**
  - [ ] Terminal output from `go build .`
  - [ ] Terminal output from `go test ./...`

  **Commit**: YES
  - Message: `feat(csp): add CSP advisor TUI tab with score display`
  - Files: `csp_view.go`, `main.go` (minimal changes)
  - Pre-commit: `go build . && go test ./... && go vet ./...`

---

## Commit Strategy

| After Task | Message | Files | Verification |
|------------|---------|-------|--------------|
| 2 | `feat(csp): add signal calculation engine with TDD tests` | `internal/csp/csp.go`, `internal/csp/csp_test.go` | `go test ./internal/csp/` |
| 3 | `feat(yahoo): add options chain and price history fetchers` | `internal/yahoo/options.go`, `internal/yahoo/options_test.go`, `internal/yahoo/testdata/*.json` | `go test ./internal/yahoo/ -run TestParse` |
| 4 | `feat(db): add CSP watchlist schema and CRUD operations` | `schema_csp.sql`, `internal/db/csp.go`, `internal/db/csp_test.go` | `go build . && go vet ./internal/db/` |
| 5 | `feat(csp): add CSP advisor TUI tab with score display` | `csp_view.go`, `main.go` | `go build . && go test ./... && go vet ./...` |

---

## Success Criteria

### Verification Commands
```bash
go build -o anyhowhodl .        # Builds without errors
go test ./...                    # All tests pass
go vet ./...                     # No warnings
grep "case 'p'" main.go         # Keybinding exists
ls internal/csp/csp.go          # Signal engine exists
ls internal/yahoo/options.go    # Options fetcher exists
ls internal/db/csp.go           # Watchlist CRUD exists
ls csp_view.go                  # TUI tab exists
ls schema_csp.sql               # Migration exists
```

### Final Checklist
- [ ] All "Must Have" present (5 signals, watchlist, add/remove, color coding, graceful degradation)
- [ ] All "Must NOT Have" absent (no B-S, no history storage, no alerts, no chain display, no new deps)
- [ ] All tests pass
- [ ] main.go changes are under 30 lines
- [ ] CSP tab displays correctly with mock/real data
