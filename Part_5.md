# Part 5 — รีวิว Pull Request `batch_processor.py` (ฉบับภาษาไทย)

**ผู้ส่ง:** Pantakan Totae · `pantakan.totae@gmail.com`
**วันที่:** 2026-05-15
**Repo:** ดู `README.md` สำหรับ layout และวิธีรัน
**English version:** [`Part_5_EN.md`](Part_5_EN.md)

---

> รีวิวเหมือน real PR — ระบุปัญหาเป็น line-level, จัด ranking, แยก block-merge กับ nit, และ flag สิ่งที่ดีด้วย

## Summary

เป็น MR ที่ดีครับ มีการแยก batch orchestration กับ per-order processing, return result แบบมี structure, มี summary helper — เป็น shape ที่ดีหมด **แต่ผมจะไม่ merge ตอนนี้** เพราะหลายปัญหาที่ Part 1 พยายามแก้ ก็ยังอยู่ใน PR นี้ (TOCTOU บน balance, floating precision / floating arimetric, sell ไม่มี inventory check) บวกกับปัญหาใหม่ที่เฉพาะ batch / concurrency model

ก่อน merge ต้องการให้แก้ทุกข้อใน **block** ด้านล่าง — ส่วน **nit** ผม comment inline แล้วให้ merge หลังจาก block ผ่านได้

## ที่ผมจะ approve

- **แยก function ดี:** `process_batch_orders` orchestrate, `process_single_order` จัดการ 1 order — test แยกง่าย
- **Return structured dict + stable `status`** (`filled` / `rejected` / `error`) — ดีกว่า `None` ของ legacy code เยอะ
- **`get_batch_summary`** เป็น aggregation helper สะอาด ไม่มี side effect
- **import `Decimal` ไว้แล้ว** — intent ถูก แค่ยังไม่ได้ใช้จริง
- **Docstring มีทุก function**
- **Single market price snapshot per batch** (`market_price` fetch ครั้งเดียวแล้วใช้ทั้ง batch) — pricing consistent ใน batch ซึ่งน่าจะเป็น semantic ที่ถูก แต่ขอให้ระบุใน docstring ด้วย

## Block (ต้องแก้ก่อน merge)

### B1. TOCTOU race บน balance — lock อยู่ผิดที่

```python
balance = customer_balances[customer_id]   # อ่าน OUTSIDE lock
…
if balance >= cost:
    with balance_lock:
        customer_balances[customer_id] = customer_balances[customer_id] - cost
```

- **ปัญหา:** 2 buy thread อ่าน `balance = 1000` พร้อมกัน, ทั้งคู่ผ่าน `balance >= cost = 600`, แล้ว debit ทีละตัว — สุดท้าย `-200` — lock protect แค่ 1 บรรทัด write ไม่ได้ protect read-check-write sequence
- **วิธีแก้:** ย้าย read + check + write ทุกอย่างเข้า `with balance_lock:` แล้ว re-read ภายใน lock

```python
with balance_lock:
    balance = customer_balances[customer_id]
    if balance < cost:
        return {"status": "rejected", "reason": "Insufficient balance"}
    customer_balances[customer_id] = balance - cost
    order_log.append({...})
```

- **อีกประเด็น:** `balance_lock` ตัวเดียว serialise ลูกค้า *ทั้งหมด* แม้ไม่เกี่ยวกัน — ที่ load จริงจะเป็น bottleneck — เปลี่ยนเป็น per-customer lock (`locks = defaultdict(threading.Lock); with locks[customer_id]:`) → ลูกค้าต่างคนทำ parallel ได้

### B2. Float arithmetic บน money

- **ปัญหา:** `Decimal` import แล้วไม่ใช้ — `customer_balances` เก็บ `500000.00` เป็น `float`, `cost = quantity * price` กับ `balance - cost` เป็น float operation — silent precision loss สะสม + rounding behavior ไม่อยู่ใน control เรา
- **วิธีแก้:** เปลี่ยน `customer_balances` value เป็น `Decimal` (หรือ satang เป็น int) + ทุก arithmetic เป็น `Decimal`

### B3. Sell path ไม่มี inventory / sanity check

```python
elif order_type == "sell":
    revenue = quantity * price
    with balance_lock:
        customer_balances[customer_id] = customer_balances[customer_id] + revenue
```

- **ปัญหา:** ลูกค้า "ขาย" quantity เท่าไหร่ ราคาเท่าไหร่ก็ได้ที่ผ่าน freshness check — ระบบ credit ให้เลย — ไม่เช็คว่ามีทองจริง, ไม่เช็ค quantity บวก
- **เหมือน Part 1 บั๊กเดิม:** เพิ่ม inventory ledger lookup + quantity validation

### B4. `customer_balances[customer_id]` ขึ้น `KeyError` กับ unknown customer

- **ปัญหา:** ไม่ check membership ก่อน access dict — unknown `customer_id` crash worker กลาง batch + order ก่อนหน้า apply ไปแล้วเงียบๆ
- **วิธีแก้:** explicit lookup `if customer_id not in customer_balances: return {"status": "rejected", ...}`

### B5. `order_log.append` อยู่นอก lock

- **ปัญหา:** `list.append` atomic ใต้ CPython GIL ปัจจุบัน แต่ (a) ไม่ควร assume สำหรับ financial audit log, (b) free-threaded Python (PEP 703) มาแน่, (c) `order_log` row ควร write *พร้อม* balance change ใน lock เดียว — ไม่งั้น order out of order ได้
- **วิธีแก้:** ย้าย `append` เข้า `with balance_lock:` block

### B6. Batch atomicity ไม่ define

- **ปัญหา:** order #3 ใน 5-order batch fail → #1, #2 apply ไปแล้ว, #4, #5 ยัง process ต่อ — เป็น behavior ที่ตั้งใจไหม? PR description ไม่บอก
- **วิธีแก้:** เลือก 1 ใน 2
  - **All-or-nothing:** process batch ใน transaction เดียว — reject อันใดอันนึง rollback ทั้งหมด
  - **Best-effort:** document ชัดเจนว่า batch best-effort + caller ต้อง reconcile จาก per-order result list
- **ตามด้วย test** สำหรับสิ่งที่เลือก — ถ้าไม่ define ชัด 2 caller จะ assume opposite semantics 1 คนผิด

### B7. `net_cost` calculation misleading

```python
"net_cost": total_spent - total_earned,
```

- **ปัญหา:** ลูกค้าซื้อ 1k + ขาย 0.5k → `total_spent = 1000`, `total_earned = 500`, `net_cost = 500` — math ถูก แต่ label misleading: คนอ่านเห็น `net_cost` คิดว่าเป็น "ที่ charge ลูกค้ารวม" ซึ่งคือ `total_spent`
- **วิธีแก้:** rename เป็น `net_cash_outflow` (หรือ `net_cash_change` แล้วกลับ sign) + document convention
- **อีกประเด็น:** dict ใช้ `cost` กับ `revenue` แยกกันตาม direction — fragile ถ้าวันหน้ามี hybrid order ที่ทั้งซื้อทั้งขาย พิจารณา normalise เป็น `{"direction": "in"/"out", "amount": …}` แทน

## Request changes (ทำดี แต่ไม่เร่งเท่า block)

- **C1. Defensive parse `order` dict.** `order["type"]`, `order["quantity"]`, `order["price"]` raise `KeyError` กับ malformed input — ใช้ `order.get(...)` + explicit error handling, หรือ pydantic / typed `Order` dataclass
- **C2. `get_market_price()` unmocked.** hardcode `42150.00` + ไม่มี error path — สำหรับ test (และ scenario upstream-feed-down จริง) ต้อง inject price source + handle `ErrPriceUnavailable`-style failure — วันนี้ function จะ return hardcoded price ต่อแม้ upstream แตก
- **C3. `order_log` in-memory.** process restart = audit trail หาย — financial system ต้อง durable (DB row, append-only file, message queue) + write inside transaction เดียวกับ balance change
- **C4. Hardcoded seed data ใน module scope.** `customer_balances` populate ตอน import — ดีสำหรับ dev แต่ห้าม ship จริง — guard ด้วย env flag อย่างน้อย
- **C5. 5% freshness vs 2% ใน spec.** Part 2 spec บอก 2%, PR นี้ใช้ 5% — มี separate batch-mode policy (กรุณา document) หรือเป็น regression? — สอบถามเจ้าของ trading-rules spec
- **C6. ไม่มี test ใน diff.** เพิ่มอย่างน้อย:
  - happy-path single buy in batch
  - mixed buy/sell batch
  - rejection mid-batch (cover atomicity decision จาก B6)
  - concurrent batch สำหรับ customer เดียวกัน (cover B1)

## Nits

- **N1.** `process_single_order` return 3 dict shape ที่ต่างกัน (`{"status": "filled", "cost": …}`, `{"status": "filled", "revenue": …}`, `{"status": "rejected", "reason": …}`) — พิจารณา shape เดียวที่มี optional field — caller ไม่ต้องจำว่า key ไหนอยู่ status ไหน
- **N2.** `customer_balances[customer_id] = customer_balances[customer_id] - cost` อ่านเป็น "x = x - cost" — `customer_balances[customer_id] -= cost` สั้นและเหมือนกัน
- **N3.** Magic number `0.05` — ดึงเป็น named constant
- **N4.** `f"Unknown order type: {order_type}"` UX ดี — ใช้แบบเดียวกันกับ rejection message อื่นด้วย (include bad value)

## ลำดับ approve ที่แนะนำ

1. แก้ B1, B2, B3, B4, B5, B6, B7 (correctness / safety)
2. เพิ่ม test ครอบคลุมอย่างน้อยที่ระบุใน C6
3. C / N ที่เหลือ ทำ follow-up PR ได้

ยินดี pair ใน per-customer-lock refactor (B1) — เป็นข้อที่เสี่ยง introduce บั๊กใหม่ถ้ารีบ
