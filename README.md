# anyhowhodl

A terminal-based portfolio + options tracker that uses Supabase (Postgres) as the data store and Yahoo Finance for live quotes.

Built with Go + `tview` for a fast keyboard-first UI.

## Problem

Spreadsheets work until they don’t:
- options legs and expiries are tedious to track,
- cash and premium accounting becomes error-prone,
- and “what expires soon?” is not visible at a glance.

anyhowhodl is a lightweight TUI for maintaining holdings + options positions with automatic premium/cash adjustments and an expiry timeline.

## Who it’s for

- Individual investors tracking a simple equity + options book
- Anyone who prefers terminal workflows and wants just enough structure without a full platform
- Builders who want a reference implementation of Go + Supabase + TUI

## Goals

- Quickly add/update holdings and options trades
- Show portfolio totals (holdings value + cash) and position weights
- Track option premium P&L (gross, fees, buybacks, net)
- Visualize time-to-expiry and highlight near-term risk

## Success metrics

- Setup time (new DB + run app): < 20 minutes
- Data entry speed: add a holding/option in < 30 seconds
- Options expiring in ≤ 7 days are obvious
- Premium/cash adjustments match trade entries

## Features

- Holdings table:
  - ticker, qty, avg cost, live price, value, P/L, weight
  - optional target price + signal column
  - highlights % distance from 52-week high (via Yahoo meta)
- Options table:
  - CALL/PUT, BUY/SELL, strike, expiry, qty, premium, fees, status
  - status color coding + days-left indicator
- Premium stats:
  - yearly premiums by CALL/PUT, fees, buyback cost, net P&L
  - return % based on capital-at-risk approximation
- Expiry timeline:
  - weekly/monthly view toggle
  - colored urgency (≤7d red, ≤14d yellow, etc.)
- Auto-processing for expired ACTIVE options:
  - attempts to auto-assign ITM and auto-expire OTM based on current price vs strike

## Scope

- Tracking holdings/options, premium stats, and cash
- Single-user workflow backed by a Supabase database

## Non-goals

- Broker integrations / real trade execution
- Complex multi-leg strategies modeling
- Tax reporting
- Guaranteed quote reliability (Yahoo endpoints can rate-limit)

## Constraints / assumptions

- Requires a Postgres connection string (Supabase recommended)
- Yahoo Finance quote endpoint may fail intermittently; UI should degrade gracefully
- Assumes options contract multiplier of 100
- “Auto-assign ITM” is based on current spot vs strike and does not model settlement nuance

## Data model

See `schema.sql` to create:
- `holdings`
- `options`
- `settings` (stores `available_cash`)

## Setup (Supabase)

1. Create a Supabase project
2. Open SQL Editor and run `schema.sql`
3. Get the connection string:
   - Project Settings → Database → Connection string (URI)

## Configure

Create `.env`:

```env
DATABASE_URL=postgresql://postgres:[YOUR-PASSWORD]@db.[YOUR-PROJECT-REF].supabase.co:5432/postgres
```

(see `.env.example` if present)

## Run locally

Prereqs: Go (Go 1.21+ recommended)

```bash
go mod tidy
go run .
```

## Roadmap

- Add README screenshots/gif (holdings/options/timeline)
- Add import/export (CSV) for faster onboarding
- Add per-position notes view and filtering
- Improve expired options handling (manual review mode)
- Add basic unit tests around DB cash adjustments and premium summary queries
