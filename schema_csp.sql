-- CSP Advisor watchlist table
-- Run this in your Supabase SQL Editor

CREATE TABLE IF NOT EXISTS csp_watchlist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticker VARCHAR(10) NOT NULL UNIQUE,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for faster ticker lookups
CREATE INDEX IF NOT EXISTS idx_csp_watchlist_ticker ON csp_watchlist(ticker);

-- Trigger to auto-update updated_at
-- (uses update_updated_at_column function from schema.sql)
DROP TRIGGER IF EXISTS update_csp_watchlist_updated_at ON csp_watchlist;
CREATE TRIGGER update_csp_watchlist_updated_at
    BEFORE UPDATE ON csp_watchlist
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
