# Part 1 — Analysis of the Flawed `process_gold_order` (English version)

**Submitted by:** Pantakan Totae · `pantakan.totae@gmail.com`
**Date:** 2026-05-15
**Repo:** see `README.md` for layout and how to run.
**Thai version:** [`Part_1.md`](Part_1.md)

---

## 1. What the function does

`process_gold_order` opens a SQLite connection to `trading.db`, looks up the customer's `balance` and `name` by `customer_id`, then branches on `order_type`:

- **buy:** computes `quantity * price`. If the balance covers the cost, it debits the balance, inserts a row into `orders`, commits, and returns `{"status": "success", "balance": new_balance}`. Otherwise it returns a failure dict.
- **sell:** always credits the balance with `quantity * price` — no validation of any kind — inserts a row, commits, and returns success.
- Anything else falls through and returns `None`.

## 2. Distinct problems, why they matter, and proposed fixes

I found more than four issues; the six below are the ones I would fix before this code went anywhere near a production trade. Ranked by blast radius.

### Problem 1 — SQL injection in every query

Every SQL statement is built with string concatenation or f-string interpolation against caller-supplied values:

```python
"SELECT balance, name FROM customers WHERE id = " + str(customer_id)
f"UPDATE customers SET balance = {new_balance} WHERE id = {customer_id}"
f"INSERT INTO orders (...) VALUES ({customer_id}, '{order_type}', ...)"
```

A malicious `customer_id` of `1; UPDATE customers SET balance = 999999999 WHERE id = 1;--` executes immediately. In a financial system the worst case is an attacker who can rewrite any customer's balance, drop the orders table, or exfiltrate the entire customer list.

**Fix:** use parameterised queries and let the driver handle escaping.

```python
conn.execute("SELECT balance, name FROM customers WHERE id = ?", (customer_id,))
conn.execute("UPDATE customers SET balance = ? WHERE id = ?", (new_balance, customer_id))
conn.execute("INSERT INTO orders (...) VALUES (?, ?, ?, ?, ?)", (...))
```

Modern ORMs (SQLAlchemy, Django ORM, GORM, etc.) handle parameter binding automatically, so for non-raw queries you rarely need to think about escaping at all.

### Problem 2 — Float arithmetic for money

`total_cost = quantity * price` and `balance - total_cost` operate on Python `float` (IEEE-754 binary). `0.1 + 0.2 == 0.30000000000000004`. Over thousands of trades these errors accumulate and break reconciliation against external books, violate regulator-required rounding rules, and confuse customers whose balance shifts by a fraction of a satang. There is no acceptable amount of silent precision loss in a balances ledger.

**Fix:** use `decimal.Decimal` with an explicit rounding mode, or store all amounts as integer minor units (satang). My Part 2/3 module uses `shopspring/decimal` for the same reason.

### Problem 3 — No exception handling; connection lingers on DB error

There is no `try`/`except` around the DB writes and no `conn.close()` on any return path.

**What actually happens in sqlite3 default mode:** `UPDATE` and `INSERT` both sit inside the same implicit transaction. If `INSERT` raises an exception before `conn.commit()` is called, the exception propagates unhandled, `conn.commit()` is never reached, and **neither** statement is committed — so the balance is not incorrectly debited. The real damage is that the connection stays open with an uncommitted transaction holding a file lock, which blocks any other writer until the connection is eventually garbage-collected.

**Why the pattern is still dangerous:** if this code is ever run against a driver with autocommit enabled — psycopg3's default, or psycopg2 with `conn.autocommit = True` (common in connection-pool setups) — `UPDATE` commits immediately without waiting for `INSERT`. The customer is debited with no corresponding order record. This is a latent bug that activates on a driver swap.

**Fix:** wrap both writes in an explicit transaction context, handle the exception, and always close the connection.

```python
conn = sqlite3.connect("trading.db")
try:
    with conn:                        # auto-commit / auto-rollback
        conn.execute("UPDATE …", (new_balance, customer_id))
        conn.execute("INSERT …", (customer_id, order_type, quantity, price, total))
except sqlite3.Error:
    return {"status": "failed", "reason": "persistence error"}
finally:
    conn.close()
```

**Note:** `with sqlite3.connect(...) as conn:` does **not** close the connection — Python's sqlite3 context manager only handles commit/rollback. Closing requires an explicit `conn.close()` (or `try/finally`).

### Problem 4 — Race condition: two concurrent buys can overdraw

Even with a transaction, the read-then-check-then-write pattern is not safe under concurrency. Two buy requests for the same customer can both read `balance = 1000`, both decide `1000 >= 600`, both debit, leaving `-200`. This is the classic time-of-check / time-of-use (TOCTOU) bug; in a trading system it lets a customer overdraw or buy more than they own.

**Fix:** make the debit atomic with the check in a single SQL statement.

```sql
UPDATE customers
   SET balance = balance - :cost
 WHERE id = :customer_id
   AND balance >= :cost;            -- 0 rows updated == reject
```

Alternatively, take a row-level lock with `SELECT … FOR UPDATE` inside the transaction, or use an optimistic version column.

### Problem 5 — Sell path skips every safety check

The `sell` branch validates nothing: it does not check that the customer owns the gold they claim to sell, does not check that `quantity` is positive, does not check the price against the market. A customer can "sell" any quantity at any price and the function credits their balance unconditionally. This is, effectively, free money on demand.

**Fix:** every order path needs the same validation pipeline (quantity > 0 in valid increments, price > 0, price within tolerance of market) plus a side-specific inventory check for sell orders. My Part 2/3 module runs both sides through the same validator for this reason.

### Problem 6 — Customer not found crash + resource leak + no input validation

**Bug A — `fetchone()` returns `None`:** if `customer_id` does not exist, `cursor.fetchone()` returns `None`. The very next line `customer[0]` crashes immediately with `TypeError: 'NoneType' object is not subscriptable` instead of returning a structured error.

```python
customer = cursor.fetchone()
if customer is None:
    return {"status": "failed", "reason": "customer not found"}
```

**Bug B — Resource leak:** `conn` is never closed on any return path. Long-running services leak file handles until they can no longer open new connections. Fix is in Problem 3's `finally` block.

**Bug C — No input validation:** `customer_id`, `quantity`, `price`, and `order_type` are accepted at face value. Negative quantity, zero price, and unknown order type (e.g. `"BUY"` in uppercase) all pass silently. The unknown order type case is especially sharp: the function returns `None`, and any caller that treats the result as a dict will blow up immediately.

**Fix:** validate all inputs structurally before opening any DB connection and always return a consistent `{"status": "...", ...}` shape — never `None`.

## Honourable mentions (would also flag in review)

- `print(f"Order successful for {name} …")` leaks PII to stdout and is not a durable audit trail. Use a structured logger and a persistent order-log table.
- No idempotency key — a network retry submits the order twice.
- `"trading.db"` is hardcoded to the working directory; the path should be injected.
- Return shape is inconsistent across branches (`None` vs `{...}`); callers cannot handle the result reliably.
