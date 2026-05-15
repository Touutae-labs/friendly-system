-- Atlas-managed schema. Apply with `make seed`.
-- All amounts are stored as TEXT so shopspring/decimal can round-trip without precision loss.

CREATE TABLE accounts (
    customer_id TEXT NOT NULL,
    balance     TEXT NOT NULL,
    PRIMARY KEY (customer_id)
);

CREATE TABLE daily_totals (
    customer_id TEXT NOT NULL,
    day         TEXT NOT NULL,
    total       TEXT NOT NULL,
    PRIMARY KEY (customer_id, day)
);
