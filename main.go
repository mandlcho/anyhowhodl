package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"anyhowhodl/internal/db"
	"anyhowhodl/internal/yahoo"

	"github.com/gdamore/tcell/v2"
	"github.com/joho/godotenv"
	"github.com/rivo/tview"
	"github.com/shopspring/decimal"
)

type App struct {
	db           *db.DB
	yahoo        *yahoo.Client
	app          *tview.Application
	pages        *tview.Pages
	table        *tview.Table
	optionsTable *tview.Table
	timeline     *tview.TextView
	statusBar    *tview.TextView
	summary      *tview.TextView
	holdings     []db.Holding
	options      []db.Option
	quotes       map[string]yahoo.Quote
	cash         decimal.Decimal
	premiums     *db.PremiumSummary
	focusIndex   int // 0 = holdings table, 1 = options table
}

func main() {
	// Load .env file
	godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Println("DATABASE_URL not set. Please create a .env file with your Supabase connection string.")
		fmt.Println("See .env.example for the format.")
		os.Exit(1)
	}

	// Connect to database
	database, err := db.New(dbURL)
	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	app := &App{
		db:     database,
		yahoo:  yahoo.NewClient(),
		quotes: make(map[string]yahoo.Quote),
	}

	app.run()
}

func (a *App) run() {
	a.app = tview.NewApplication()

	// Create holdings table
	a.table = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0).
		SetSeparator(' ').
		SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorDarkSlateGray))

	a.table.SetSelectedFunc(func(row, column int) {
		if row > 0 && row <= len(a.holdings) {
			a.showHoldingActions(row - 1)
		}
	})

	// Create options table
	a.optionsTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0).
		SetSeparator(' ').
		SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorDarkSlateGray))

	a.optionsTable.SetSelectedFunc(func(row, column int) {
		if row > 0 && row <= len(a.options) {
			a.showOptionActions(row - 1)
		}
	})

	// Timeline view
	a.timeline = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.timeline.SetBorder(true).SetTitle(" Expiration Timeline ").SetTitleAlign(tview.AlignLeft).SetBorderColor(tcell.ColorTeal)

	// Status bar
	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow]a[white]:Add Holding  [yellow]o[white]:Add Option  [yellow]c[white]:Cash  [yellow]Tab[white]:Switch  [yellow]d[white]:Delete  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")

	// Summary bar
	a.summary = tview.NewTextView().SetDynamicColors(true)

	// Options section (table + timeline)
	optionsSection := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.optionsTable, 0, 1, false).
		AddItem(a.timeline, 5, 0, false)

	// Main layout - holdings gets 3:2 space vs options
	mainFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.createHeader(), 8, 0, false).
		AddItem(a.table, 0, 3, true).
		AddItem(optionsSection, 0, 2, false).
		AddItem(a.summary, 2, 0, false).
		AddItem(a.statusBar, 1, 0, false)

	a.pages = tview.NewPages().
		AddPage("main", mainFlex, true, true)

	// Key bindings - only active on main page
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Only handle shortcuts when on main page
		name, _ := a.pages.GetFrontPage()
		if name != "main" {
			return event
		}

		// Tab to switch focus between tables
		if event.Key() == tcell.KeyTab {
			a.focusIndex = (a.focusIndex + 1) % 2
			if a.focusIndex == 0 {
				a.app.SetFocus(a.table)
			} else {
				a.app.SetFocus(a.optionsTable)
			}
			return nil
		}

		switch event.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case 'a':
			a.showAddForm()
			return nil
		case 'o':
			a.showAddOptionForm()
			return nil
		case 'c':
			a.showCashForm()
			return nil
		case 'd':
			if a.focusIndex == 0 {
				row, _ := a.table.GetSelection()
				if row > 0 && row <= len(a.holdings) {
					a.confirmDelete(row - 1)
				}
			} else {
				row, _ := a.optionsTable.GetSelection()
				if row > 0 && row <= len(a.options) {
					a.confirmDeleteOption(row - 1)
				}
			}
			return nil
		case 'r':
			a.refreshData()
			return nil
		}
		return event
	})

	// Initial data load
	a.refreshData()

	if err := a.app.SetRoot(a.pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func (a *App) createHeader() *tview.TextView {
	ascii := "\n[teal::b]" +
		" █████╗ ███╗   ██╗██╗   ██╗██╗  ██╗ ██████╗ ██╗    ██╗██╗  ██╗ ██████╗ ██████╗ ██╗     \n" +
		"██╔══██╗████╗  ██║╚██╗ ██╔╝██║  ██║██╔═══██╗██║    ██║██║  ██║██╔═══██╗██╔══██╗██║     \n" +
		"███████║██╔██╗ ██║ ╚████╔╝ ███████║██║   ██║██║ █╗ ██║███████║██║   ██║██║  ██║██║     \n" +
		"██╔══██║██║╚██╗██║  ╚██╔╝  ██╔══██║██║   ██║██║███╗██║██╔══██║██║   ██║██║  ██║██║     \n" +
		"██║  ██║██║ ╚████║   ██║   ██║  ██║╚██████╔╝╚███╔███╔╝██║  ██║╚██████╔╝██████╔╝███████╗\n" +
		"╚═╝  ╚═╝╚═╝  ╚═══╝   ╚═╝   ╚═╝  ╚═╝ ╚═════╝  ╚══╝╚══╝ ╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝[-:-:-]"
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(ascii)
	return header
}

func (a *App) refreshData() {
	a.statusBar.SetText(" [yellow]Loading...")
	a.app.ForceDraw()

	ctx := context.Background()

	// Get holdings from DB
	holdings, err := a.db.GetHoldings(ctx)
	if err != nil {
		a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
		return
	}
	a.holdings = holdings

	// Get available cash
	cash, err := a.db.GetAvailableCash(ctx)
	if err != nil {
		cash = decimal.Zero
	}
	a.cash = cash

	// Process expired options first (auto-assign or expire based on ITM/OTM)
	a.processExpiredOptions(ctx)

	// Get active options
	options, err := a.db.GetActiveOptions(ctx)
	if err != nil {
		options = []db.Option{}
	}
	a.options = options

	// Get premium summary for current year
	currentYear := time.Now().Year()
	premiums, err := a.db.GetPremiumsByYear(ctx, currentYear)
	if err != nil {
		premiums = &db.PremiumSummary{}
	}
	a.premiums = premiums

	// Get unique tickers
	tickers := make([]string, 0)
	tickerMap := make(map[string]bool)
	for _, h := range holdings {
		if !tickerMap[h.Ticker] {
			tickers = append(tickers, h.Ticker)
			tickerMap[h.Ticker] = true
		}
	}

	// Fetch quotes
	if len(tickers) > 0 {
		quotes, err := a.yahoo.GetQuotes(tickers)
		if err != nil {
			a.statusBar.SetText(fmt.Sprintf(" [yellow]Prices unavailable: %v", err))
		} else {
			a.quotes = quotes
		}
	}

	a.updateTable()
	a.updateOptionsTable()
	a.updateTimeline()
	a.statusBar.SetText(" [yellow]a[white]:Add Holding  [yellow]o[white]:Add Option  [yellow]c[white]:Cash  [yellow]Tab[white]:Switch  [yellow]d[white]:Delete  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")
}

func (a *App) updateTable() {
	a.table.Clear()

	// Header row - cyan color scheme
	headers := []string{"TICKER", "QTY", "AVG COST", "PRICE", "VALUE", "P/L", "P/L %", "WEIGHT", "vs HIGH", "SIGNAL"}
	for i, h := range headers {
		cell := tview.NewTableCell(" "+h+" ").
			SetTextColor(tcell.ColorBlack).
			SetBackgroundColor(tcell.ColorTeal).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(1)
		a.table.SetCell(0, i, cell)
	}

	// First pass: calculate total portfolio value
	var totalCost, totalValue decimal.Decimal
	positionValues := make([]decimal.Decimal, len(a.holdings))

	for i, h := range a.holdings {
		quote, hasQuote := a.quotes[h.Ticker]
		costBasis := h.Quantity.Mul(h.AvgCost)
		totalCost = totalCost.Add(costBasis)

		if hasQuote {
			price := decimal.NewFromFloat(quote.Price)
			value := h.Quantity.Mul(price)
			positionValues[i] = value
			totalValue = totalValue.Add(value)
		} else {
			positionValues[i] = costBasis
			totalValue = totalValue.Add(costBasis)
		}
	}

	// Second pass: populate table with weight %
	for i, h := range a.holdings {
		row := i + 1
		rowBg := tcell.ColorBlack

		// Ticker - magenta/purple for visibility
		a.table.SetCell(row, 0, tview.NewTableCell(" "+h.Ticker+" ").
			SetTextColor(tcell.ColorFuchsia).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Quantity
		a.table.SetCell(row, 1, tview.NewTableCell(" "+formatNumber(h.Quantity.StringFixed(2))+" ").
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Avg Cost
		a.table.SetCell(row, 2, tview.NewTableCell(" $"+formatNumber(h.AvgCost.StringFixed(2))+" ").
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		quote, hasQuote := a.quotes[h.Ticker]
		costBasis := h.Quantity.Mul(h.AvgCost)
		value := positionValues[i]

		// Calculate weight
		weight := decimal.Zero
		if !totalValue.IsZero() {
			weight = value.Div(totalValue).Mul(decimal.NewFromInt(100))
		}

		if hasQuote {
			price := decimal.NewFromFloat(quote.Price)
			pl := value.Sub(costBasis)
			plPct := decimal.Zero
			if !costBasis.IsZero() {
				plPct = pl.Div(costBasis).Mul(decimal.NewFromInt(100))
			}

			// Price - cyan
			a.table.SetCell(row, 3, tview.NewTableCell(" $"+formatNumber(price.StringFixed(2))+" ").
				SetTextColor(tcell.ColorAqua).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))

			// Value - yellow
			a.table.SetCell(row, 4, tview.NewTableCell(" $"+formatNumber(value.StringFixed(2))+" ").
				SetTextColor(tcell.ColorYellow).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))

			// P/L
			plColor := tcell.ColorWhite
			if pl.IsPositive() {
				plColor = tcell.ColorLime
			} else if pl.IsNegative() {
				plColor = tcell.ColorRed
			}
			plSign := ""
			if pl.IsPositive() {
				plSign = "+"
			}
			a.table.SetCell(row, 5, tview.NewTableCell(" "+plSign+"$"+formatNumber(pl.StringFixed(2))+" ").
				SetTextColor(plColor).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))

			// P/L %
			pctSign := ""
			if plPct.IsPositive() {
				pctSign = "+"
			}
			a.table.SetCell(row, 6, tview.NewTableCell(" "+pctSign+formatNumber(plPct.StringFixed(2))+"% ").
				SetTextColor(plColor).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))

			// Weight % - orange if > 25%, red if > 40%
			weightColor := tcell.ColorWhite
			if weight.GreaterThan(decimal.NewFromInt(40)) {
				weightColor = tcell.ColorRed
			} else if weight.GreaterThan(decimal.NewFromInt(25)) {
				weightColor = tcell.ColorOrange
			}
			a.table.SetCell(row, 7, tview.NewTableCell(" "+formatNumber(weight.StringFixed(1))+"% ").
				SetTextColor(weightColor).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))

			// % from 52-week high - green if big dip (buying opportunity)
			pctFromHigh := quote.PctFromHigh
			highPrice := decimal.NewFromFloat(quote.FiftyTwoWeekHigh)
			highColor := tcell.ColorWhite
			highText := fmt.Sprintf(" %.1f%% ($%s) ", pctFromHigh, formatNumber(highPrice.StringFixed(2)))
			if pctFromHigh <= -20 {
				highColor = tcell.ColorLime // Big dip - potential buy
			} else if pctFromHigh <= -10 {
				highColor = tcell.ColorYellow // Moderate dip
			}
			a.table.SetCell(row, 8, tview.NewTableCell(highText).
				SetTextColor(highColor).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))

			// SIGNAL - based on target price vs current price
			signalText := " - "
			signalColor := tcell.ColorWhite
			if h.TargetPrice.Valid {
				target := h.TargetPrice.Decimal
				if price.LessThan(target) {
					signalText = " BUY "
					signalColor = tcell.ColorLime
				} else {
					signalText = " SELL "
					signalColor = tcell.ColorRed
				}
			}
			a.table.SetCell(row, 9, tview.NewTableCell(signalText).
				SetTextColor(signalColor).
				SetBackgroundColor(rowBg).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))
		} else {
			a.table.SetCell(row, 3, tview.NewTableCell(" - ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
			a.table.SetCell(row, 4, tview.NewTableCell(" - ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
			a.table.SetCell(row, 5, tview.NewTableCell(" - ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
			a.table.SetCell(row, 6, tview.NewTableCell(" - ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
			a.table.SetCell(row, 7, tview.NewTableCell(" "+formatNumber(weight.StringFixed(1))+"% ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
			a.table.SetCell(row, 8, tview.NewTableCell(" - ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
			a.table.SetCell(row, 9, tview.NewTableCell(" - ").SetBackgroundColor(rowBg).SetAlign(tview.AlignLeft).SetExpansion(1))
		}
	}

	// Update summary
	totalPL := totalValue.Sub(totalCost)
	totalPLPct := decimal.Zero
	if !totalCost.IsZero() {
		totalPLPct = totalPL.Div(totalCost).Mul(decimal.NewFromInt(100))
	}

	plColor := "[white]"
	if totalPL.IsPositive() {
		plColor = "[green]"
	} else if totalPL.IsNegative() {
		plColor = "[red]"
	}

	plSign := ""
	if totalPL.IsPositive() {
		plSign = "+"
	}

	// Total portfolio = holdings value + cash
	totalPortfolio := totalValue.Add(a.cash)

	summaryText := fmt.Sprintf(" [white]Total: [yellow]$%s[white]  |  Holdings: $%s  |  Cash: [aqua]$%s[white]  |  P/L: %s%s$%s (%s%.2f%%)",
		formatNumber(totalPortfolio.StringFixed(2)),
		formatNumber(totalValue.StringFixed(2)),
		formatNumber(a.cash.StringFixed(2)),
		plColor, plSign, formatNumber(totalPL.Abs().StringFixed(2)),
		plSign, totalPLPct.InexactFloat64())

	a.summary.SetText(summaryText)
}

func (a *App) showAddForm() {
	form := tview.NewForm().
		AddInputField("Ticker", "", 10, nil, nil).
		AddInputField("Quantity", "", 15, nil, nil)

	// Auto-uppercase ticker
	tickerField := form.GetFormItem(0).(*tview.InputField)
	tickerField.SetChangedFunc(func(text string) {
		upper := strings.ToUpper(text)
		if text != upper {
			tickerField.SetText(upper)
		}
	})

	form.
		AddInputField("Avg Cost ($)", "", 15, nil, nil).
		AddInputField("Target Price ($)", "", 15, nil, nil).
		AddInputField("Entry Date (YYYY-MM-DD)", time.Now().Format("2006-01-02"), 15, nil, nil).
		AddInputField("Notes", "", 30, nil, nil)

	// Style the form
	form.SetBackgroundColor(tcell.ColorBlack)
	form.SetFieldBackgroundColor(tcell.ColorDarkSlateGray)
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetLabelColor(tcell.ColorTeal)
	form.SetButtonBackgroundColor(tcell.ColorTeal)
	form.SetButtonTextColor(tcell.ColorBlack)
	form.SetBorderColor(tcell.ColorTeal)
	form.SetTitleColor(tcell.ColorTeal)

	form.AddButton("Save", func() {
		ticker := strings.ToUpper(form.GetFormItem(0).(*tview.InputField).GetText())
		qtyStr := form.GetFormItem(1).(*tview.InputField).GetText()
		costStr := form.GetFormItem(2).(*tview.InputField).GetText()
		targetStr := form.GetFormItem(3).(*tview.InputField).GetText()
		dateStr := form.GetFormItem(4).(*tview.InputField).GetText()
		notes := form.GetFormItem(5).(*tview.InputField).GetText()

		if ticker == "" || qtyStr == "" || costStr == "" {
			a.statusBar.SetText(" [red]Ticker, Quantity, and Avg Cost are required")
			return
		}

		qty, err := decimal.NewFromString(qtyStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid quantity")
			return
		}

		cost, err := decimal.NewFromString(costStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid cost")
			return
		}

		var targetPrice decimal.NullDecimal
		if targetStr != "" {
			tp, err := decimal.NewFromString(targetStr)
			if err != nil {
				a.statusBar.SetText(" [red]Invalid target price")
				return
			}
			targetPrice = decimal.NullDecimal{Decimal: tp, Valid: true}
		}

		entryDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid date format")
			return
		}

		ctx := context.Background()
		if err := a.db.AddHolding(ctx, ticker, qty, cost, entryDate, targetPrice, notes); err != nil {
			a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
			return
		}

		a.pages.SwitchToPage("main")
		a.pages.RemovePage("add")
		a.refreshData()
	})

	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.pages.RemovePage("add")
	})

	form.SetBorder(true).SetTitle(" Add Holding ").SetTitleAlign(tview.AlignLeft)

	a.createModalPage("add", form, 50, 15)
}

func (a *App) showHoldingActions(index int) {
	h := a.holdings[index]

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Actions for %s\n%.2f shares @ $%s", h.Ticker, h.Quantity.InexactFloat64(), h.AvgCost.StringFixed(2))).
		AddButtons([]string{"Edit", "Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			switch buttonLabel {
			case "Edit":
				a.pages.RemovePage("actions")
				a.showEditForm(index)
			case "Delete":
				a.pages.RemovePage("actions")
				a.confirmDelete(index)
			default:
				a.pages.RemovePage("actions")
			}
		})

	a.pages.AddPage("actions", modal, true, true)
}

func (a *App) showEditForm(index int) {
	h := a.holdings[index]

	targetStr := ""
	if h.TargetPrice.Valid {
		targetStr = h.TargetPrice.Decimal.String()
	}

	form := tview.NewForm().
		AddInputField("Quantity", h.Quantity.String(), 15, nil, nil).
		AddInputField("Avg Cost ($)", h.AvgCost.String(), 15, nil, nil).
		AddInputField("Target Price ($)", targetStr, 15, nil, nil).
		AddInputField("Notes", h.Notes, 30, nil, nil)

	// Style the form
	form.SetBackgroundColor(tcell.ColorBlack)
	form.SetFieldBackgroundColor(tcell.ColorDarkSlateGray)
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetLabelColor(tcell.ColorTeal)
	form.SetButtonBackgroundColor(tcell.ColorTeal)
	form.SetButtonTextColor(tcell.ColorBlack)
	form.SetBorderColor(tcell.ColorTeal)
	form.SetTitleColor(tcell.ColorTeal)

	form.AddButton("Save", func() {
		qtyStr := form.GetFormItem(0).(*tview.InputField).GetText()
		costStr := form.GetFormItem(1).(*tview.InputField).GetText()
		targetStr := form.GetFormItem(2).(*tview.InputField).GetText()
		notes := form.GetFormItem(3).(*tview.InputField).GetText()

		qty, err := decimal.NewFromString(qtyStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid quantity")
			return
		}

		cost, err := decimal.NewFromString(costStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid cost")
			return
		}

		var targetPrice decimal.NullDecimal
		if targetStr != "" {
			tp, err := decimal.NewFromString(targetStr)
			if err != nil {
				a.statusBar.SetText(" [red]Invalid target price")
				return
			}
			targetPrice = decimal.NullDecimal{Decimal: tp, Valid: true}
		}

		ctx := context.Background()
		if err := a.db.UpdateHolding(ctx, h.ID, qty, cost, targetPrice, notes); err != nil {
			a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
			return
		}

		a.pages.SwitchToPage("main")
		a.pages.RemovePage("edit")
		a.refreshData()
	})

	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.pages.RemovePage("edit")
	})

	form.SetBorder(true).SetTitle(fmt.Sprintf(" Edit %s ", h.Ticker)).SetTitleAlign(tview.AlignLeft)

	a.createModalPage("edit", form, 50, 12)
}

func (a *App) confirmDelete(index int) {
	h := a.holdings[index]

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete %s?\n%.2f shares @ $%s", h.Ticker, h.Quantity.InexactFloat64(), h.AvgCost.StringFixed(2))).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete" {
				ctx := context.Background()
				if err := a.db.DeleteHolding(ctx, h.ID); err != nil {
					a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
				}
				a.refreshData()
			}
			a.pages.RemovePage("confirm")
		})

	a.pages.AddPage("confirm", modal, true, true)
}

func (a *App) showCashForm() {
	form := tview.NewForm()

	saveCash := func() {
		cashStr := form.GetFormItem(0).(*tview.InputField).GetText()

		cash, err := decimal.NewFromString(cashStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid cash amount")
			return
		}

		ctx := context.Background()
		if err := a.db.SetAvailableCash(ctx, cash); err != nil {
			a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
			return
		}

		a.pages.SwitchToPage("main")
		a.pages.RemovePage("cash")
		a.refreshData()
	}

	form.AddInputField("Available Cash ($)", a.cash.StringFixed(2), 15, nil, func(text string) {})
	form.GetFormItem(0).(*tview.InputField).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			saveCash()
		}
	})

	// Style the form
	form.SetBackgroundColor(tcell.ColorBlack)
	form.SetFieldBackgroundColor(tcell.ColorDarkSlateGray)
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetLabelColor(tcell.ColorTeal)
	form.SetButtonBackgroundColor(tcell.ColorTeal)
	form.SetButtonTextColor(tcell.ColorBlack)
	form.SetBorderColor(tcell.ColorTeal)
	form.SetTitleColor(tcell.ColorTeal)

	form.AddButton("Save", saveCash)

	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.pages.RemovePage("cash")
	})

	form.SetBorder(true).SetTitle(" Set Available Cash ").SetTitleAlign(tview.AlignLeft)

	a.createModalPage("cash", form, 45, 9)
}

func (a *App) updateOptionsTable() {
	a.optionsTable.Clear()

	// Header row
	headers := []string{"TICKER", "TYPE", "ACTION", "STRIKE", "EXPIRY", "QTY", "PREMIUM", "DAYS LEFT"}
	for i, h := range headers {
		cell := tview.NewTableCell(" "+h+" ").
			SetTextColor(tcell.ColorBlack).
			SetBackgroundColor(tcell.ColorTeal).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(1)
		a.optionsTable.SetCell(0, i, cell)
	}

	today := time.Now().Truncate(24 * time.Hour)

	for i, o := range a.options {
		row := i + 1
		rowBg := tcell.ColorBlack

		// Ticker
		a.optionsTable.SetCell(row, 0, tview.NewTableCell(" "+o.Ticker+" ").
			SetTextColor(tcell.ColorFuchsia).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Type (CALL/PUT)
		typeColor := tcell.ColorLime
		if o.OptionType == "PUT" {
			typeColor = tcell.ColorRed
		}
		a.optionsTable.SetCell(row, 1, tview.NewTableCell(" "+o.OptionType+" ").
			SetTextColor(typeColor).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Action (BUY/SELL)
		actionColor := tcell.ColorYellow
		if o.Action == "SELL" {
			actionColor = tcell.ColorAqua
		}
		a.optionsTable.SetCell(row, 2, tview.NewTableCell(" "+o.Action+" ").
			SetTextColor(actionColor).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Strike
		a.optionsTable.SetCell(row, 3, tview.NewTableCell(" $"+formatNumber(o.Strike.StringFixed(2))+" ").
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Expiry
		a.optionsTable.SetCell(row, 4, tview.NewTableCell(" "+o.ExpiryDate.Format("2006-01-02")+" ").
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Quantity
		a.optionsTable.SetCell(row, 5, tview.NewTableCell(" "+fmt.Sprintf("%d", o.Quantity)+" ").
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Premium
		a.optionsTable.SetCell(row, 6, tview.NewTableCell(" $"+formatNumber(o.Premium.StringFixed(2))+" ").
			SetTextColor(tcell.ColorYellow).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))

		// Days left
		daysLeft := int(o.ExpiryDate.Sub(today).Hours() / 24)
		daysColor := tcell.ColorWhite
		if daysLeft <= 7 {
			daysColor = tcell.ColorRed
		} else if daysLeft <= 14 {
			daysColor = tcell.ColorYellow
		} else if daysLeft <= 30 {
			daysColor = tcell.ColorOrange
		}
		a.optionsTable.SetCell(row, 7, tview.NewTableCell(" "+fmt.Sprintf("%d", daysLeft)+" ").
			SetTextColor(daysColor).
			SetBackgroundColor(rowBg).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
	}
}

func (a *App) updateTimeline() {
	currentYear := time.Now().Year()

	// Premium summary line
	premiumText := fmt.Sprintf(" [teal]%d Premiums:[white] Calls: [lime]$%s[white]  Puts: [lime]$%s[white]  Total: [yellow]$%s[white]",
		currentYear,
		formatNumber(a.premiums.CallPremiums.StringFixed(2)),
		formatNumber(a.premiums.PutPremiums.StringFixed(2)),
		formatNumber(a.premiums.TotalPremiums.StringFixed(2)))

	if len(a.options) == 0 {
		a.timeline.SetText(premiumText + "\n [gray]No active options")
		return
	}

	today := time.Now().Truncate(24 * time.Hour)
	var timelineText string

	// Expiration timeline
	timelineText = "\n "
	for _, o := range a.options {
		daysLeft := int(o.ExpiryDate.Sub(today).Hours() / 24)

		// Color based on days left
		color := "white"
		if daysLeft <= 7 {
			color = "red"
		} else if daysLeft <= 14 {
			color = "yellow"
		} else if daysLeft <= 30 {
			color = "orange"
		}

		typeSymbol := "C"
		if o.OptionType == "PUT" {
			typeSymbol = "P"
		}

		timelineText += fmt.Sprintf("[%s]%s %s$%s %s (%dd)[white]  ",
			color, o.Ticker, typeSymbol, o.Strike.StringFixed(0), o.ExpiryDate.Format("01/02"), daysLeft)
	}

	a.timeline.SetText(premiumText + timelineText)
}

func (a *App) showAddOptionForm() {
	form := tview.NewForm().
		AddInputField("Ticker", "", 10, nil, nil)

	// Auto-uppercase ticker
	tickerField := form.GetFormItem(0).(*tview.InputField)
	tickerField.SetChangedFunc(func(text string) {
		upper := strings.ToUpper(text)
		if text != upper {
			tickerField.SetText(upper)
		}
	})

	form.
		AddDropDown("Type", []string{"CALL", "PUT"}, 0, nil).
		AddDropDown("Action", []string{"SELL", "BUY"}, 0, nil).
		AddInputField("Strike ($)", "", 15, nil, nil).
		AddInputField("Expiry (YYYY-MM-DD)", "", 15, nil, nil).
		AddInputField("Quantity", "1", 10, nil, nil).
		AddInputField("Premium ($)", "", 15, nil, nil).
		AddInputField("Notes", "", 30, nil, nil)

	// Style the form
	form.SetBackgroundColor(tcell.ColorBlack)
	form.SetFieldBackgroundColor(tcell.ColorDarkSlateGray)
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetLabelColor(tcell.ColorTeal)
	form.SetButtonBackgroundColor(tcell.ColorTeal)
	form.SetButtonTextColor(tcell.ColorBlack)
	form.SetBorderColor(tcell.ColorTeal)
	form.SetTitleColor(tcell.ColorTeal)

	form.AddButton("Save", func() {
		ticker := strings.ToUpper(form.GetFormItem(0).(*tview.InputField).GetText())
		_, optionType := form.GetFormItem(1).(*tview.DropDown).GetCurrentOption()
		_, action := form.GetFormItem(2).(*tview.DropDown).GetCurrentOption()
		strikeStr := form.GetFormItem(3).(*tview.InputField).GetText()
		expiryStr := form.GetFormItem(4).(*tview.InputField).GetText()
		qtyStr := form.GetFormItem(5).(*tview.InputField).GetText()
		premiumStr := form.GetFormItem(6).(*tview.InputField).GetText()
		notes := form.GetFormItem(7).(*tview.InputField).GetText()

		if ticker == "" || strikeStr == "" || expiryStr == "" || premiumStr == "" {
			a.statusBar.SetText(" [red]Ticker, Strike, Expiry, and Premium are required")
			return
		}

		strike, err := decimal.NewFromString(strikeStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid strike price")
			return
		}

		expiry, err := time.Parse("2006-01-02", expiryStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid expiry date format")
			return
		}

		qty, err := strconv.Atoi(qtyStr)
		if err != nil || qty < 1 {
			a.statusBar.SetText(" [red]Invalid quantity")
			return
		}

		premium, err := decimal.NewFromString(premiumStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid premium")
			return
		}

		ctx := context.Background()
		if err := a.db.AddOption(ctx, ticker, optionType, action, strike, expiry, qty, premium, notes); err != nil {
			a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
			return
		}

		a.pages.SwitchToPage("main")
		a.pages.RemovePage("addoption")
		a.refreshData()
	})

	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.pages.RemovePage("addoption")
	})

	form.SetBorder(true).SetTitle(" Add Option ").SetTitleAlign(tview.AlignLeft)

	a.createModalPage("addoption", form, 55, 18)
}

func (a *App) showOptionActions(index int) {
	o := a.options[index]

	typeStr := o.OptionType
	actionDesc := "You receive shares"
	if typeStr == "CALL" {
		actionDesc = "Your shares get called away"
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("%s %s %s $%s\nExpires: %s\n\nAssign: %s", o.Action, o.Ticker, typeStr, o.Strike.StringFixed(2), o.ExpiryDate.Format("2006-01-02"), actionDesc)).
		AddButtons([]string{"Assign", "Expire", "Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			switch buttonLabel {
			case "Assign":
				a.pages.RemovePage("optionactions")
				a.confirmAssignOption(index)
			case "Expire":
				a.pages.RemovePage("optionactions")
				a.confirmExpireOption(index)
			case "Delete":
				a.pages.RemovePage("optionactions")
				a.confirmDeleteOption(index)
			default:
				a.pages.RemovePage("optionactions")
			}
		})

	a.pages.AddPage("optionactions", modal, true, true)
}

func (a *App) confirmDeleteOption(index int) {
	o := a.options[index]

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete %s %s $%s?", o.Ticker, o.OptionType, o.Strike.StringFixed(2))).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete" {
				ctx := context.Background()
				if err := a.db.DeleteOption(ctx, o.ID); err != nil {
					a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
				}
				a.refreshData()
			}
			a.pages.RemovePage("confirmoption")
		})

	a.pages.AddPage("confirmoption", modal, true, true)
}

func (a *App) confirmAssignOption(index int) {
	o := a.options[index]

	shares := o.Quantity * 100
	totalValue := o.Strike.Mul(decimal.NewFromInt(int64(shares)))

	var actionText string
	if o.OptionType == "PUT" {
		actionText = fmt.Sprintf("BUY %d shares of %s @ $%s\nCash: -$%s",
			shares, o.Ticker, o.Strike.StringFixed(2), formatNumber(totalValue.StringFixed(2)))
	} else {
		actionText = fmt.Sprintf("SELL %d shares of %s @ $%s\nCash: +$%s",
			shares, o.Ticker, o.Strike.StringFixed(2), formatNumber(totalValue.StringFixed(2)))
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Assign %s %s $%s?\n\n%s", o.Ticker, o.OptionType, o.Strike.StringFixed(2), actionText)).
		AddButtons([]string{"Confirm", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Confirm" {
				ctx := context.Background()
				if err := a.db.AssignOption(ctx, o.ID); err != nil {
					a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
				} else {
					a.statusBar.SetText(fmt.Sprintf(" [green]Option assigned: %s %s", o.Ticker, o.OptionType))
				}
				a.refreshData()
			}
			a.pages.RemovePage("confirmassign")
		})

	a.pages.AddPage("confirmassign", modal, true, true)
}

func (a *App) confirmExpireOption(index int) {
	o := a.options[index]

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Mark %s %s $%s as expired?\n\nOption expires worthless, no shares exchanged.", o.Ticker, o.OptionType, o.Strike.StringFixed(2))).
		AddButtons([]string{"Confirm", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Confirm" {
				ctx := context.Background()
				if err := a.db.ExpireOption(ctx, o.ID); err != nil {
					a.statusBar.SetText(fmt.Sprintf(" [red]Error: %v", err))
				} else {
					a.statusBar.SetText(fmt.Sprintf(" [green]Option expired: %s %s", o.Ticker, o.OptionType))
				}
				a.refreshData()
			}
			a.pages.RemovePage("confirmexpire")
		})

	a.pages.AddPage("confirmexpire", modal, true, true)
}

func (a *App) processExpiredOptions(ctx context.Context) {
	// Get expired options that are still ACTIVE
	expiredOptions, err := a.db.GetExpiredActiveOptions(ctx)
	if err != nil || len(expiredOptions) == 0 {
		return
	}

	// Get unique tickers
	tickers := make([]string, 0)
	tickerMap := make(map[string]bool)
	for _, o := range expiredOptions {
		if !tickerMap[o.Ticker] {
			tickers = append(tickers, o.Ticker)
			tickerMap[o.Ticker] = true
		}
	}

	// Fetch current prices
	quotes, err := a.yahoo.GetQuotes(tickers)
	if err != nil {
		return
	}

	// Process each expired option
	for _, o := range expiredOptions {
		quote, hasQuote := quotes[o.Ticker]
		if !hasQuote {
			continue
		}

		currentPrice := decimal.NewFromFloat(quote.Price)
		isITM := false

		// CALL is ITM if current price > strike (shares get called away)
		// PUT is ITM if current price < strike (you get assigned shares)
		if o.OptionType == "CALL" {
			isITM = currentPrice.GreaterThan(o.Strike)
		} else {
			isITM = currentPrice.LessThan(o.Strike)
		}

		if isITM {
			// Auto-assign
			a.db.AssignOption(ctx, o.ID)
		} else {
			// Auto-expire (OTM)
			a.db.ExpireOption(ctx, o.ID)
		}
	}
}

func (a *App) createModalPage(name string, content tview.Primitive, width, height int) {
	// Create transparent boxes that capture input but don't obscure background
	leftBox := tview.NewBox()
	rightBox := tview.NewBox()
	topBox := tview.NewBox()
	bottomBox := tview.NewBox()

	// Center the content with input-capturing spacers
	flex := tview.NewFlex().
		AddItem(leftBox, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(topBox, 0, 1, false).
			AddItem(content, height, 0, true).
			AddItem(bottomBox, 0, 1, false), width, 0, true).
		AddItem(rightBox, 0, 1, false)

	a.pages.AddPage(name, flex, true, true)
}

func formatNumber(s string) string {
	parts := strings.Split(s, ".")
	intPart := parts[0]

	negative := false
	if strings.HasPrefix(intPart, "-") {
		negative = true
		intPart = intPart[1:]
	}

	var result []byte
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}

	formatted := string(result)
	if len(parts) > 1 {
		formatted += "." + parts[1]
	}
	if negative {
		formatted = "-" + formatted
	}
	return formatted
}

// Helper to parse float - not used but kept for potential future use
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
