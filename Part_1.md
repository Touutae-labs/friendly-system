# Part 1 — วิเคราะห์ `process_gold_order` (ฉบับภาษาไทย)

**ผู้ส่ง:** Pantakan Totae · `pantakan.totae@gmail.com`
**วันที่:** 2026-05-16
**Repo:** ดู `README.md` สำหรับ layout และวิธีรัน
**English version:** [`Part_1_EN.md`](Part_1_EN.md)

---

## 1. ฟังก์ชันนี้ทำอะไร

`process_gold_order` เป็นฟังก์ชันที่เปิด connection ไปยัง SQLite database `trading.db` ใช้ `customer_id` เป็น key เพื่อ query `balance` กับ `name` ของลูกค้าออกมา แล้วจัดการเคสแยกตาม `order_type` (`buy` กับ `sell`)

- **buy** คำนวณ `total_cost = quantity * price` ถ้า `balance >= total_cost` หัก `total_cost` ออกจาก `balance` แล้ว insert row ลงตาราง `orders` commit แล้ว return `{"status": "success", "balance": new_balance}` กลับไป หาก balance ไม่พอก็ return dict failed กลับไป (`new_balance` คือยอดหลังหักแล้ว)
- **sell** เพิ่ม `balance` ด้วย `quantity * price` ทันที **ไม่มี validation ใดๆ** insert row ลง `orders` commit แล้ว return success
- order_type อื่นๆ นอกจาก `buy`/`sell` ตกลงมา return `None`

## 2. ปัญหาที่เจอ ทำไมเป็นเรื่องใหญ่ และวิธีแก้

เจอมากกว่า 4 ข้อ ข้างล่างคือสิ่งที่ผมจะแก้ก่อน production — เรียงตาม blast radius

### Problem 1 — SQL injection ทุก query

- **อะไรผิด:** โค้ดเอา input ของ user มาต่อ SQL โดย string concat / f-string ทุก statement ทั้ง `SELECT`, `UPDATE`, `INSERT`
- **ตัวอย่างการ exploit:** `customer_id = "1; UPDATE customers SET balance = 999999999 WHERE id = 1;--"` รันได้เลย
- **ผลกระทบ:** attacker เปลี่ยน balance ลูกค้าได้ทุกคน, drop ตาราง `orders`, หรือ dump customer list ออกทั้งฐานข้อมูล
- **วิธีแก้:** ใช้ parameterised query (placeholder `?`) ให้ driver จัดการ escape — ห้ามต่อ string เอง

```python
# ผิด
"SELECT balance, name FROM customers WHERE id = " + str(customer_id)

# ถูก
conn.execute("SELECT balance, name FROM customers WHERE id = ?", (customer_id,))
```

ORM สมัยใหม่ (SQLAlchemy, Django ORM, GORM ฯลฯ) ใช้ parameter binding อัตโนมัติอยู่แล้ว ถ้าไม่ได้เขียน raw query แทบไม่ต้องคิดเรื่อง escape เลย

### Problem 2 — Float arithmetic สำหรับเงิน

- **อะไรผิด:** `quantity * price` และ `balance - total_cost` ใช้ Python `float` (IEEE-754 binary)
- **ทำไมพัง:** `0.1 + 0.2 == 0.30000000000000004` — error สะสมไปเรื่อยๆจนเลขในระบบไม่ตรงกับ external books, reconciliation พัง, ผิด rounding rule ของ regulator (ผิดทั้งกฎหมาย และ อาจทำให้สูญเงินจำนวนมากถ้า transaction เยอะ)
- **วิธีแก้:** ใช้ `decimal.Decimal` พร้อมกำหนด rounding mode ให้ชัด หรือเก็บเป็น integer satang (1 บาท = 100 satang) — module ใน Part 2/3 ของผมใช้ `shopspring/decimal` ด้วยเหตุผลนี้

### Problem 3 — ไม่มี exception handling; connection ค้างเมื่อ DB error

- **อะไรผิด:** ไม่มี `try`/`except` ครอบ DB writes เลย และไม่มี `conn.close()` ทุก return path
- **สิ่งที่เกิดขึ้นจริงใน sqlite3 default:** `UPDATE` กับ `INSERT` ทั้งคู่อยู่ใน implicit transaction เดียวกัน ถ้า `INSERT` fail ก่อน `conn.commit()` ถูกเรียก → exception ลอยขึ้น uncaught → `conn.commit()` ไม่ถูกเรียก → ไม่มีอะไรถูก commit เลย (ทั้ง UPDATE และ INSERT) แต่ **connection ยังเปิดค้างอยู่** พร้อม uncommitted transaction ที่ lock ไฟล์ database ไว้ → connection ถัดไปที่พยายาม write จะ block หรือ timeout
- **กรณีที่อันตรายกว่า:** ถ้าเปลี่ยน driver เป็น autocommit (psycopg3 default หรือ psycopg2 ที่ตั้ง `conn.autocommit = True` ซึ่งเจอบ่อยใน connection-pool setup) — UPDATE จะ commit ทันทีโดยไม่รอ INSERT ทำให้ balance หักไปแล้วแต่ order หายจริงๆ โค้ดนี้เป็น time bomb ถ้า migrate DB
- **วิธีแก้:** ครอบด้วย `try`/`except` และ `finally` เพื่อ close connection เสมอ

```python
conn = sqlite3.connect("trading.db")
try:
    with conn:                        # auto commit / rollback
        conn.execute("UPDATE …", (new_balance, customer_id))
        conn.execute("INSERT …", (customer_id, order_type, quantity, price, total))
except sqlite3.Error:
    return {"status": "failed", "reason": "persistence error"}
finally:
    conn.close()
```

หมายเหตุ: `with sqlite3.connect(...) as conn:` **ไม่ได้ close connection** — Python's context manager ของ sqlite3 จัดการแค่ commit/rollback เท่านั้น ต้อง close เองผ่าน `finally`

### Problem 4 — Race condition: buy พร้อมกัน 2 ครั้ง overdraw ได้

- **อะไรผิด:** read-then-check-then-write ไม่ปลอดภัยใน concurrent context แม้จะมี transaction แล้วก็ตาม
- **ตัวอย่าง:** 2 thread อ่าน `balance = 1000` พร้อมกัน, ทั้งคู่เห็น `1000 >= 600`, ทั้งคู่หัก → balance เหลือ `-200`
- **ผลกระทบ:** ลูกค้า overdraw ได้ฟรีๆ (TOCTOU — time-of-check / time-of-use bug)
- **วิธีแก้:** ทำให้ debit atomic กับ check ใน SQL เดียว

```sql
UPDATE customers
   SET balance = balance - :cost
 WHERE id = :customer_id
   AND balance >= :cost;            -- 0 rows updated = balance ไม่พอ
```

หรือใช้ row-level lock (`SELECT … FOR UPDATE`) / optimistic concurrency ก็ได้

### Problem 5 — Sell ไม่ตรวจสอบอะไรเลย

- **อะไรผิด:** branch `sell` ไม่มี validation เลย ไม่เช็คว่ามีทองให้ขายจริงไหม, ไม่เช็ค `quantity > 0`, ไม่เช็คราคาใกล้ตลาด
- **ผลกระทบ:** ลูกค้า "ขาย" เท่าไหร่ ราคาเท่าไหร่ก็ได้ → effectively ปั๊มเงินเข้า balance ฟรีๆ
- **วิธีแก้:** ทุก order path ต้องผ่าน validation pipeline เดียวกัน (quantity > 0 + valid increment, price > 0, price ใกล้ market) บวก inventory check สำหรับฝั่ง sell — module ใน Part 2/3 ของผม validate ทั้ง buy/sell ผ่าน pipeline เดียวด้วยเหตุผลนี้

### Problem 6 — Customer not found crash + resource leak + no input validation

- **bug A — `fetchone()` return `None`:** ถ้า `customer_id` ไม่มีในระบบ `cursor.fetchone()` คืน `None` → `customer[0]` crash ทันทีด้วย `TypeError: 'NoneType' object is not subscriptable` — แทนที่จะได้ structured error กลับไป
- **วิธีแก้ A:**
```python
customer = cursor.fetchone()
if customer is None:
    return {"status": "failed", "reason": "customer not found"}
```
- **bug B — Resource leak:** `conn` ไม่ถูก close ทุก return path → service รันยาวๆ leak file handles จนกระทั่งเปิด connection ใหม่ไม่ได้ (ดู fix ใน Problem 3)
- **bug C — ไม่มี input validation:** `customer_id`, `quantity`, `price`, `order_type` รับมาดิบๆ → quantity ติดลบ, price เป็น 0 หรือ NaN, order_type เป็น `"BUY"` (uppercase) ผ่านได้หมด — เคสสุดท้าย return `None` ซึ่ง caller ที่คาดว่าเป็น dict จะ crash
- **วิธีแก้ C:** validate inputs ก่อนเปิด DB connection แล้ว return structured shape เสมอ ไม่มี `None`

## เรื่องเล็กที่จะ flag ใน review ด้วย

- `print(f"Order successful for {name} …")` leak PII ออก stdout ไม่ใช่ audit trail จริง ควรใช้ structured logger + ตาราง audit log ที่ persist
- ไม่มี idempotency key — network retry submit order ซ้ำได้
- hardcode `"trading.db"` ขึ้น cwd ควร inject path เข้ามา
- return shape ไม่ consistent (`None` vs dict) → caller handle ยาก
