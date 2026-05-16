# Part 4 — Explain Your Decisions (English version)

**Submitted by:** Pantakan Totae · `pantakan.totae@gmail.com`
**Date:** 2026-05-16
**Repo:** see `README.md` for layout and how to run.
**Thai version:** [`Part_4.md`](Part_4.md)


---

### 1. Trade-offs

I deliberately favoured **clarity and testability over generality**. The biggest concrete trade-offs:

- **Dependency injection via interfaces, composed with Google Wire.**
  Each module accepts its ports through `New(...)` and **Wire** assembles the full graph. This ensures clear separation between Infrastructure and Domain, making the system highly testable and maintainable.
- **Two-Phase Logic (Soft Check vs. Hard Check).**
  Validation is split into two stages: the **Validator** (read-only pre-flight) and the **Processor** (authoritative transactional). This balances high performance (filtering bad requests early) with strict consistency (locking and re-verifying inside a transaction).
- **`shopspring/decimal` over `float64`.**
  Eliminates money-precision bugs entirely. For a financial system handling gold weight and currency, exact decimal arithmetic is non-negotiable.
- **One module per business sub-domain (ports & adapters style).**
  Refactored from a monolith into four distinct modules (`orderval`, `quote`, `balance`, `limit`). This keeps business logic isolated, tests focused, and future expansion (e.g., adding KYC rules) clean.
- **Stateless `Validator`, Transactional `Processor`.**
  The Validator remains pure and fast, while the Processor orchestrates the database transaction lifecycle, handling locks and state mutations authoritatively.

### 2. Alternatives I considered

- **Float64 with careful rounding.** Rejected. Financial systems cannot tolerate silent failures or approximation errors. Specialist libraries are worth the dependency.
- **Integer satang instead of `decimal`.** A valid alternative for reducing dependencies, but `decimal` was chosen for readability. Code that reads like the domain ("0.5% spread") is easier to audit.
- **Short-circuit on the first error.** Rejected for poor UX. Users should see all validation errors at once (e.g., bad quantity AND stale price) rather than fixing them one by one.

### 3. How I analysed the flawed code (Part 1)

My process focused on impact:
1. **Security first:** Identified the string-built SQL as a critical SQL injection vulnerability.
2. **Numeric correctness:** Flagged the use of binary floats for financial balances.
3. **Failure modes:** Noted missing error handling, connection leaks, and the read-then-write race condition that could be exploited to drain funds.
4. **Ranking:** Categorized findings by "blast radius" to prioritize fixes that protect the company's core assets first.

### 4. Evolving to thousands of orders per minute

What I built as a foundation for scale:

- **Transaction Safety:** Real row-level locking (`SELECT FOR UPDATE`) ensures consistency for balance and limit checks under high concurrency.
- **Idempotency:** Native support via `Idempotency-Key` headers and database UNIQUE constraints in the `OrderRepository`.
- **Two-Phase Architecture:** Separates "cheap" read-only checks from "expensive" transactional execution to protect the database from junk load.
- **Dynamic Price Feed:** Port-based architecture allows plugging in real-time feeds (simulated via `DynamicPriceProvider`).
- **Observability:** Structured logging (`slog`) and full `context.Context` propagation for tracing and cancellation support.

**Future Roadmap:**
- **Market Price Caching:** Adding a Redis layer for the price feed with a short TTL to reduce upstream pressure.
- **Outbox Pattern:** Decoupling the execution path from downstream notifications using a transactional outbox.
- **Service Split:** Decoupling `validator-svc`, `price-svc`, and `execution-svc` into separate horizontal tiers when throughput exceeds monolith capacity.

### 5. Tools

I used **Claude (Anthropic) + Gemini CLI** as collaborators during this assessment:

- **Reasoning Partner:** Used to pressure-test the architectural refactor, enumerate complex edge cases (e.g., context cancellation during active transactions), and refine the priority ranking of vulnerabilities.
- **Drafting & Refactoring:** Accelerated the boilerplate generation for table-driven tests and assisted in the large-scale refactor from a monolithic validator to a ports-and-adapters service structure.
- **Verification:** Systematically verified every business rule against the implementation to ensure 100% requirement coverage.

**Verification Method:**
- **Comprehensive Test Suite:** Implemented unit and integration tests for all modules, including concurrency stress tests and boundary checks.
- **Human-Centric Error Messages:** Verified that all error codes return clear, actionable messages for the end-user.
- **Clean Build:** Enforced a strict zero-warning/zero-failure policy across the entire workspace.
