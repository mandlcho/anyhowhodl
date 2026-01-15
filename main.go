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
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0)

	a.table.SetSelectedFunc(func(row, column int) {
		if row > 0 && row <= len(a.holdings) {
			a.showHoldingActions(row - 1)
		}
	})

	// Status bar
	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow]a[white]:Add  [yellow]d[white]:Delete  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")

	// Summary bar
	a.summary = tview.NewTextView().SetDynamicColors(true)

	// Main layout
	mainFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.createHeader(), 3, 0, false).
		AddItem(a.table, 0, 1, true).
		AddItem(a.summary, 3, 0, false).
		AddItem(a.statusBar, 1, 0, false)

	a.pages = tview.NewPages().
		AddPage("main", mainFlex, true, true)

	// Key bindings
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case 'a':
			a.showAddForm()
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
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("\n[green::b]ANYHOWHODL[white] - Portfolio Tracker")
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
	a.statusBar.SetText(" [yellow]a[white]:Add  [yellow]d[white]:Delete  [yellow]r[white]:Refresh  [yellow]q[white]:Quit")
}

func (a *App) updateTable() {
	a.table.Clear()

	// Header row
	headers := []string{"TICKER", "QTY", "AVG COST", "PRICE", "VALUE", "P/L", "P/L %", "DAY CHG"}
	for i, h := range headers {
		cell := tview.NewTableCell(h).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignRight).
			SetSelectable(false)
		if i == 0 {
			cell.SetAlign(tview.AlignLeft)
		}
		a.table.SetCell(0, i, cell)
	}

	var totalCost, totalValue decimal.Decimal

	for i, h := range a.holdings {
		row := i + 1

		// Ticker
		a.table.SetCell(row, 0, tview.NewTableCell(h.Ticker).
			SetAlign(tview.AlignLeft))

		// Quantity
		a.table.SetCell(row, 1, tview.NewTableCell(h.Quantity.StringFixed(2)).
			SetAlign(tview.AlignRight))

		// Avg Cost
		a.table.SetCell(row, 2, tview.NewTableCell("$"+h.AvgCost.StringFixed(2)).
			SetAlign(tview.AlignRight))

		quote, hasQuote := a.quotes[h.Ticker]
		costBasis := h.Quantity.Mul(h.AvgCost)
		totalCost = totalCost.Add(costBasis)

		if hasQuote {
			price := decimal.NewFromFloat(quote.Price)
			value := h.Quantity.Mul(price)
			totalValue = totalValue.Add(value)
			pl := value.Sub(costBasis)
			plPct := decimal.Zero
			if !costBasis.IsZero() {
				plPct = pl.Div(costBasis).Mul(decimal.NewFromInt(100))
			}

			// Price
			a.table.SetCell(row, 3, tview.NewTableCell("$"+price.StringFixed(2)).
				SetAlign(tview.AlignRight))

			// Value
			a.table.SetCell(row, 4, tview.NewTableCell("$"+value.StringFixed(2)).
				SetAlign(tview.AlignRight))

			// P/L
			plColor := tcell.ColorWhite
			if pl.IsPositive() {
				plColor = tcell.ColorGreen
			} else if pl.IsNegative() {
				plColor = tcell.ColorRed
			}
			plSign := ""
			if pl.IsPositive() {
				plSign = "+"
			}
			a.table.SetCell(row, 5, tview.NewTableCell(plSign+"$"+pl.StringFixed(2)).
				SetTextColor(plColor).
				SetAlign(tview.AlignRight))

			// P/L %
			pctSign := ""
			if plPct.IsPositive() {
				pctSign = "+"
			}
			a.table.SetCell(row, 6, tview.NewTableCell(pctSign+plPct.StringFixed(2)+"%").
				SetTextColor(plColor).
				SetAlign(tview.AlignRight))

			// Day change
			dayChgColor := tcell.ColorWhite
			if quote.ChangePercent > 0 {
				dayChgColor = tcell.ColorGreen
			} else if quote.ChangePercent < 0 {
				dayChgColor = tcell.ColorRed
			}
			daySign := ""
			if quote.ChangePercent > 0 {
				daySign = "+"
			}
			a.table.SetCell(row, 7, tview.NewTableCell(fmt.Sprintf("%s%.2f%%", daySign, quote.ChangePercent)).
				SetTextColor(dayChgColor).
				SetAlign(tview.AlignRight))
		} else {
			totalValue = totalValue.Add(costBasis)
			a.table.SetCell(row, 3, tview.NewTableCell("-").SetAlign(tview.AlignRight))
			a.table.SetCell(row, 4, tview.NewTableCell("-").SetAlign(tview.AlignRight))
			a.table.SetCell(row, 5, tview.NewTableCell("-").SetAlign(tview.AlignRight))
			a.table.SetCell(row, 6, tview.NewTableCell("-").SetAlign(tview.AlignRight))
			a.table.SetCell(row, 7, tview.NewTableCell("-").SetAlign(tview.AlignRight))
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

	summaryText := fmt.Sprintf("\n [white]Total: [green]$%s[white]  |  Cost: $%s  |  P/L: %s%s$%s (%s%.2f%%)",
		totalValue.StringFixed(2),
		totalCost.StringFixed(2),
		plColor, plSign, totalPL.Abs().StringFixed(2),
		plSign, totalPLPct.InexactFloat64())

	a.summary.SetText(summaryText)
}

func (a *App) showAddForm() {
	form := tview.NewForm().
		AddInputField("Ticker", "", 10, nil, nil).
		AddInputField("Quantity", "", 15, nil, nil).
		AddInputField("Avg Cost ($)", "", 15, nil, nil).
		AddInputField("Entry Date (YYYY-MM-DD)", time.Now().Format("2006-01-02"), 15, nil, nil).
		AddInputField("Notes", "", 30, nil, nil)

	form.AddButton("Save", func() {
		ticker := strings.ToUpper(form.GetFormItem(0).(*tview.InputField).GetText())
		qtyStr := form.GetFormItem(1).(*tview.InputField).GetText()
		costStr := form.GetFormItem(2).(*tview.InputField).GetText()
		dateStr := form.GetFormItem(3).(*tview.InputField).GetText()
		notes := form.GetFormItem(4).(*tview.InputField).GetText()

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

		entryDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			a.statusBar.SetText(" [red]Invalid date format")
			return
		}

		ctx := context.Background()
		if err := a.db.AddHolding(ctx, ticker, qty, cost, entryDate, notes); err != nil {
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

	form := tview.NewForm().
		AddInputField("Quantity", h.Quantity.String(), 15, nil, nil).
		AddInputField("Avg Cost ($)", h.AvgCost.String(), 15, nil, nil).
		AddInputField("Notes", h.Notes, 30, nil, nil)

	form.AddButton("Save", func() {
		qtyStr := form.GetFormItem(0).(*tview.InputField).GetText()
		costStr := form.GetFormItem(1).(*tview.InputField).GetText()
		notes := form.GetFormItem(2).(*tview.InputField).GetText()

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

		ctx := context.Background()
		if err := a.db.UpdateHolding(ctx, h.ID, qty, cost, notes); err != nil {
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
