# Part 5 — Pull Request Review of `batch_processor.py` (English version)

**Submitted by:** Pantakan Totae · `pantakan.totae@gmail.com`
**Date:** 2026-05-16
**Repo:** see `README.md` for layout and how to run.
**Thai version:** [`Part_5.md`](Part_5.md)

---

> Reviewing as I would on a real PR: line-level findings, ranked, with blocks vs. nits called out separately. I'll flag the good things too.

### Summary

Direction is right — separating batch orchestration from per-order processing, returning structured results, and providing a summary helper are all good shapes. **I would not merge this as-is**, because several of the same problems we should be fixing from Part 1 are still present here (TOCTOU on balance, float arithmetic on money, missing inventory check on sells), plus new issues specific to the batch and concurrency model.

All **block** items below must be resolved before merge. **Nit** items I'd leave as inline comments and clear once the blocks are done.

---

### What I'd approve

- ✅ Good separation: `process_batch_orders` orchestrates, `process_single_order` handles one order — easy to test individually.
- ✅ Structured result dicts with a stable `status` field (`filled` / `rejected` / `error`) — much better than the `None` return from the legacy code.
- ✅ `get_batch_summary` is a clean, side-effect-free aggregation helper.
- ✅ `Decimal` is imported — the intent is correct, we just need to actually use it.
- ✅ Docstrings on every function.
- ✅ Single market price snapshot per batch — consistent pricing within the batch is probably the right semantic; please call it out explicitly in the docstring.

---

### Block (must fix before merge)

#### B1. TOCTOU race on balance — the lock is in the wrong place

```python
balance = customer_balances[customer_id]   # read OUTSIDE the lock
…
if balance >= cost:
    with balance_lock:
        customer_balances[customer_id] = customer_balances[customer_id] - cost
```

Two buy threads can both read `balance = 1000`, both pass `balance >= 600`, then debit sequentially, leaving `-200`. The lock only protects the single write line — not the read-check-write sequence.

**Fix:** move the read, check, and write all inside the lock:

```python
with balance_lock:
    balance = customer_balances[customer_id]
    if balance < cost:
        return {"status": "rejected", "reason": "Insufficient balance"}
    customer_balances[customer_id] = balance - cost
    order_log.append({...})
```

**Also:** one global `balance_lock` serialises *all* customers, even unrelated ones — a bottleneck at any real load. Switch to per-customer locks (`locks = defaultdict(threading.Lock); with locks[customer_id]:`) so different customers can run in parallel.

#### B2. Float arithmetic on money

`Decimal` is imported but unused. `customer_balances` stores values as `float`; `cost = quantity * price` and `balance - cost` are float operations. Same issue as Part 1 — silent precision loss across many trades. **Fix:** store balances as `Decimal` (or integer satang) and do all arithmetic in `Decimal`.

#### B3. Sell path has no inventory or sanity check

```python
elif order_type == "sell":
    revenue = quantity * price
    with balance_lock:
        customer_balances[customer_id] = customer_balances[customer_id] + revenue
```

A customer can "sell" any quantity at any price that passes the freshness check and the system credits them unconditionally. No check that they own the gold, no check that quantity is positive. **Fix:** add an inventory ledger lookup and quantity validation on the sell path — same as we should have done in Part 1.

#### B4. `customer_balances[customer_id]` raises `KeyError` on unknown customer

No membership check before the dict access. An unknown `customer_id` crashes the worker mid-batch, and any orders already applied in that batch silently stay committed. **Fix:** `if customer_id not in customer_balances: return {"status": "rejected", ...}`.

#### B5. `order_log.append` outside the lock

`list.append` is atomic under CPython's GIL today, but (a) financial audit logs shouldn't depend on that assumption, (b) free-threaded Python (PEP 703) removes the GIL, and (c) the log entry should be written *atomically with* the balance change so they cannot get out of order. **Fix:** move the `append` inside the `with balance_lock:` block.

#### B6. Batch atomicity is undefined

If order #3 of 5 fails, orders #1 and #2 are already applied and #4 and #5 continue processing. Is that intentional? The PR doesn't say. **Pick one:**

- **All-or-nothing:** process the batch under a single transaction; any rejection rolls everything back.
- **Best-effort:** document this explicitly so callers know they must reconcile from the per-order result list.

Then write a test for whichever you choose. Without a decision, two callers will assume opposite semantics and one will be wrong.

#### B7. `net_cost` label is misleading

```python
"net_cost": total_spent - total_earned,
```

For a customer who buys 1k and sells 0.5k, `net_cost = 500`. The math is correct, but a reader sees `net_cost` and expects "total charged to the customer", which is `total_spent`. **Fix:** rename to `net_cash_outflow` (or `net_cash_change` with a flipped sign) and document the convention.

---

### Request changes (worth doing, less urgent than blocks)

- **C1. Defensive `order` dict parsing.** `order["type"]`, `order["quantity"]`, `order["price"]` all raise `KeyError` on malformed input. Use `order.get(...)` with explicit error handling, or a pydantic / typed `Order` dataclass.
- **C2. `get_market_price()` is hardcoded and has no error path.** For tests and for real upstream-feed-down scenarios, inject the price source and handle a feed-unavailable failure. Today it will return the hardcoded price even if the upstream is broken.
- **C3. `order_log` is in-memory.** A process restart loses the entire audit trail. For a financial system the log must be durable (DB row, append-only file, or message queue) and written inside the same transaction as the balance change.
- **C4. Hardcoded seed data in module scope.** `customer_balances` is populated at import time — fine for dev, dangerous in production. Guard it with an env flag at minimum.
- **C5. 5% freshness threshold vs. 2% in the spec.** Part 2 specifies 2%. Either there is a separate batch-mode policy (please document) or this is a regression. Please reconcile with whoever owns the trading-rules spec.
- **C6. No tests in the diff.** Add at minimum:
  - Happy-path single buy in a batch.
  - Mixed buy/sell batch.
  - Rejection mid-batch (covers the atomicity decision from B6).
  - Concurrent batches for the same customer (covers B1).

---

### Nits

- **N1.** `process_single_order` returns three different dict shapes depending on outcome. Consider a single shape with optional fields so callers don't have to remember which keys exist for which `status`.
- **N2.** `customer_balances[customer_id] = customer_balances[customer_id] - cost` → `customer_balances[customer_id] -= cost`. Identical, shorter.
- **N3.** Magic number `0.05` — extract to a named constant.
- **N4.** `f"Unknown order type: {order_type}"` is good error UX — apply the same pattern to the other rejection messages (include the bad value).

---

### Suggested approval flow

1. Resolve B1–B7 (correctness and safety).
2. Add tests covering at least the items in C6.
3. Remaining C and N items can land in a follow-up PR.

Happy to pair on the per-customer-lock refactor (B1) — that's the one most likely to introduce its own bug if rushed.
