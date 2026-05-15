# Part 4 — Explain Your Decisions (English version)

**Submitted by:** Pantakan Totae · `pantakan.totae@gmail.com`
**Date:** 2026-05-15
**Repo:** see `README.md` for layout and how to run.
**Thai version:** [`Part_4.md`](Part_4.md)


---

### 1. Trade-offs

I deliberately favoured **clarity and testability over generality**. The
biggest concrete trade-offs:

- **Dependency injection via interfaces, composed with Google Wire.**
  Each module accepts its ports through `New(...)` and **Wire** assembles
  the full graph at the composition root (`wire/wire.go`). Provider
  functions declare which adapter binds to which port; `wire` generates
  `wire_gen.go` containing the actual constructor code at `make
  generate` time. The cost is one extra codegen step in the workflow
  (and the generated file in the repo). The payoff is that the
  dependency graph is checked at **compile time** — forgetting to
  register a provider fails the build, not the startup, which is
  qualitatively better than reflection-based DI containers (uber-fx,
  dig). And because Wire is codegen, the production binary has no Wire
  runtime dependency once compiled: the build artifact is as clean as
  hand-written constructors. Adapters live next to the port they
  implement (`balance/memory.go`, `quote/memory.go`), so a production
  swap from `Memory` to `Postgres`/`Redis` is one provider-function
  change in `wire/providers.go` — the module's validation logic does
  not move.

- **`shopspring/decimal` over `float64`.** One external dependency in
  exchange for eliminating an entire class of silent money-precision
  bugs. For a system whose primary job is "tell me whether this order
  is acceptable, and quote the exact spread", using floats would be
  indefensible — the fact that the flawed code in Part 1 used floats was
  one of my top three concerns. The `decimal.RequireFromString("0.005")`
  pattern in `DefaultConfig` is intentional: I want exact constants, not
  binary-float approximations of them.

- **Collect every error vs. fail on the first one.** I chose to collect.
  A user submitting an order with a bad quantity *and* a stale price
  would rather see both at once than play whack-a-mole. Cost: `Result`
  carries a slice; tests have to look up by error code rather than just
  `==` on a single string. Worth it.

- **Stateless `Validator`, no internal mutex.** The validator does not
  apply orders, just judges them. Concurrency belongs to the caller (the
  order-execution service) where the actual balance mutation happens.
  Pushing locks into the validator would be premature — and in practice
  the per-customer lock that you actually need is in the execution
  layer, not in validation.

- **One module per business sub-domain (ports & adapters style).**
  The validator is split into four modules — `orderval` (order shape,
  Part 2 rules 1-3), `quote` (price freshness plus the Part 3A spread
  calculation, Part 2 rule 5), `balance` (the Part 2 rule-4 balance
  check for buy orders), and `limit` (Part 3B daily quota). Each module
  owns its ports, error codes, and Config; `service/validator` is the
  orchestrator that runs them in order and handles short-circuit logic
  (shape fail → return early, market unavailable → return early).
  The first draft was a monolithic `Validator` with every rule in one
  method; what made me refactor was the realisation that Part 3 had just
  *added* two new business concerns and Part 4 was about future
  evolution — that combination is a clear signal that more rules
  (KYC, AML, tier-specific limits) will keep landing. Splitting the
  rules into modules early means each future addition is a new module
  rather than an edit to a steadily growing file, which keeps test
  scope tight and refactor blast radius small. The cost is more
  folders and longer import paths, but a reviewer opening
  `domain/module/` sees the full set of business sub-domains at a
  glance — that's navigation, not noise. I would *not* split this way
  for a system whose rule set is known to be stable and whose team has
  no plan to peel off microservices later; the overhead doesn't pay back
  in that world.

- **`limit` module is optional.** `Service.New()` accepts a `nil` limit
  validator and skips the daily-limit branch entirely. That keeps Part 2
  testable in isolation (no `DailyLedger` mock needed) and lets a caller
  that doesn't care about per-day limits — an internal ops tool, say,
  that needs to override quota for VIP customers — opt out without
  rewriting the service. The cost is a single `nil` check in
  `Service.Validate()` before calling `limit.Apply()`; trivial.

### 2. Alternatives I considered

- **Float64 with careful rounding.** Rejected — see above. The cost of
  getting it subtly wrong is much higher than the cost of one
  dependency.
- **Integer satang (1 THB = 100 satang) instead of `decimal`.** This is
  a perfectly valid alternative with no external dependency and is what
  I'd use if dependency policy forbade `shopspring/decimal`. I picked
  `decimal` because it's idiomatic in the Go financial-services world
  and the resulting code reads more like the domain ("baht-weight",
  "0.5%") than satang-with-scaling-factor arithmetic.
- **Functional configuration (`func() decimal.Decimal` for price)
  instead of a `MarketPriceProvider` interface.** Lighter syntactically,
  but it loses the natural place to attach an `error` return for the
  fail-closed behavior I wanted on a missing market feed.
- **Monolithic `Validator` (one struct holding every rule) vs
  ports-and-adapters modules.** My first iteration had a single
  `Validator` that took all three ports in its constructor and ran the
  rules as helper methods (`checkOrderType`, `checkQuantity`,
  `checkBuy`, `checkSell`, …) inside one `Validate()` method. That
  shape was fine for the Part 2 rule set on its own — the pipeline
  ceremony of separate modules wouldn't have paid for itself. What
  tipped me over was Part 3: it adds two rules that are clearly their
  own sub-domains (spread is a pricing concern, daily quota is a
  rate-limiting concern), and Part 4 explicitly asks how this evolves
  at scale. Together those say: business is going to keep adding rules.
  Refactoring to one-module-per-sub-domain at that moment, while the
  surface area was still small, was much cheaper than doing it later
  when `validator.go` had grown to twenty rules. The refactor cost was
  roughly an hour (rewrite Part 2 tests against the new
  `service.Service`, rewire the Wire setup, write the orchestration
  logic in the service); the payoff is that the next new rule lands as
  a new module without touching the existing four.
- **Short-circuit on the first error (everywhere).** Tried this on
  paper and rejected it as worse UX. A user who submits an order with
  both a bad quantity *and* a stale price wants to see both at once,
  not fix one and resubmit only to discover the other. I kept
  short-circuit only between *phases* — shape fail → skip the market
  lookup (don't waste an upstream fetch on garbage input); market
  unavailable → skip balance and limit (can't validate without a
  reference price anyway) — but within a phase all rules run and
  collect errors into the same `Result`.

### 3. How I analysed the flawed code (Part 1)

My read order, roughly:

1. **Skim once for what it does.** Decide whether I'm reading buy logic
   or sell logic at each branch.
2. **Read it again with a security hat on.** Financial code → first
   thing I look for is SQL injection or unauthenticated mutation.
   String-built SQL was visible immediately, so SQL injection went to
   the top of the list.
3. **Read it again with a numeric-correctness hat on.** Money + Python
   floats is a yellow flag in any context; "balances" + Python floats is
   a red flag.
4. **Read it for failure modes.** No `try`/`except`, no rollback, no
   `conn.close()`, sell path with no validation, race conditions on the
   read-then-write pattern. These are the bugs that bite in production
   long after the original review.
5. **Rank by blast radius.** SQL injection (anyone can rewrite any
   balance) → silent slow corruption (float arithmetic) → race
   conditions on read-then-write → no exception handling + connection
   leak → sell-side missing validation → customer-not-found crash +
   input validation. That ranking is what I'd lead with in a real review
   conversation, because "fix all of these" overwhelms a junior author
   and "fix the one that lets attackers drain the company" is actionable.

### 4. Evolving to thousands of orders per minute

What I would **keep**:

- The dependency-injected interfaces — they are the single biggest
  enabler of horizontal change.
- The `Result` shape with stable error `Code`s — clients depend on these.
- Decimal arithmetic — gets *more* important at scale, not less.
- The "fail closed when the market price is unavailable" rule.

What I would **change**:

- **Move balance + ledger lookups into a single transactional read** in
  the order-execution service that calls the validator, so we don't
  validate against a balance that's already stale by the time we apply
  the order. The validator stays pure; the call site stitches it into a
  `BEGIN; SELECT FOR UPDATE; validate; UPDATE; COMMIT;` flow.
- **Replace `limit.Memory`** with a real store. Two reasonable
  shapes: a Redis sorted set keyed `customer:YYYY-MM-DD` with a TTL of
  48h, or a `daily_totals` row updated transactionally with the order.
  Whichever the team picks, the in-memory implementation goes away the
  day it ships.
- **Cache the market price** with a short TTL (~250–500 ms). At
  thousands of orders/minute, refetching per validate is wasteful and
  punishes the upstream feed. I would also add a "price age" field to
  `Result` so callers can trace bad fills back to a stale tick.
- **Add observability:** validation latency histogram, error-code
  counter, and a structured log line per invalid result with
  `customer_id`, `error_code`, request-trace ID. Without this you cannot
  tell whether a sudden spike in `STALE_OR_OFF_PRICE` is an upstream
  feed wobble or genuine market movement.
- **Idempotency keys** on the calling order-submission API to dedupe
  retries.
- **Per-customer rate limit** (orders/sec) at the API boundary, before
  validation.
- **Separate "cheap" pre-validation** (shape rules) into the API gateway
  so we can reject obvious garbage without burning a market lookup.
  Validator's heavier rules stay in the order service.
- **Property-based tests** in addition to the table-driven ones — to
  fuzz quantity/price boundaries.

**Service split: validator + price feed + execution (when horizontal
scale demands it).** At thousands of orders/min, refetching the market
price per validate and redeploying the whole world to ship a single fix
start to hurt. Splitting by responsibility pays off at that point.

Target layout:

```
core/                  ← pure logic, no I/O
  validator/           ← current package + interfaces
  order/               ← Order, Result, Error types
  ledger/              ← daily-limit math
common/                ← shared contract (proto / OpenAPI)
  domain/
services/
  validator-svc/       ← thin HTTP/gRPC wrapper over core/validator
  price-svc/           ← subscribes upstream feed, fans out via Redis
  execution-svc/       ← receives order, calls validator-svc, applies balance in tx
```

Price service + Redis backend:

- `price-svc` subscribes the upstream feed (Bloomberg / Reuters / a
  gold-hub API) and writes every tick:
  `SET market:gold:thb <price> EX 5` + `PUBLISH market:gold:thb:tick`.
- `validator-svc` reads with a `GET` (sub-millisecond) or maintains a
  local cache primed from the pub/sub.
- The current `MarketPriceProvider` interface lets us swap `StaticMarket`
  for a `RedisMarketAdapter` with **zero changes** to the validator
  itself — that is the payoff of the DI choice from §1.

Why this layout:

- `core/` stays testable as pure logic — unit tests never touch Redis
  or the network.
- `common/` carries the contract: a proto file or OpenAPI spec
  generates types for all three services, so changing the `Order`
  shape once propagates everywhere.
- `services/` are thin — each `main()` wires `core` to a transport and
  an observability stack and nothing else.

Minimum changes to enable:

1. Implement `RedisMarketAdapter` satisfying `MarketPriceProvider`.
2. Replace `limit.Memory` with a Redis sorted set (key
   `customer:YYYY-MM-DD`) or a transactional `daily_totals` row.
3. Wrap `Validator.Validate` in a gRPC handler inside `validator-svc`.
4. Validator core: zero lines changed.

When **not** to split: if throughput is well under ~100 orders/sec, the
team is small, and the observability stack is immature, three services
become overhead rather than benefit. A monolith with good interface
seams is still a valid production option — split when there is a
concrete signal that one of the boundaries is causing real pain.

### 5. Tools

I used **Claude (Anthropic)** during this assessment in two ways:

- **As a reasoning partner** — to enumerate edge cases I might have
  missed (e.g., "what if `expected` is zero in `withinTolerance`",
  "what if the customer's daily total has already exceeded the limit
  before this order"), and to pressure-test my Part 1 prioritisation
  ("am I sure SQL injection outranks the float-precision issue here?").
- **For first drafts of mechanical code** — most of the test
  scaffolding (table-driven cases, the `dec` helper) and the
  `cmd/seed/main.go` + `cmd/server/main.go` drivers were drafted with
  Claude's help. The `Validator.Validate` body and the orchestration
  in `service/validator/service.go` are mine.

I verified the output by:

- Running `go test ./... -v` and reading every failing test until they
  passed.
- Hand-walking every error code at least once and confirming the
  human-readable message reads the way I'd want to read it as a
  customer ("balance 500.00 THB is less than required 42210.00 THB", not
  "ERR_INSUFFICIENT").
- Re-reading the assessment requirements against my implementation,
  rule by rule, and ticking each off (order type, quantity positive +
  multiples of 0.5, positive price, sufficient balance for buys, ±2%
  freshness, 0.5% spread margin on buys, 5 baht-weight daily limit
  with remaining-allowance message). Every requirement maps to at least
  one named test.
- Reading the diff at the end with the same mindset I used for Part 1
  — float anywhere it shouldn't be, stringly-typed errors that should be
  enums, missing `nil` checks. None found in the final pass.
