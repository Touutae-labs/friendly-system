# Part 5 — Pull Request Review of `batch_processor.py` (English version)

**Submitted by:** Pantakan Totae · `pantakan.totae@gmail.com`
**Date:** 2026-05-15
**Repo:** see `README.md` for layout and how to run.
**Thai version:** [`Part_5.md`](Part_5.md)


---

> Reviewing as I would on a real PR: specific line-level concerns,
> ranked, with what I'd block-merge on vs. nit. I'll flag good things too.

### Summary

Direction is right — splitting batch orchestration from per-order
processing, returning structured results, and adding a summary helper
are all good shapes. **I would not merge this as-is**, though, because
several of the same problems we should be working hard to fix from
Part 1 are present here (TOCTOU on balance, float arithmetic on money,
missing inventory check on sells), plus a few new ones specific to the
batch and concurrency model.

Before merging I'd want changes on the items marked **block** below.
The items marked **nit** I'd leave as inline comments and merge once
the blocks are addressed.

### What I'd approve

- ✅ Good separation: `process_batch_orders` orchestrates,
  `process_single_order` handles one order. Easy to test individually.
- ✅ Returning structured result dicts with a stable `status` field
  (`filled` / `rejected` / `error`) — much better than the `None`
  return from the legacy code.
- ✅ `get_batch_summary` is a clean, side-effect-free aggregation helper.
- ✅ The intent to import `Decimal` is good; we just need to actually use it.
- ✅ Docstrings present on every function.
- ✅ A single market price snapshot per batch (`market_price` is fetched
  once and reused) gives consistent pricing within the batch — that's
  probably the right semantic; please call it out in the docstring.

### Block (must address before merge)

#### B1. TOCTOU race on balance — the lock is in the wrong place

```python
balance = customer_balances[customer_id]   # read OUTSIDE the lock
…
if balance >= cost:
    with balance_lock:
        customer_balances[customer_id] = customer_balances[customer_id] - cost
```

Two buy threads can both read `balance = 1000`, both pass
`balance >= cost = 600`, then sequentially debit, leaving `-200`. The
lock only protects the single write line, not the read-check-write
sequence. **Fix:** move the read, the check, and the write all inside
the `with balance_lock:` block, and re-read inside the lock:

```python
with balance_lock:
    balance = customer_balances[customer_id]
    if balance < cost:
        return {"status": "rejected", "reason": "Insufficient balance"}
    customer_balances[customer_id] = balance - cost
    order_log.append({...})
```

Also: a single global `balance_lock` serialises *all* customers, even
unrelated ones. At any real load that becomes the bottleneck. Switch to
a per-customer lock (e.g.,
`locks = defaultdict(threading.Lock); with locks[customer_id]:`) so two
different customers can be processed in parallel.

#### B2. Float arithmetic on money

`Decimal` is imported but unused. `customer_balances` holds `500000.00`
as `float`; `cost = quantity * price` and `balance - cost` are float
operations. Same issue as Part 1 — silent precision loss across many
trades, and the rounding behavior is not under our control. **Fix:**
make `customer_balances` values `Decimal` (or store satang as int) and
do all arithmetic in `Decimal`.

#### B3. Sell path has no inventory or sanity check

```python
elif order_type == "sell":
    revenue = quantity * price
    with balance_lock:
        customer_balances[customer_id] = customer_balances[customer_id] + revenue
```

A customer can "sell" any quantity at any price the freshness check
accepts and the system credits them. No check that they own the gold,
no check that quantity is positive. This is the same bug as the legacy
function — please add an inventory ledger lookup and quantity validation.

#### B4. `customer_balances[customer_id]` raises `KeyError` on unknown customer

There is no membership check before the dict access. An unknown
`customer_id` crashes the worker mid-batch and any prior orders in the
batch silently stay applied. **Fix:** explicit lookup with
`if customer_id not in customer_balances: return {"status": "rejected", ...}`.

#### B5. `order_log.append` outside the lock

`list.append` is atomic under CPython's GIL today, but (a) we shouldn't
assume that for a financial audit log, (b) free-threaded Python (PEP 703)
is on the horizon, and (c) the order_log row should be written *together*
with the balance change inside the same lock so they cannot get out of
order. Move the `append` inside the `with balance_lock:` block.

#### B6. Batch atomicity is undefined

If order #3 in a 5-order batch fails, orders #1 and #2 are already
applied and orders #4 and #5 still get processed. Is that the intended
behavior? The PR description doesn't say. Please pick one of:

- **All-or-nothing**: process the batch under a single transaction;
  any rejection rolls everything back.
- **Best-effort**: document explicitly that the batch is best-effort and
  the caller must reconcile from the per-order result list.

…and write a test for whichever you pick. Without an explicit decision,
two callers will assume opposite semantics and one of them will be
wrong.

#### B7. `net_cost` calculation is misleading

```python
"net_cost": total_spent - total_earned,
```

For a customer who buys 1k of gold and sells 0.5k, `total_spent = 1000`,
`total_earned = 500`, `net_cost = 500`. The math is fine, but the label
is misleading: a casual reader sees `net_cost` and thinks "this is what
we charged the customer overall", which is `total_spent`. Rename to
`net_cash_outflow` (or `net_cash_change` and flip the sign), and
document the convention. Also: the result dicts split `cost` and
`revenue` keys by direction, which is fragile — a future hybrid order
that's both a buy and a sell breaks the aggregation. Consider
normalising to `{"direction": "in"/"out", "amount": …}`.

### Request changes (worth doing, less critical than the blocks)

- **C1. Defensive parsing of the `order` dict.** `order["type"]`,
  `order["quantity"]`, `order["price"]` raise `KeyError` on malformed
  input. Use `order.get(...)` with explicit error handling, or pydantic
  / a typed `Order` dataclass.
- **C2. `get_market_price()` is unmocked.** It hardcodes `42150.00` and
  has no error path. For tests (and for real upstream-feed-down
  scenarios) we need to inject the price source and handle an
  `ErrPriceUnavailable`-style failure. Today the function will keep
  returning the same hardcoded price even if the upstream is broken.
- **C3. `order_log` is in-memory.** A process restart loses the audit
  trail. For a financial system the order log must be durable
  (database row, append-only file, or a message queue) and the write
  should be inside the same transaction as the balance change.
- **C4. Hardcoded seed data in module scope.** `customer_balances` is
  populated at import time. Fine for dev, but make sure this never
  ships to a real environment — at minimum guard it with an env flag.
- **C5. 5% freshness vs. 2% in the spec.** The Part 2 spec said 2%; this
  PR uses 5%. Either there's a separate batch-mode policy (please
  document) or this is a regression. Please reconcile with whoever owns
  the trading-rules spec.
- **C6. No tests in the diff.** Please add at least:
  - happy-path single buy in a batch
  - mixed buy/sell batch
  - rejection mid-batch (covers your atomicity decision from B6)
  - concurrent batches for the same customer (covers B1)

### Nits

- **N1.** `process_single_order` returns three different dict shapes
  (`{"status": "filled", "cost": …}`, `{"status": "filled", "revenue":
  …}`, `{"status": "rejected", "reason": …}`). Consider a single shape
  with optional fields, so callers don't have to remember which keys
  exist for which `status`.
- **N2.** `customer_balances[customer_id] = customer_balances[customer_id] - cost`
  reads as "x = x - cost"; `customer_balances[customer_id] -= cost` is
  shorter and identical.
- **N3.** Magic number `0.05` — pull into a named constant.
- **N4.** `f"Unknown order type: {order_type}"` is great error UX; do
  the same in the rejection messages (include the bad value).

### Suggested approval flow

1. Resolve B1, B2, B3, B4, B5, B6, B7. (These are correctness / safety.)
2. Add tests covering at least the items in C6.
3. The remaining `C` and `N` items can land in a follow-up PR.

Happy to pair on the per-customer-lock refactor (B1) — that's the one
that's most likely to introduce its own bugs if rushed.
