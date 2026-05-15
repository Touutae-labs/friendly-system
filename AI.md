# สรุปว่า AI ช่วยอะไรบ้าง (Opus 4.6)

1. Format .MD ทั้งหลาย
2. ทำแปลภาษาอังกฤษคู่กับภาษาไทย
3. ช่วยเรื่อง Honourable mention ที่พูดการ print PII ใน stdout (อันนี้ไม่ได้สังเกตเลย), Idempotency key (อันนี้ลืมสนิท), Hardcoded path ของ trading.db, inconsistence return
4. ช่วย implement Go บางส่วนกับ init repo go lang ให้
5. ปรับคำพูดที่ผมเขียนให้เข้าใจง่ายขึ้นในบางส่วนโดยไม่ให้เสียสไตล์การเขียนไป
6. มีการตรวจสอบบางอย่างช่วยผมหรือ research ข้อมูล เช่น driver sql มี explicit auto commit ของ transaction เพราะในโค๊ดเห็น commit แต่ไม่เห็น db.begin เลย หรือแม้แต่ lib อื่นๆ ความเสี่ยงที่มนุษย์มองข้ามไป
7. ตอน migrate Python `process_gold_order` มาเป็น Go executor service AI ช่วย enumerate Part 1 patches ทั้ง 6 ข้อให้ผมตรวจดูครบ + ช่วย design idempotency-key flow (UNIQUE index บน orders table + replay-as-duplicate) ที่ผมเคยลืมจากครั้งแรก
8. ตอน setup Atlas + GORM + Wire AI ช่วย research command/clause syntax ที่ผมไม่คุ้น (`clause.Locking{Strength: "UPDATE"}` สำหรับ FOR UPDATE, atlas `--dev-url` flag สำหรับ SQLite, `BEGIN IMMEDIATE` ของ SQLite driver)
