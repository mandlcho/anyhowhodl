package db

import (
	"context"
	"time"
)

type CSPWatchItem struct {
	ID        string
	Ticker    string
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (d *DB) AddCSPWatchTicker(ctx context.Context, ticker, notes string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO csp_watchlist (ticker, notes) VALUES ($1, $2)`,
		ticker, notes)
	return err
}

func (d *DB) RemoveCSPWatchTicker(ctx context.Context, ticker string) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM csp_watchlist WHERE ticker = $1`, ticker)
	return err
}

func (d *DB) GetCSPWatchlist(ctx context.Context) ([]CSPWatchItem, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, ticker, notes, created_at, updated_at FROM csp_watchlist ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CSPWatchItem
	for rows.Next() {
		var item CSPWatchItem
		var notes *string
		err := rows.Scan(&item.ID, &item.Ticker, &notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if notes != nil {
			item.Notes = *notes
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
