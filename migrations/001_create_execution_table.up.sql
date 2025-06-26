-- Create execution table
CREATE TABLE IF NOT EXISTS execution (
    id SERIAL PRIMARY KEY,
    execution_service_id INTEGER NOT NULL UNIQUE,
    is_open BOOLEAN NOT NULL DEFAULT true,
    execution_status VARCHAR(20) NOT NULL,
    trade_type VARCHAR(10) NOT NULL,
    destination VARCHAR(20) NOT NULL,
    trade_date DATE NOT NULL,
    security_id CHAR(24) NOT NULL,
    ticker VARCHAR(20) NOT NULL,
    portfolio_id CHAR(24),
    quantity DECIMAL(18,8) NOT NULL,
    limit_price DECIMAL(18,8),
    received_timestamp TIMESTAMPTZ NOT NULL,
    sent_timestamp TIMESTAMPTZ NOT NULL,
    last_fill_timestamp TIMESTAMPTZ,
    quantity_filled DECIMAL(18,8) NOT NULL DEFAULT 0,
    total_amount DECIMAL(18,8) DEFAULT 0,
    average_price DECIMAL(18,8) NOT NULL,
    ready_to_send_timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 1
);

-- Create indexes
CREATE INDEX IF NOT EXISTS execution_execution_service_id_ndx ON execution(execution_service_id);
CREATE INDEX IF NOT EXISTS execution_ready_to_send_timestamp_ndx ON execution(ready_to_send_timestamp);

-- Create batch_history table
CREATE TABLE IF NOT EXISTS batch_history (
    id SERIAL PRIMARY KEY,
    start_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    previous_start_time TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

-- Create unique indexes for batch_history
CREATE UNIQUE INDEX IF NOT EXISTS batch_history_start_time_ndx ON batch_history(start_time);
CREATE UNIQUE INDEX IF NOT EXISTS batch_history_previous_start_time_ndx ON batch_history(previous_start_time); 