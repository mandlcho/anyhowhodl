package main

import (
	"context"
	"fmt"
	"math"
	"time"

	"anyhowhodl/internal/csp"
	"anyhowhodl/internal/db"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// initCSPView sets up the CSP table and related UI components
func (a *App) initCSPView() {
	// Create CSP table
	a.cspTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0).
		SetSeparator(' ').
		SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorDarkSlateGray))

	// Create status bar
	a.cspStatusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	// Create layout
	a.cspSection = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.cspTable, 0, 1, true).
		AddItem(a.cspStatusBar, 1, 0, false)

	// Initialize data structures
	a.cspScores = make(map[string]csp.SignalOutput)
	a.cspWatchlist = []db.CSPWatchItem{}
}

// updateCSPTable refreshes the CSP advisor table with latest data
func (a *App) updateCSPTable() {
	a.cspTable.Clear()

	// Header row
	headers := []string{"TICKER", "PRICE", "STRIKE", "DTE", "DELTA", "CSP SCORE", "VIX", "IV RANK", "RSI", "P/C", "YIELD", "SIGNAL"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		a.cspTable.SetCell(0, col, cell)
	}

	// Data rows
	row := 1
	for _, item := range a.cspWatchlist {
		ticker := item.Ticker
		score, hasScore := a.cspScores[ticker]

		// Get quote for current price
		quote, hasQuote := a.quotes[ticker]
		priceStr := "N/A"
		if hasQuote {
			priceStr = fmt.Sprintf("$%.2f", quote.Price)
		}

		// Ticker column
		a.cspTable.SetCell(row, 0, tview.NewTableCell(ticker).
			SetTextColor(tcell.ColorFuchsia).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// Price column
		a.cspTable.SetCell(row, 1, tview.NewTableCell(priceStr).
			SetTextColor(tcell.ColorAqua).
			SetAlign(tview.AlignRight).
			SetExpansion(1))

		if !hasScore {
			// No score data available
			for col := 2; col < len(headers); col++ {
				a.cspTable.SetCell(row, col, tview.NewTableCell("N/A").
					SetTextColor(tcell.ColorDimGray).
					SetAlign(tview.AlignCenter).
					SetExpansion(1))
			}
			row++
			continue
		}

		// Extract contract info from the score metadata (stored during refresh)
		contractInfo, hasContract := a.cspContractInfo[ticker]

		// Strike column
		strikeStr := "N/A"
		if hasContract && contractInfo.Strike > 0 {
			strikeStr = fmt.Sprintf("$%.2f", contractInfo.Strike)
		}
		a.cspTable.SetCell(row, 2, tview.NewTableCell(strikeStr).
			SetTextColor(tcell.ColorAqua).
			SetAlign(tview.AlignRight).
			SetExpansion(1))

		// DTE column
		dteStr := "N/A"
		if hasContract && contractInfo.DTE > 0 {
			dteStr = fmt.Sprintf("%d", contractInfo.DTE)
		}
		a.cspTable.SetCell(row, 3, tview.NewTableCell(dteStr).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// Delta column
		deltaStr := "N/A"
		if hasContract && contractInfo.Delta != 0 {
			deltaStr = fmt.Sprintf("%.2f", contractInfo.Delta)
		}
		a.cspTable.SetCell(row, 4, tview.NewTableCell(deltaStr).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// CSP Score column
		scoreColor := tcell.ColorRed
		if score.CompositeScore >= 70 {
			scoreColor = tcell.ColorLime
		} else if score.CompositeScore >= 50 {
			scoreColor = tcell.ColorYellow
		}
		a.cspTable.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%.1f", score.CompositeScore)).
			SetTextColor(scoreColor).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// VIX column
		a.cspTable.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%.1f", score.RawVIX)).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// IV Rank column
		ivRankStr := "N/A"
		if !math.IsNaN(score.RawIVRank) {
			ivRankStr = fmt.Sprintf("%.1f", score.RawIVRank)
		}
		a.cspTable.SetCell(row, 7, tview.NewTableCell(ivRankStr).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// RSI column
		rsiStr := "N/A"
		if !math.IsNaN(score.RawRSI) {
			rsiStr = fmt.Sprintf("%.1f", score.RawRSI)
		}
		a.cspTable.SetCell(row, 8, tview.NewTableCell(rsiStr).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// P/C Ratio column
		a.cspTable.SetCell(row, 9, tview.NewTableCell(fmt.Sprintf("%.2f", score.RawPutCallRatio)).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// Yield column
		a.cspTable.SetCell(row, 10, tview.NewTableCell(fmt.Sprintf("%.1f%%", score.RawPremiumYield)).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		// Signal column
		signalColor := tcell.ColorRed
		if score.Signal == "STRONG" {
			signalColor = tcell.ColorLime
		} else if score.Signal == "MODERATE" {
			signalColor = tcell.ColorYellow
		}
		a.cspTable.SetCell(row, 11, tview.NewTableCell(score.Signal).
			SetTextColor(signalColor).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))

		row++
	}

	// Update status bar
	a.updateCSPStatusBar()
}

// refreshCSPData fetches options data and computes scores for all watchlist tickers
func (a *App) refreshCSPData() {
	ctx := context.Background()

	// Update status
	a.cspStatusBar.Clear()
	fmt.Fprintf(a.cspStatusBar, "[yellow]Loading CSP data...")
	a.app.Draw()

	// Get watchlist from DB
	watchlist, err := a.db.GetCSPWatchlist(ctx)
	if err != nil {
		a.cspStatusBar.Clear()
		fmt.Fprintf(a.cspStatusBar, "[red]Error loading watchlist: %v", err)
		return
	}
	a.cspWatchlist = watchlist

	if len(a.cspWatchlist) == 0 {
		a.cspStatusBar.Clear()
		fmt.Fprintf(a.cspStatusBar, "[yellow]No tickers in watchlist. Press [white]a[yellow] to add.")
		a.updateCSPTable()
		return
	}

	// Fetch VIX once (shared across all tickers)
	vixQuote, err := a.yahoo.GetQuote("^VIX")
	vix := 20.0 // Default VIX if fetch fails
	if err == nil && vixQuote != nil {
		vix = vixQuote.Price
	}

	// Fetch quotes for all tickers (for current prices)
	tickers := make([]string, len(a.cspWatchlist))
	for i, item := range a.cspWatchlist {
		tickers[i] = item.Ticker
	}
	quotes, _ := a.yahoo.GetQuotes(tickers)
	a.quotes = quotes

	// Initialize contract info map
	a.cspContractInfo = make(map[string]ContractInfo)

	// Process each ticker sequentially (with delay to avoid rate limiting)
	for i, item := range a.cspWatchlist {
		ticker := item.Ticker

		// Update status
		a.cspStatusBar.Clear()
		fmt.Fprintf(a.cspStatusBar, "[yellow]Loading %s (%d/%d)...", ticker, i+1, len(a.cspWatchlist))
		a.app.Draw()

		// Fetch options chain
		optionsData, err := a.yahoo.FetchOptionsChain(ticker)
		if err != nil {
			a.cspScores[ticker] = csp.SignalOutput{}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Fetch price history for RSI
		priceHistory, err := a.yahoo.FetchPriceHistory(ticker)
		if err != nil || len(priceHistory) < 15 {
			a.cspScores[ticker] = csp.SignalOutput{}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Select target contract
		targetContract := csp.SelectTargetContract(*optionsData)
		if targetContract == nil {
			a.cspScores[ticker] = csp.SignalOutput{}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Calculate IV Rank (collect all IVs from puts)
		var allIVs []float64
		for _, put := range optionsData.Puts {
			if put.ImpliedVolatility > 0 {
				allIVs = append(allIVs, put.ImpliedVolatility)
			}
		}

		currentIV := targetContract.ImpliedVolatility
		ivLow52w := currentIV
		ivHigh52w := currentIV
		if len(allIVs) > 0 {
			for _, iv := range allIVs {
				if iv < ivLow52w {
					ivLow52w = iv
				}
				if iv > ivHigh52w {
					ivHigh52w = iv
				}
			}
		}

		// Calculate total put/call volume for P/C ratio
		var totalPutVolume, totalCallVolume float64
		for _, put := range optionsData.Puts {
			totalPutVolume += float64(put.Volume)
		}
		for _, call := range optionsData.Calls {
			totalCallVolume += float64(call.Volume)
		}

		// Compute DTE
		expTime := time.Unix(targetContract.Expiration, 0)
		dte := int(time.Until(expTime).Hours() / 24)
		if dte < 0 {
			dte = 0
		}

		// Build signal input
		input := csp.SignalInput{
			VIX:             vix,
			CurrentIV:       currentIV,
			IVHigh52w:       ivHigh52w,
			IVLow52w:        ivLow52w,
			ClosingPrices:   priceHistory,
			TotalPutVolume:  totalPutVolume,
			TotalCallVolume: totalCallVolume,
			PutPremium:      (targetContract.Bid + targetContract.Ask) / 2,
			StrikePrice:     targetContract.Strike,
			DTE:             dte,
		}

		// Compute signals
		output := csp.ComputeSignals(input)
		a.cspScores[ticker] = output

		// Store contract info for display
		a.cspContractInfo[ticker] = ContractInfo{
			Strike: targetContract.Strike,
			DTE:    dte,
			Delta:  targetContract.Delta,
		}

		// Rate limiting
		time.Sleep(200 * time.Millisecond)
	}

	// Update table and status
	a.updateCSPTable()
}

// updateCSPStatusBar updates the CSP status bar
func (a *App) updateCSPStatusBar() {
	a.cspStatusBar.Clear()
	fmt.Fprintf(a.cspStatusBar, "[lime]CSP Advisor[white] | [yellow]p[white]:Portfolio  [yellow]a[white]:Add  [yellow]d[white]:Remove  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")
}

// showAddCSPWatchForm shows the form to add a ticker to CSP watchlist
func (a *App) showAddCSPWatchForm() {
	form := tview.NewForm()
	form.SetBorder(true).
		SetTitle(" Add to CSP Watchlist ").
		SetTitleAlign(tview.AlignCenter)

	ticker := ""
	notes := ""

	form.AddInputField("Ticker", "", 10, func(text string, lastChar rune) bool {
		// Auto-uppercase and limit to letters
		if lastChar >= 'a' && lastChar <= 'z' {
			lastChar = lastChar - 'a' + 'A'
		}
		return (lastChar >= 'A' && lastChar <= 'Z') || lastChar == 0
	}, func(text string) {
		ticker = text
	})

	form.AddInputField("Notes (optional)", "", 50, nil, func(text string) {
		notes = text
	})

	form.AddButton("Add", func() {
		if ticker == "" {
			return
		}

		ctx := context.Background()
		err := a.db.AddCSPWatchTicker(ctx, ticker, notes)
		if err != nil {
			a.pages.RemovePage("add_csp_watch")
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Failed to add ticker: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					a.pages.RemovePage("error")
				})
			a.pages.AddPage("error", errorModal, true, true)
			return
		}

		a.pages.RemovePage("add_csp_watch")
		a.refreshCSPData()
	})

	form.AddButton("Cancel", func() {
		a.pages.RemovePage("add_csp_watch")
	})

	styleForm(form)

	a.pages.AddPage("add_csp_watch", form, true, true)
}

// showRemoveCSPWatchConfirm confirms removal of a ticker from watchlist
func (a *App) showRemoveCSPWatchConfirm(index int) {
	if index < 0 || index >= len(a.cspWatchlist) {
		return
	}

	ticker := a.cspWatchlist[index].Ticker

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Remove %s from CSP watchlist?", ticker)).
		AddButtons([]string{"Remove", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.pages.RemovePage("confirm_remove_csp")
			if buttonLabel == "Remove" {
				ctx := context.Background()
				err := a.db.RemoveCSPWatchTicker(ctx, ticker)
				if err != nil {
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to remove ticker: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.pages.RemovePage("error")
						})
					a.pages.AddPage("error", errorModal, true, true)
					return
				}
				a.refreshCSPData()
			}
		})

	a.pages.AddPage("confirm_remove_csp", modal, true, true)
}

// ContractInfo stores selected contract details for display
type ContractInfo struct {
	Strike float64
	DTE    int
	Delta  float64
}
