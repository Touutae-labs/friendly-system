# Part 4 — เหตุผลเบื้องหลังการตัดสินใจ (ฉบับภาษาไทย)

**ผู้ส่ง:** Pantakan Totae · `pantakan.totae@gmail.com`
**วันที่:** 2026-05-16
**Repo:** ดู `README.md` สำหรับ layout และวิธีรัน
**English version:** [`Part_4_EN.md`](Part_4_EN.md)

---

## 1. Trade-offs ที่เลือก

ผมเลือก **ความชัดเจน + ความเทสง่าย มากกว่าความ general** อย่างจงใจ trade-off หลักๆ มีดังนี้

### Dependency injection ผ่าน interface + Google Wire สำหรับ composition

ใช้ dependency injection ใน composition pattern พวก idiomatic go ให้เหมาะสมเช่นการ inject interface เพื่อให้เทสง่ายขึ้น รวมถึงการลดที่ให้การประกาศแล้ว inject ใน main.go ตอน run service ด้วย
- **ทำไม:** ถูก pattern และโค๊ดอ่านง่ายกว่า
- **ราคาที่จ่าย:** ต้องเรียนรู้ wire เพิ่ม

### `shopspring/decimal` แทน `float64`

แก้ floating precision
- **ทำไม:** เพราะมันดีกว่าทุกอย่างในแง่ business ที่เกี่ยวกับเงิน
- **ราคาที่จ่าย:** ต้อง convert เป็น shopspring type ไปมา แล้วต้องใช้ lib เพิ่ม

### Collect ทุก error vs short-circuit ที่ตัวแรก

รวม error ทั้งหมดไว้ Audit 

### Stateless `Validator`, ไม่มี internal mutex

ตอนจะ lock ไม่ล้อคไม่ควรอยู่ใน preflight ควรอยู่ในตอนทำ transaction
- **ทำไม:** ทำไม ? เร็วกว่า ตรงกว่าไม่เปลือง resource DB
- **ราคาที่จ่าย:** ข้อเสีย ? ต้องไปจัดการใน transaction ตอนทำ transaction จริงๆ (ซึ่งถูกแล้ว)

### 1 module = 1 business sub-domain (ports & adapters style)

- **ทำไม:** โค๊ดอ่านง่ายกว่า แก้ง่ายกว่า มีการแบ่ง business ชัดเจนให้ player ที่เป็นคน หรือ high-level อ่านว่าโค๊ดแต่ละส่วนทำงานยังไงได้มากขึ้น มีการแบ่ง service -> Orchestator / Module -> Business of each module /Repository -> Persistance layer
- **ราคาที่จ่าย:** ไฟล์กับโฟลเดอร์จะเยอะนิดนึง แต่โค๊ดโดยรวมจะเป็นสัดส่วนแบ่งง่ายขึ้น

### `limit` module เป็น optional

`Service.New()` รับ `*limit.Validator` ที่อาจเป็น `nil` ได้ → daily-limit check ถูก skip ทั้ง branch
- **ทำไม:** ฟังก์ชัน Service.New() อนุญาตให้รับ *limit.Validator เป็น nil ได้ เพื่อให้ง่ายต่อการทดสอบ Part 2 แบบ Pure Logic โดยไม่ต้องพึ่ง Part 3 (ไม่ต้อง Mock DailyLedger) นอกจากนี้ Use case อย่าง Internal Ops Tool ที่ต้องการข้ามโควตาลูกค้า VIP ก็สามารถ Opt-out ได้เพียงแค่ปรับ Config ของ Service ตัวเดียว
- **ราคาที่จ่าย:** ต้องมี Nil check ใน service.Validate() ก่อนเรียก limit.Apply() ซึ่งถือว่าคุ้มค่าเมื่อเทียบกับความยืดหยุ่นที่ได้มี nil check ใน service.Validate() ก่อนเรียก limit.Apply() — แลกแล้วคุ้ม

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

- **ทำไม** รวม rule check ที่เดียวเพื่อ central log กับ audit
- **ข้อเสีย** ต้อง migrate ถ้าแต่ละ module ใหญ่เกินไปในภายหลัง เช่นการเช็คอาจต้องมีการ KYC etc.

### Short-circuit ที่ error แรก (full short-circuit)

- **ที่ลอง:** รัน Rule ทีละตัว เจอ Error แรกก็ Return เลยจะได้ไม่ต้องเสียเวลาเช็คส่วนอื่น
- **ที่ปฏิเสธ:** UX แย่กว่ามาก หาก User ส่ง Order ที่ Quantity ผิด และ Price Stale พร้อมกัน การเก็บ Short-circuit ไว้คั่นระหว่าง Phase เหมาะสมกว่า เช่น Shape ผิด -> ไม่ต้องถาม DB (ประหยัด Lookup), Market ล่ม -> ไม่ต้องเช็ค Balance (เพราะ Validate ต่อไม่ได้) แต่ภายใน Phase เดียวกัน ควร Collect Error ให้ครบ

## 3. กระบวนการวิเคราะห์ Part 1 (debugging process)

ลำดับการอ่านโค้ด ประมาณนี้

1. **อ่านรอบแรก เพื่อเข้าใจว่ามันทำอะไร** — ทีแรกดูก่อนว่า context ของ code คืออะไร
2. **ใช้ AI Research + Google เพื่อดูพวก library ต่างๆ** จะดูว่า Lib เกี่ยวกับ DB เป็นไงใช้ถูกต้องไหม โดยเฉพาะ SQL Injection + SQL Handling
3. **หา memory leak** อันนี้มาดูก่อนว่า mem leak อยู่ตรงไหนบ้าง
4. **ดูข้อ 2 ต่อ** ว่ามีการทำ pagination ไหม หรือทีการทำ chunk ไหม ว่า DB เพราะการที่ memory bloat ใน ram = service down ผมจะทำ backpressure etc ตามสไตล์คนทำงาน batch proceser
5. **Optimized SQL** ก่อนเลย อย่างแรก เพราะ DB เป็น bottomneck ตลอด
6. **Race Condition** เช็คพวก race condition Idempotency key ต่อ
7. คือใช้ AI ช่วยว่าผมขาดส่วนไหน ประมาณนี้เลย

## 4. ถ้าโตเป็น production service ระดับ thousands of orders/min

### สิ่งที่ผมจะ keep

- Dependency-injected interfaces — single biggest enabler ของการเปลี่ยน horizontally
- `Result` shape + stable error `Code` constants — client depend on นี้
- Decimal arithmetic — สำคัญ *กว่า* ที่ scale ใหญ่ ไม่ใช่น้อยลง
- "Fail closed when market price unavailable" rule
- Hardening pieces ที่เพิ่งทำ: config validation, sanity ceilings, customer_id scrubbing — ทั้งหมดยิ่งสำคัญที่ scale

### สิ่งที่สร้างไว้แล้วเป็นรากฐาน
- Transaction Safety (BEGIN; SELECT FOR UPDATE; COMMIT;): ครอบคลุม Account Lock, Balance Check, Daily Ledger และ Audit Log ไว้ใน ACID Transaction เดียวกัน

- Idempotency: ป้องกันปัญหา Double-debit ด้วย Idempotency-Key และ UNIQUE constraint

- Observability: มีการทำ Structured Logging (slog) เก็บ Metrics สำคัญโดยไม่ละเมิด PII ของลูกค้า

- HTTP Hardening: กำหนด Timeouts ชัดเจน, ป้องกัน Unknown JSON fields และรองรับ Graceful Shutdown### สิ่งที่ผม build ไปแล้วใน submission นี้ (เป็นพื้นฐานของ scale path)

### สิ่งที่จะพัฒนาต่อ (Future Roadmap)
- Market Price Caching: เพิ่ม Redis เพื่อ Cache ราคา (TTL สั้นๆ 250-500ms) ลดโหลดของ Upstream Feed

- Deep Observability: ผูก Request-Trace ID แบบ End-to-end (Gateway → Validator → Processor) และเพิ่ม Latency Histograms

- Rate Limiting: ทำ Per-customer limit (Orders/sec) ที่ API Gateway

- Event-Driven Architecture (Outbox Pattern): เมื่อ Order filled จะเขียนลง Outbox table เพื่อ Publish เข้า Kafka/NATS ไปยังระบบปลายทาง (Settlement, Notifications) โดยไม่บล็อก Core Executionotification

### Service split: validator + price feed + execution (เมื่อ scale มาถึงจุดที่ต้องแยก)
หาก Throughput สูงมาก การย้าย Price Feed และ Execution ออกจากกันจะจำเป็น ผมเล็ง Layout ไว้ดังนี้:

- validator-svc: ทำหน้าที่ตรวจสอบ Business Rules ล้วนๆ

- price-svc: Subscribe ราคาจากแหล่งนอก แล้ว Broadcast ลง Redis (Pub/Sub)

- execution-svc: รับ Order ที่ผ่าน Validation แล้วไปรัน ACID Transaction ในฐานข้อมูล

โครงสร้าง Code ปัจจุบันถูกออกแบบมาให้รองรับการ Split นี้ได้อย่างเจ็บปวดน้อยที่สุดครับ

**เมื่อไรไม่ควร split:**

ถ้า throughput ยังไม่ถึง ~100 orders/sec, ทีมยังเล็ก, observability stack ยังไม่พร้อม → 3 service = ภาระ มากกว่าประโยชน์ Monolith ที่ design interface ดีๆ ยังเป็น production-ready option ที่ valid — split เมื่อมี concrete signal ว่า boundary ใด boundary หนึ่งเริ่มเจ็บจริง

## 5. Tools ที่ใช้

ผมใช้ **Claude (Anthropic) + Gemini ** ใน assessment
- ช่วย research 
- ช่วยเขียน code guideline มาก่อนแบบเอกสารนี้แล้วผมแก้ทีหลัง
- ทำให้ภาษาที่ผมเขียนอ่านได้ง่ายขึ้น หรือบางทีถ้าคำมันผิดหลักการก็จะได้แก้ให้เพราะบางทีคำเดียวกันแต่ใช้คนละแบบ (ผมมีปัญหานี้ตอนสัมภาษณ์บ่อย)
- ช่วยเขียนโค๊ดหรือช่วยหาปัญหา index code เพื่อใช้ภายหลัง เวลาจะถามหรือส่วนให้
- ทุกส่วนผมจะอ่านแล้วมาแก้เสมอ จะไม่มีส่วนไหนที่ AI เขียนแล้วผมไม่ยุ่ง ยกเว้นการแปลงานอะไรแบบนี้ ที่ output ผิดได้บ้าง

