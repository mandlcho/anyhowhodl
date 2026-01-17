package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type Holding struct {
	ID          string
	Ticker      string
	Quantity    decimal.Decimal
	AvgCost     decimal.Decimal
	EntryDate   time.Time
	TargetPrice decimal.NullDecimal
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Option struct {
	ID         string
	Ticker     string
	OptionType string // CALL or PUT
	Action     string // BUY or SELL
	Strike     decimal.Decimal
	ExpiryDate time.Time
	Quantity   int
	Premium    decimal.Decimal
	Status     string // ACTIVE, EXPIRED, ASSIGNED
	Notes      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DB struct {
	pool *pgxpool.Pool
}

func New(databaseURL string) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	// Disable prepared statements for Supabase transaction pooler compatibility
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close() {
	d.pool.Close()
}

func (d *DB) AddHolding(ctx context.Context, ticker string, quantity, avgCost decimal.Decimal, entryDate time.Time, targetPrice decimal.NullDecimal, notes string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO holdings (ticker, quantity, avg_cost, entry_date, target_price, notes) VALUES ($1, $2, $3, $4, $5, $6)`,
		ticker, quantity, avgCost, entryDate, targetPrice, notes)
	return err
}

func (d *DB) GetHoldings(ctx context.Context) ([]Holding, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, ticker, quantity, avg_cost, entry_date, target_price, notes, created_at, updated_at FROM holdings ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var holdings []Holding
	for rows.Next() {
		var h Holding
		var targetPrice *decimal.Decimal
		var notes *string
		err := rows.Scan(&h.ID, &h.Ticker, &h.Quantity, &h.AvgCost, &h.EntryDate, &targetPrice, &notes, &h.CreatedAt, &h.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if targetPrice != nil {
			h.TargetPrice = decimal.NullDecimal{Decimal: *targetPrice, Valid: true}
		}
		if notes != nil {
			h.Notes = *notes
		}
		holdings = append(holdings, h)
	}
	return holdings, rows.Err()
}

func (d *DB) UpdateHolding(ctx context.Context, id string, quantity, avgCost decimal.Decimal, targetPrice decimal.NullDecimal, notes string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE holdings SET quantity = $2, avg_cost = $3, target_price = $4, notes = $5 WHERE id = $1`,
		id, quantity, avgCost, targetPrice, notes)
	return err
}

func (d *DB) DeleteHolding(ctx context.Context, id string) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM holdings WHERE id = $1`, id)
	return err
}

func (d *DB) GetHoldingByTicker(ctx context.Context, ticker string) (*Holding, error) {
	var h Holding
	var targetPrice *decimal.Decimal
	var notes *string
	err := d.pool.QueryRow(ctx,
		`SELECT id, ticker, quantity, avg_cost, entry_date, target_price, notes, created_at, updated_at FROM holdings WHERE ticker = $1`,
		ticker).Scan(&h.ID, &h.Ticker, &h.Quantity, &h.AvgCost, &h.EntryDate, &targetPrice, &notes, &h.CreatedAt, &h.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if targetPrice != nil {
		h.TargetPrice = decimal.NullDecimal{Decimal: *targetPrice, Valid: true}
	}
	if notes != nil {
		h.Notes = *notes
	}
	return &h, nil
}

func (d *DB) GetAvailableCash(ctx context.Context) (decimal.Decimal, error) {
	var value string
	err := d.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'available_cash'`).Scan(&value)
	if err == pgx.ErrNoRows {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(value)
}

func (d *DB) SetAvailableCash(ctx context.Context, amount decimal.Decimal) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES ('available_cash', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW()`,
		amount.String())
	return err
}

func (d *DB) AddOption(ctx context.Context, ticker, optionType, action string, strike decimal.Decimal, expiryDate time.Time, quantity int, premium decimal.Decimal, notes string) error {
	// Insert the option
	_, err := d.pool.Exec(ctx,
		`INSERT INTO options (ticker, option_type, action, strike, expiry_date, quantity, premium, status, notes) VALUES ($1, $2, $3, $4, $5, $6, $7, 'ACTIVE', $8)`,
		ticker, optionType, action, strike, expiryDate, quantity, premium, notes)
	if err != nil {
		return err
	}

	// Auto-adjust cash based on action
	// SELL = receive premium, BUY = pay premium
	premiumTotal := premium.Mul(decimal.NewFromInt(int64(quantity))).Mul(decimal.NewFromInt(100))

	currentCash, err := d.GetAvailableCash(ctx)
	if err != nil {
		currentCash = decimal.Zero
	}

	if action == "SELL" {
		currentCash = currentCash.Add(premiumTotal)
	} else {
		currentCash = currentCash.Sub(premiumTotal)
	}

	return d.SetAvailableCash(ctx, currentCash)
}

func (d *DB) GetActiveOptions(ctx context.Context) ([]Option, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, ticker, option_type, action, strike, expiry_date, quantity, premium, status, notes, created_at, updated_at
		 FROM options
		 WHERE status = 'ACTIVE'
		 ORDER BY expiry_date, ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []Option
	for rows.Next() {
		var o Option
		var notes *string
		err := rows.Scan(&o.ID, &o.Ticker, &o.OptionType, &o.Action, &o.Strike, &o.ExpiryDate, &o.Quantity, &o.Premium, &o.Status, &notes, &o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if notes != nil {
			o.Notes = *notes
		}
		options = append(options, o)
	}
	return options, rows.Err()
}

func (d *DB) UpdateOption(ctx context.Context, id string, strike decimal.Decimal, expiryDate time.Time, quantity int, premium decimal.Decimal, notes string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE options SET strike = $2, expiry_date = $3, quantity = $4, premium = $5, notes = $6 WHERE id = $1`,
		id, strike, expiryDate, quantity, premium, notes)
	return err
}

func (d *DB) DeleteOption(ctx context.Context, id string) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM options WHERE id = $1`, id)
	return err
}

func (d *DB) ExpireOption(ctx context.Context, id string) error {
	_, err := d.pool.Exec(ctx, `UPDATE options SET status = 'EXPIRED' WHERE id = $1`, id)
	return err
}

func (d *DB) AssignOption(ctx context.Context, id string) error {
	// Get the option details first
	var o Option
	var notes *string
	err := d.pool.QueryRow(ctx,
		`SELECT id, ticker, option_type, action, strike, expiry_date, quantity, premium, status, notes FROM options WHERE id = $1`, id).
		Scan(&o.ID, &o.Ticker, &o.OptionType, &o.Action, &o.Strike, &o.ExpiryDate, &o.Quantity, &o.Premium, &o.Status, &notes)
	if err != nil {
		return err
	}

	// Calculate total value (strike × quantity × 100)
	totalValue := o.Strike.Mul(decimal.NewFromInt(int64(o.Quantity))).Mul(decimal.NewFromInt(100))
	shares := decimal.NewFromInt(int64(o.Quantity * 100))

	// Get current cash
	currentCash, err := d.GetAvailableCash(ctx)
	if err != nil {
		currentCash = decimal.Zero
	}

	if o.OptionType == "PUT" {
		// PUT assigned: we buy shares at strike price
		// Deduct cash, add to holdings
		currentCash = currentCash.Sub(totalValue)

		// Check if holding exists, update or create
		existing, err := d.GetHoldingByTicker(ctx, o.Ticker)
		if err != nil {
			return err
		}

		if existing != nil {
			// Update existing holding with new average cost
			totalShares := existing.Quantity.Add(shares)
			totalCost := existing.Quantity.Mul(existing.AvgCost).Add(shares.Mul(o.Strike))
			newAvgCost := totalCost.Div(totalShares)
			err = d.UpdateHolding(ctx, existing.ID, totalShares, newAvgCost, existing.TargetPrice, existing.Notes)
		} else {
			// Create new holding
			err = d.AddHolding(ctx, o.Ticker, shares, o.Strike, time.Now(), decimal.NullDecimal{}, "Assigned from PUT option")
		}
		if err != nil {
			return err
		}
	} else {
		// CALL assigned: we sell shares at strike price
		// Add cash, remove from holdings
		currentCash = currentCash.Add(totalValue)

		// Find and reduce/remove holding
		existing, err := d.GetHoldingByTicker(ctx, o.Ticker)
		if err != nil {
			return err
		}

		if existing != nil {
			remainingShares := existing.Quantity.Sub(shares)
			if remainingShares.LessThanOrEqual(decimal.Zero) {
				// Remove holding entirely
				err = d.DeleteHolding(ctx, existing.ID)
			} else {
				// Reduce holding
				err = d.UpdateHolding(ctx, existing.ID, remainingShares, existing.AvgCost, existing.TargetPrice, existing.Notes)
			}
			if err != nil {
				return err
			}
		}
	}

	// Update cash
	err = d.SetAvailableCash(ctx, currentCash)
	if err != nil {
		return err
	}

	// Mark option as assigned
	_, err = d.pool.Exec(ctx, `UPDATE options SET status = 'ASSIGNED' WHERE id = $1`, id)
	return err
}

type PremiumSummary struct {
	CallPremiums decimal.Decimal
	PutPremiums  decimal.Decimal
	TotalPremiums decimal.Decimal
}

func (d *DB) GetPremiumsByYear(ctx context.Context, year int) (*PremiumSummary, error) {
	var callPremiums, putPremiums decimal.Decimal

	// Get CALL premiums sold
	err := d.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(premium * quantity * 100), 0) FROM options
		 WHERE action = 'SELL' AND option_type = 'CALL'
		 AND EXTRACT(YEAR FROM created_at) = $1`, year).Scan(&callPremiums)
	if err != nil {
		return nil, err
	}

	// Get PUT premiums sold
	err = d.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(premium * quantity * 100), 0) FROM options
		 WHERE action = 'SELL' AND option_type = 'PUT'
		 AND EXTRACT(YEAR FROM created_at) = $1`, year).Scan(&putPremiums)
	if err != nil {
		return nil, err
	}

	return &PremiumSummary{
		CallPremiums:  callPremiums,
		PutPremiums:   putPremiums,
		TotalPremiums: callPremiums.Add(putPremiums),
	}, nil
}
