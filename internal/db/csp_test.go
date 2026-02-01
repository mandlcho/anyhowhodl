package db

import (
	"context"
	"os"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	d, err := New(url)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() {
		d.pool.Exec(context.Background(), `DELETE FROM csp_watchlist`)
		d.Close()
	})
	d.pool.Exec(context.Background(), `DELETE FROM csp_watchlist`)
	return d
}

func TestAddCSPWatchlistTicker(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	err := d.AddCSPWatchTicker(ctx, "AAPL", "test note")
	if err != nil {
		t.Fatalf("AddCSPWatchTicker: %v", err)
	}

	items, err := d.GetCSPWatchlist(ctx)
	if err != nil {
		t.Fatalf("GetCSPWatchlist: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Ticker != "AAPL" {
		t.Errorf("expected ticker AAPL, got %s", items[0].Ticker)
	}
	if items[0].Notes != "test note" {
		t.Errorf("expected notes 'test note', got %q", items[0].Notes)
	}
	if items[0].ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestAddCSPWatchlistDuplicate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	err := d.AddCSPWatchTicker(ctx, "MSFT", "first")
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	err = d.AddCSPWatchTicker(ctx, "MSFT", "second")
	if err == nil {
		t.Fatal("expected error on duplicate ticker, got nil")
	}
}

func TestRemoveCSPWatchlistTicker(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	_ = d.AddCSPWatchTicker(ctx, "TSLA", "")
	err := d.RemoveCSPWatchTicker(ctx, "TSLA")
	if err != nil {
		t.Fatalf("RemoveCSPWatchTicker: %v", err)
	}

	items, err := d.GetCSPWatchlist(ctx)
	if err != nil {
		t.Fatalf("GetCSPWatchlist: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items after remove, got %d", len(items))
	}
}

func TestGetCSPWatchlist(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	items, err := d.GetCSPWatchlist(ctx)
	if err != nil {
		t.Fatalf("GetCSPWatchlist empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}

	_ = d.AddCSPWatchTicker(ctx, "MSFT", "")
	_ = d.AddCSPWatchTicker(ctx, "AAPL", "apple notes")
	_ = d.AddCSPWatchTicker(ctx, "TSLA", "")

	items, err = d.GetCSPWatchlist(ctx)
	if err != nil {
		t.Fatalf("GetCSPWatchlist: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].Ticker != "AAPL" || items[1].Ticker != "MSFT" || items[2].Ticker != "TSLA" {
		t.Errorf("unexpected order: %s, %s, %s", items[0].Ticker, items[1].Ticker, items[2].Ticker)
	}
}
