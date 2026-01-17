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
