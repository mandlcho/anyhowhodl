-- Run this in your Supabase SQL Editor to create the holdings table

CREATE TABLE IF NOT EXISTS holdings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticker VARCHAR(10) NOT NULL,
    quantity DECIMAL(18, 8) NOT NULL,
    avg_cost DECIMAL(18, 4) NOT NULL,
    entry_date DATE NOT NULL DEFAULT CURRENT_DATE,
    target_price DECIMAL(18, 4),
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Migration: Add target_price column if it doesn't exist
-- Run this if you already have the table:
-- ALTER TABLE holdings ADD COLUMN IF NOT EXISTS target_price DECIMAL(18, 4);

-- Index for faster ticker lookups
CREATE INDEX IF NOT EXISTS idx_holdings_ticker ON holdings(ticker);

-- Trigger to auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

DROP TRIGGER IF EXISTS update_holdings_updated_at ON holdings;
CREATE TRIGGER update_holdings_updated_at
    BEFORE UPDATE ON holdings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Settings table for portfolio-level settings like available cash
CREATE TABLE IF NOT EXISTS settings (
    key VARCHAR(50) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Initialize available cash to 0
INSERT INTO settings (key, value) VALUES ('available_cash', '0')
ON CONFLICT (key) DO NOTHING;

-- Options table for tracking option contracts
CREATE TABLE IF NOT EXISTS options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticker VARCHAR(10) NOT NULL,
    option_type VARCHAR(4) NOT NULL CHECK (option_type IN ('CALL', 'PUT')),
    action VARCHAR(4) NOT NULL CHECK (action IN ('BUY', 'SELL')),
    strike DECIMAL(18, 2) NOT NULL,
    expiry_date DATE NOT NULL,
    quantity INTEGER NOT NULL,
    premium DECIMAL(18, 4) NOT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'EXPIRED', 'ASSIGNED')),
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Migration: Add status column if it doesn't exist
-- ALTER TABLE options ADD COLUMN IF NOT EXISTS status VARCHAR(10) NOT NULL DEFAULT 'ACTIVE';

-- Index for faster expiry lookups
CREATE INDEX IF NOT EXISTS idx_options_expiry ON options(expiry_date);
CREATE INDEX IF NOT EXISTS idx_options_ticker ON options(ticker);

-- Trigger for options updated_at
DROP TRIGGER IF EXISTS update_options_updated_at ON options;
CREATE TRIGGER update_options_updated_at
    BEFORE UPDATE ON options
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
