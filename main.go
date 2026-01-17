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
	db          *db.DB
	yahoo       *yahoo.Client
	app         *tview.Application
	pages       *tview.Pages
	table       *tview.Table
	statusBar   *tview.TextView
	summary     *tview.TextView
	holdings    []db.Holding
	quotes      map[string]yahoo.Quote
	cash        decimal.Decimal
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

	// Create main table
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

	// Status bar
	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow]a[white]:Add  [yellow]c[white]:Cash  [yellow]d[white]:Delete  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")

	// Summary bar
	a.summary = tview.NewTextView().SetDynamicColors(true)

	// Main layout
	mainFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.createHeader(), 8, 0, false).
		AddItem(a.table, 0, 1, true).
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

		switch event.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case 'a':
			a.showAddForm()
			return nil
		case 'c':
			a.showCashForm()
			return nil
		case 'd':
			row, _ := a.table.GetSelection()
			if row > 0 && row <= len(a.holdings) {
				a.confirmDelete(row - 1)
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
	a.statusBar.SetText(" [yellow]a[white]:Add  [yellow]c[white]:Cash  [yellow]d[white]:Delete  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")
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
			highColor := tcell.ColorWhite
			highText := fmt.Sprintf(" %.1f%% ", pctFromHigh)
			if pctFromHigh <= -20 {
				highColor = tcell.ColorLime // Big dip - potential buy
				highText = fmt.Sprintf(" %.1f%% DIP ", pctFromHigh)
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
		AddInputField("Quantity", "", 15, nil, nil).
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

	// Center the form
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 15, 1, true).
			AddItem(nil, 0, 1, false), 50, 1, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("add", flex, true, true)
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

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 12, 1, true).
			AddItem(nil, 0, 1, false), 50, 1, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("edit", flex, true, true)
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
	form := tview.NewForm().
		AddInputField("Available Cash ($)", a.cash.StringFixed(2), 15, nil, nil)

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
	})

	form.AddButton("Cancel", func() {
		a.pages.SwitchToPage("main")
		a.pages.RemovePage("cash")
	})

	form.SetBorder(true).SetTitle(" Set Available Cash ").SetTitleAlign(tview.AlignLeft)

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 9, 1, true).
			AddItem(nil, 0, 1, false), 45, 1, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("cash", flex, true, true)
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
