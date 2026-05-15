# Part 4 — เหตุผลเบื้องหลังการตัดสินใจ (ฉบับภาษาไทย)

**ผู้ส่ง:** Pantakan Totae · `pantakan.totae@gmail.com`
**วันที่:** 2026-05-15
**Repo:** ดู `README.md` สำหรับ layout และวิธีรัน
**English version:** [`Part_4_EN.md`](Part_4_EN.md)

---

## 1. Trade-offs ที่เลือก

ผมเลือก **ความชัดเจน + ความเทสง่าย มากกว่าความ general** อย่างจงใจ trade-off หลักๆ มีดังนี้

### Dependency injection ผ่าน interface + Google Wire สำหรับ composition

- **ที่เลือก:** แต่ละ module รับ port (interface) ตอน construct ผ่าน `New(...)` แล้วใช้ **Google Wire** เป็น composition root ที่ `wire/wire.go` — provider function ระบุว่า port ไหนผูกกับ adapter ตัวไหน + config ตัวไหน Wire จะ generate `wire_gen.go` เป็น constructor code ตอน `make generate`
- **ราคาที่จ่าย:** เพิ่ม codegen step ใน dev workflow (`make generate` หลังเพิ่ม/ลบ dependency) + reviewer ต้องอ่าน `wire_gen.go` เพื่อดู topology ของ dependency tree ที่ runtime
- **ที่ได้ ที่สำคัญที่สุด:** Wire ตรวจ DI graph ตอน **compile time** — ลืม register provider ของ port ไหน Wire แจ้งทันที ไม่ต้องรอ panic ตอน startup เหมือน reflection-based DI (uber-fx, dig) นอกจากนี้ test ใช้ memory adapter ที่อยู่ใน module เดียวกัน (`balance.NewMemory(...)`, `quote.NewMemory(...)`) production swap เป็น Redis/Postgres adapter โดยเปลี่ยน provider function ตัวเดียว — validator logic ใน module ไม่ต้องแตะ
- **อีกประเด็น:** Wire เป็น codegen tool ไม่ใช่ runtime framework — production binary ไม่มี Wire dependency หลังจาก compile แล้ว build artifact สะอาดเท่า manual constructor

### `shopspring/decimal` แทน `float64`

- **ที่เลือก:** ใช้ decimal library
- **ราคาที่จ่าย:** เพิ่ม external dependency 1 ตัว
- **ที่ได้:** กำจัดบั๊ก money-precision หรือ flaoting precision, shopspring เป็น standard ที่จัดการเรื่องนี้ได้ดีมาก แม้แต่บริษัทที่ขาย retail ที่ผมเคยทำงานก็ใช้
- **หมายเหตุ:** ใช้ `decimal.RequireFromString("0.005")` ใน `DefaultConfig` จงใจ — อยากได้ exact constant ไม่ใช่ binary-float approximation

### Collect ทุก error vs short-circuit ที่ตัวแรก

- **ที่เลือก:** Collect ทุกอัน
- **ทำไม:** user ที่ submit order quantity ผิด *และ* price stale ด้วย เห็นทั้งสองพร้อมกันดีกว่ามาทีละอัน (whack-a-mole UX)
- **ราคาที่จ่าย:** `Result` มี slice, test ต้อง look up by error code แทน `==` กับ string เดียว — รับได้

### Stateless `Validator`, ไม่มี internal mutex

- **ที่เลือก:** validator แค่ตัดสิน ไม่ apply order, ไม่ mutate balance
- **ทำไม:** concurrency เป็นเรื่องของ caller (order-execution service) ตรงที่ apply balance จริง — push lock เข้า validator คือ premature
- **อีกประเด็น:** per-customer lock ที่ต้องใช้จริงๆ มันอยู่ใน execution layer ไม่ใช่ใน validation

### 1 module = 1 business sub-domain (ports & adapters style)

- **ที่เลือก:** แยก business rule ออกเป็น 4 module ตาม sub-domain ของ assessment — `orderval` (shape ของ order, Part 2 rules 1-3), `quote` (price freshness + spread calc, Part 2 rule 5 รวม Part 3A), `balance` (balance check สำหรับ buy, Part 2 rule 4), `limit` (daily trading quota, Part 3B) แต่ละ module เป็นเจ้าของ port ของตัวเอง + error codes + Config service เป็นแค่ orchestrator ที่ร้อย module เรียงตามลำดับ
- **ทำไม:** ตอนแรกเขียนเป็น Validator monolithic ตัวเดียวที่รับทุก port — แต่พอเห็น Part 3 "เพิ่ม" rule ใหม่ 2 ตัว (spread กับ daily limit) ก็เริ่ม signal ว่า business ของ InterGold ยังจะมี rule ใหม่เข้ามาเรื่อยๆ ในอนาคต (KYC tier, AML, per-customer-tier limit ฯลฯ) ถ้าคงเป็น monolithic ไว้ เพิ่ม rule ที่ 7 ที่ 8 จะแก้ file เดิมที่ใหญ่ขึ้นเรื่อยๆ test scope กว้างขึ้น blast radius ของทุก refactor ก็เพิ่ม — เพิ่มผ่าน module ใหม่ทุกอย่างมัน scoped ใหม่หมด ของเดิมไม่ต้องแตะเลย
- **ราคาที่จ่าย:** folder เยอะขึ้น (4 modules + service + adapters แยก folder) import path ยาวขึ้น (`domain/module/quote` แทนที่จะเป็น `validator` ตัวเดียว) — แต่ reviewer เปิด `domain/module/` ก็เห็นภาพรวม business sub-domain ทันทีว่ามีอะไรบ้าง — เป็น navigation aid ไม่ใช่ overhead
- **ที่ได้ ที่จับต้องได้:**
  - test แต่ละ module pure ตามที่มันต้องการ — `orderval` ไม่ต้องมี mock เลย, `quote` mock แค่ `MarketPriceProvider` เท่านั้น — ไม่ต้องลากทั้ง stack มา test rule เดียว
  - swap adapter ของ module ใดได้โดยไม่กระทบ module อื่น — `adapter/market/static.go` → `adapter/market/redis.go` กระทบแค่ `quote`
  - ถ้าวันหน้า `limit` ต้อง durable counter (Redis sorted set) แล้วโตเป็น service ของตัวเอง — boundary มันชัดอยู่แล้ว split ออกได้โดยไม่ต้องคุ้ย code
- **Service orchestrator มี short-circuit logic ที่ไม่ใช่ของ module ใด module หนึ่ง:** shape fail → return ก่อน fetch market (เปลือง upstream feed), market unavailable → return ก่อน hit balance/ledger (fail closed) — pattern แบบนี้ถ้าฝัง logic เข้าไปใน module จะ blur boundary ปล่อยอยู่ใน service ที่รู้ context ของ flow ทั้งหมดดีกว่า
- **เมื่อไรไม่ต้อง split แบบนี้:** rule set รู้แน่ว่านิ่งตลอด lifetime + ทีมเล็ก + ไม่มีแผน split microservice — overhead ของ 4 folder ไม่คุ้ม กลับไปใช้ Validator monolithic ตัวเดียวอ่านง่ายกว่า

### `limit` module เป็น optional

- **ที่เลือก:** `Service.New()` รับ `*limit.Validator` ที่อาจเป็น `nil` ได้ → daily-limit check ถูก skip ทั้ง branch
- **ทำไม:** Part 2 ต้อง testable แบบ pure โดยไม่พึ่ง Part 3 — Service ที่สร้างด้วย `limit = nil` ผ่าน Part 2 ทุกข้อโดยไม่ต้อง mock DailyLedger เพิ่ม นอกจากนี้ caller ที่ไม่สนใจ daily limit (เช่น internal ops tool ที่ override ข้าม quota ของลูกค้า VIP) opt out ได้โดยไม่ต้องเขียน validator ตัวที่สอง — กลับมาเปิดเปลี่ยน config ของ service ตัวเดียวก็พอ
- **ราคาที่จ่าย:** มี nil check ใน service.Validate() ก่อนเรียก limit.Apply() — แลกแล้วคุ้ม

## 2. ทางเลือกอื่นๆ ที่พิจารณา

### Float64 + careful rounding

- **ที่ปฏิเสธ:** ผิดเงียบๆ มันแย่กว่า depend on library 1 ตัวเยอะมาก

### Integer satang (1 บาท = 100 satang) แทน decimal library

- **ที่พิจารณา:** valid alternative, ไม่มี dependency, จะใช้ถ้า dependency policy ห้าม `shopspring/decimal`
- **ที่เลือก decimal เพราะ:** idiomatic ใน Go financial-services ecosystem, อ่านโค้ดเหมือน domain ("baht-weight", "0.5%") มากกว่า "satang × scaling factor"

### Functional config (`func() decimal.Decimal` แทน interface)

- **ที่พิจารณา:** เบากว่าเชิง syntax
- **ที่ปฏิเสธ:** เสียที่ที่จะใส่ `error` return — fail-closed บน missing market feed ทำไม่ได้ clean

### Monolithic Validator (1 struct ทุก rule) vs ports-and-adapters modules

- **ที่พิจารณา + ที่ลองเขียนช่วงแรก:** ทุก rule อยู่ใน `Validator` ตัวเดียวที่รับทุก port (`AccountRepository` + `MarketPriceProvider` + `DailyLedger`) ใน constructor แล้วทำ rule sequence เป็น helper methods (`checkOrderType`, `checkQuantity`, `checkBuy`, `checkSell` ฯลฯ) ใน `Validate()` method เดียว — เริ่มเขียนแบบนี้จริงๆ ใน iteration แรก เพราะ rule set ของ Part 2 มันเล็กพอที่ pipeline ceremony ยังไม่คุ้ม
- **ที่เปลี่ยนเพราะอะไร:** พอเข้า Part 3 เห็นเลยว่า assessment มัน "เพิ่ม" rule ใหม่ 2 ตัวที่เป็น sub-domain แยก (spread คือ pricing concern, daily limit คือ quota concern) — เป็น preview ของ business future: จะมี KYC, AML, tier limit, market-specific rule เข้ามาเรื่อยๆ ถ้าคง monolithic ต่อไป file `validator.go` จะโตเรื่อยๆ จนถึงจุดที่ refactor แพงเกินกว่าจะทำ — refactor ตอนยังเล็กถูกที่สุด เลยตัด split ตอนนี้เลย
- **ราคาของการ refactor ในตอนนั้น:** rewrite test ของ Part 2 ทั้งหมดให้ใช้ service ที่ compose modules + update Wire setup + เขียน orchestration logic ใน service — ใช้เวลาเพิ่มประมาณ 1 ชม แลกกับ scalability ที่ business จะเริ่มเห็นใน iteration ถัดๆ ไป
- **เมื่อไรเดิม monolithic จะดีกว่า:** ถ้ารู้แน่ว่า rule set จะนิ่งตลอด lifetime + ทีมเล็กไม่ต้อง parallelise การพัฒนา + ไม่มีแผน split microservice — overhead ของ folder/wire setup กับ orchestration logic ไม่คุ้ม monolithic อ่านจบใน 1 file ก็พอ

### Short-circuit ที่ error แรก (full short-circuit)

- **ที่ลอง:** บนกระดาษ — Validate รัน rule ทีละตัว เจอ error แรกก็ return ทันที
- **ที่ปฏิเสธ:** UX แย่กว่า — user submit order ที่ quantity ผิด *และ* price stale พร้อมกัน ถ้า short-circuit ที่ตัวแรกจะเห็น error ทีละอันแก้ทีละครั้ง (whack-a-mole) เก็บ short-circuit ไว้แค่ระหว่าง *phase* แทน: shape ผิด → ไม่ถาม DB (เปลือง lookup) market ล่ม → ไม่ต้อง check balance/limit (validate ไม่ได้อยู่แล้ว) — ภายใน phase เดียว collect ทุก error ที่เจอ

## 3. กระบวนการวิเคราะห์ Part 1 (debugging process)

ลำดับการอ่านโค้ด ประมาณนี้

1. **อ่านรอบแรก เพื่อเข้าใจว่ามันทำอะไร** — ดูว่า branch ไหน buy / sell
2. **อ่านรอบสอง สวมหมวก security** — financial code → สิ่งแรกที่หาคือ SQL injection หรือ unauthenticated mutation — string-built SQL เห็นได้ทันที, SQL injection ขึ้น top ของ list
3. **อ่านรอบสาม สวมหมวก numeric correctness** — money + Python float = yellow flag, "balances" + Python float = red flag
4. **อ่านรอบสี่ failure modes** — ไม่มี try/except, ไม่มี rollback, ไม่มี conn.close(), sell path ไม่ validate, race condition จาก read-then-write — บั๊กแบบนี้กัดใน production หลังจาก review นานๆ
5. **rank by blast radius** — SQL injection (rewrite ทุก balance) → silent slow corruption (float arithmetic) → race condition จาก read-then-write → no exception handling + connection leak → sell-side missing validation → customer-not-found crash + input validation — ranking นี้คือสิ่งที่ผมจะ lead ใน real review conversation เพราะ "fix ทุกข้อ" จะ overwhelm junior author แต่ "fix ข้อที่ทำให้ attacker drain เงินบริษัทได้" actionable

## 4. ถ้าโตเป็น production service ระดับ thousands of orders/min

### สิ่งที่ผมจะ keep

- Dependency-injected interfaces — single biggest enabler ของการเปลี่ยน horizontally
- `Result` shape + stable error `Code` constants — client depend on นี้
- Decimal arithmetic — สำคัญ *กว่า* ที่ scale ใหญ่ ไม่ใช่น้อยลง
- "Fail closed when market price unavailable" rule
- Hardening pieces ที่เพิ่งทำ: config validation, sanity ceilings, customer_id scrubbing — ทั้งหมดยิ่งสำคัญที่ scale

### สิ่งที่ผมจะ change

- **ย้าย balance + ledger lookup เป็น single transactional read** ใน order-execution service ที่ call validator — เพื่อไม่ validate กับ balance ที่ stale ตอน apply — validator คงเดิมแบบ pure, call site แค่ stitch เข้า `BEGIN; SELECT FOR UPDATE; validate; UPDATE; COMMIT;` flow
- **เปลี่ยน `limit.Memory` เป็น real store** — 2 shape ที่เป็นไปได้: (1) Redis sorted set key `customer:YYYY-MM-DD` TTL 48h, (2) `daily_totals` row update transactional กับ order — ทีมเลือกอย่างไรก็ได้ in-memory implementation หายไปวันที่ ship
- **Cache market price** ด้วย short TTL (~250–500 ms) — ที่ thousands/min, refetch ต่อ validate มันสิ้นเปลือง + รังแก upstream feed — เพิ่ม "price age" field ใน `Result` ด้วย caller จะ trace bad fill กลับ tick stale ได้
- **เพิ่ม observability:** validation latency histogram, error-code counter, structured log line per invalid result พร้อม `customer_id`, `error_code`, request-trace ID — ไม่มีอันนี้บอกไม่ได้ว่า spike ของ `STALE_OR_OFF_PRICE` คือ upstream feed wobble หรือ market move จริง
- **Idempotency keys** บน calling order-submission API เพื่อ dedupe retry
- **Per-customer rate limit** (orders/sec) ที่ API boundary ก่อน validation
- **Split "cheap" pre-validation** (shape rules) ไป API gateway → reject garbage โดยไม่เผา market lookup, validator's heavier rules อยู่ใน order service
- **Property-based tests** เพิ่มจาก table-driven — fuzz quantity/price boundaries

### Service split: validator + price feed + execution (เมื่อ scale มาถึงจุดที่ต้องแยก)

ที่ thousands/min การ refetch market price ทุก validation + redeploy ทุกอย่างเพื่อแก้ feature เดียว เริ่มเจ็บ — แยก service ตาม responsibility คุ้มที่จุดนี้

**Layout ที่จะ aim:**

```
core/                  ← pure logic, ไม่มี I/O
  validator/           ← package ปัจจุบัน + interfaces
  order/               ← Order, Result, Error types
  ledger/              ← daily-limit math
common/                ← shared contract (proto / OpenAPI gen)
  domain/
services/
  validator-svc/       ← thin HTTP/gRPC wrapper รอบ core/validator
  price-svc/           ← subscribe upstream feed → fan out via Redis
  execution-svc/       ← รับ order → call validator-svc → apply balance ใน tx
```

**Price service + Redis backend:**

- `price-svc` subscribe upstream feed (Bloomberg / Reuters / gold-hub API) — เขียนทุก tick: `SET market:gold:thb <price> EX 5` + `PUBLISH market:gold:thb:tick`
- `validator-svc` ดึงผ่าน `GET market:gold:thb` (sub-ms) หรือ maintain local cache prime จาก pub/sub
- `MarketPriceProvider` interface ปัจจุบัน swap implementation จาก `StaticMarket` → `RedisMarketAdapter` ได้ตรงๆ — **validator core code = 0 lines เปลี่ยน** ซึ่งคือ payoff ของ DI ที่อธิบายในข้อ 1

**ทำไม layout นี้:**

- `core/` ทดสอบเป็น pure logic ได้ — unit test ไม่แตะ Redis / network
- `common/` เป็น contract กลาง — proto/OpenAPI generate ให้ทั้ง 3 service เปลี่ยน `Order` shape ครั้งเดียว downstream ทุกตัวตามทันที
- `services/` บางๆ — `main()` แค่ wire `core` เข้า transport + observability stack

**Minimum changes เพื่อ enable:**

1. Implement `RedisMarketAdapter` ที่ satisfy `MarketPriceProvider`
2. ย้าย `limit.Memory` → Redis sorted set (key `customer:YYYY-MM-DD`) หรือ Postgres `daily_totals` row update transactional กับ order
3. Wrap `Validator.Validate` ด้วย gRPC handler ใน `validator-svc`
4. Validator core: 0 lines เปลี่ยน

**เมื่อไรไม่ควร split:**

ถ้า throughput ยังไม่ถึง ~100 orders/sec, ทีมยังเล็ก, observability stack ยังไม่พร้อม → 3 service = ภาระ มากกว่าประโยชน์ Monolith ที่ design interface ดีๆ ยังเป็น production-ready option ที่ valid — split เมื่อมี concrete signal ว่า boundary ใด boundary หนึ่งเริ่มเจ็บจริง

## 5. Tools ที่ใช้

ผมใช้ **Claude (Anthropic)** ใน assessment นี้ 2 แบบ

- **เป็น reasoning partner** — ช่วย enumerate edge case ที่อาจมองข้าม (เช่น "ถ้า `expected` เป็น 0 ใน `withinTolerance` ล่ะ", "ถ้า customer's daily total เกิน limit แล้วก่อน order นี้มาล่ะ") + pressure-test การจัด ranking ใน Part 1 ("แน่ใจไหมว่า SQL injection ranks กว่า float precision?")
- **draft mechanical code ตัวแรก** — test scaffolding (table-driven cases, helper `dec`) กับ `cmd/demo/main.go` driver ส่วนใหญ่ draft ด้วย Claude — body ของ `Validator.Validate` เป็นของผม

วิธีที่ผม verify output

- รัน `make ci` (= tidy + fmt-check + vet + test) และ `make test-race` — อ่าน failing test จนผ่าน
- Hand-walk ทุก error code อย่างน้อย 1 ครั้ง — ยืนยันว่า human-readable message อ่านแล้วเข้าใจในมุมลูกค้า ("balance 500.00 THB is less than required 42210.00 THB" ไม่ใช่ "ERR_INSUFFICIENT")
- อ่าน assessment requirement ทีละ rule เทียบกับ implementation — tick ทุกอัน (order type, quantity positive + multiples of 0.5, positive price, sufficient balance for buys, ±2% freshness, 0.5% spread margin on buys, 5 baht-weight daily limit + remaining-allowance message) — requirement ทุกข้อ map ไป test ที่ตั้งชื่อไว้อย่างน้อย 1 อัน
- อ่าน diff รอบสุดท้ายด้วย mindset เดียวกับ Part 1 — float ที่อยู่ผิดที่, error stringly-typed ที่ควรเป็น const, missing nil check — ไม่เจอใน final pass
