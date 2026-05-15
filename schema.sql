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

-- Audit log of executed orders. `idempotency_key` is a client-supplied
-- token; the UNIQUE constraint lets us treat duplicate POSTs as no-ops
-- and return the original result instead of double-debiting.
CREATE TABLE orders (
    id              TEXT NOT NULL,
    customer_id     TEXT NOT NULL,
    order_type      TEXT NOT NULL,
    quantity        TEXT NOT NULL,
    quoted_price    TEXT NOT NULL,
    total           TEXT NOT NULL,
    new_balance     TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE (idempotency_key)
);
