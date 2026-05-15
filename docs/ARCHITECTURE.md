# Architecture — Vertical-slice domain with ports & adapters

โครงสร้างนี้คือ **ports & adapters** แบบ vertical slice ภายใต้ root `domain/` — แต่ละ business module เป็น self-contained slice ที่มี business rule + port + in-memory adapter อยู่ใน folder เดียว; production-shaped adapters (GORM-based) แยกอยู่ที่ `domain/repositories/` เพราะมัน share ORM models กับ schema เดียวกัน เน้น **business เป็น first-class citizen** ไม่ใช่ technical layer

## โครงสร้าง

```
domain/                                ← root ของ domain layer ทั้งหมด
  common/order/order.go                ← shared kernel: Order, Result, Error, OrderType
  module/                              ← per business sub-domain; self-contained
    orderval/core.go                   ← rule (no I/O, no port)
    quote/
      core.go                          ← rule
      provider.go                      ← MarketPriceProvider port (Provider pattern)
      memory.go                        ← in-memory adapter (quote.Memory)
    balance/
      core.go                          ← rule
      repository.go                    ← AccountRepository port (Repository pattern)
      memory.go                        ← in-memory adapter (balance.Memory)
    limit/
      core.go                          ← rule
      repository.go                    ← DailyLedger port
      memory.go                        ← in-memory adapter (limit.Memory)
  repositories/                        ← GORM implementations ของ repository ports
    models.go                          ← AccountModel, DailyTotalModel (GORM tags)
    account.go                         ← repositories.Account → balance.AccountRepository
    ledger.go                          ← repositories.Ledger  → limit.DailyLedger
  service/validator/service.go         ← orchestrator (plain Go, no Wire)
  mocks/                               ← Mockery output, generated
wire/                                  ← Google Wire composition root (infrastructure)
  wire.go, providers.go, gorm.go, wire_gen.go
cmd/
  seed/main.go                         ← inserts sample data (Atlas owns the schema, see schema.sql)
  server/main.go                       ← HTTP server (POST /orders/validate)
```

## หลักการแบ่ง folder

### `domain/common/` — kernel ที่ใช้ข้าม module

มีแต่ type ที่ "ข้าม" module เช่น `Order`, `Result`, `Error` — ถ้าวันหน้ามี `execution-svc` มา consume order intent มันก็ import `common/order` เดียวกับ validator ไม่ต้องลาก `module/balance` มาแค่เพื่อใช้ type

**กฎ:** common ห้ามมี business logic ห้ามมี I/O ห้าม depend on อะไรใน `module/`, `repositories/`, หรือ `service/` — มันคือใบไม้ใบล่างสุดของ dependency tree

### `domain/module/<name>/` — 1 folder = 1 business sub-domain

แต่ละ module เป็น **vertical slice** มีทุกอย่างที่เกี่ยวกับ sub-domain นั้นรวมอยู่ใน folder เดียว:

| ไฟล์ | บทบาท |
|---|---|
| `core.go` | business rule + Validator struct + Config + error codes |
| `repository.go` / `provider.go` | port interface ที่ rule นี้ต้องการ (Repository pattern สำหรับ persisted state, Provider pattern สำหรับ read-only gateway) |
| `memory.go` | in-memory adapter ของ port — ใช้ test, demo, dev |

**ตัวอย่าง `domain/module/balance/`:**
- `core.go`: `Validator` (rule logic), `Code*` (error codes)
- `repository.go`: `AccountRepository` (port) + `ErrCustomerNotFound`
- `memory.go`: `balance.Memory` struct ที่ implement `AccountRepository` แบบ thread-safe in-memory map

**กฎสำคัญ — ports อยู่ที่ module:**
- Module เป็นคนรู้ว่าตัวเอง depend on อะไร → module เลย define interface
- Adapter (อยู่ใน `module/*/memory.go` หรือ `repositories/*.go`) เป็นคนทำให้สำเร็จ → adapter ผูก behaviour ใส่ interface ของ module
- Compile-time contract check: `var _ AccountRepository = (*Memory)(nil)` ในไฟล์ memory.go และ `var _ balance.AccountRepository = (*Account)(nil)` ใน repositories/account.go — interface เปลี่ยน adapter fail compile ทันที

### `domain/repositories/` — GORM-backed production adapters

แยกจาก `module/*/memory.go` เพราะ:
- มี **ORM models** ที่ share table schema ระหว่าง 2+ repository — `AccountModel` กับ `DailyTotalModel` อยู่ในไฟล์ `models.go` เดียว schema เจ้าของจริงคือ `schema.sql` ที่ Atlas apply ตอน `make seed` (GORM tags ในไฟล์ models ใช้สำหรับ query mapping เท่านั้น ไม่ใช้ AutoMigrate)
- มี **shared transaction concerns** — repositories ทั้ง package ใช้ `*gorm.DB` ตัวเดียวกัน เปิด tx ครอบ multi-repo write ได้ในอนาคต
- GORM-specific code (clause hints, raw SQL escape hatches) รวมที่เดียว — ไม่กระจาย import GORM เข้า module
- คนอ่านเปิด `domain/repositories/` เห็นทั้ง persistence layer จบในที่เดียว แทนที่จะ jump 4 folder

**กฎ:** `repositories/` import จาก `module/*` ได้ (เอา interface มา satisfy) แต่ `module/*` ห้าม import จาก `repositories/` (จะกลับทิศ dependency)

### `domain/service/<name>/` — orchestrator

`domain/service/validator/service.go` ร้อย 4 modules เข้าด้วยกัน — `Validate(o)` เรียก `orderval.Apply` → `quote.Apply` → `balance.Apply` → `limit.Apply` พร้อม short-circuit logic ที่ไม่เป็นของ module ใด module หนึ่ง:

- shape fail → return ก่อน fetch market (เปลือง upstream)
- market unavailable → return ก่อน hit balance/ledger (fail closed)
- buy เท่านั้นถึงรัน balance.Apply
- limit เป็น optional — `nil` = skip ทั้ง branch

**Service ห้าม import Wire** — Wire เป็น infrastructure ส่วน service เป็น domain — domain ไม่ควรรู้จัก codegen tool

### `domain/mocks/` — Mockery output

generate จาก port interface ใน `module/*/repository.go` / `provider.go` ออกที่ `domain/mocks/<package>/<InterfaceName>.go`

อยู่ใต้ `domain/` เพื่อ scope ให้ชัดว่ามัน test artifact ของ domain layer ไม่ใช่ tool config — `make wire` + `make mock` overwrite ทั้ง folder ได้สะอาด

### `wire/` — Google Wire composition root

อยู่ **นอก** `domain/` เพราะ Wire = infrastructure ไม่ใช่ business

มี 2 injector:
- `InitValidatorService()` — ใช้ in-memory adapters (`module/*/memory.go`)
- `InitValidatorServiceGorm(db *gorm.DB)` — ใช้ GORM adapters (`domain/repositories/`)

provider functions ระบุว่า port ไหนผูกกับ adapter ตัวไหน Wire generate `wire_gen.go` เป็น constructor code ตอน `make wire` + `make mock`

**ทำไม Wire ไม่ใช่ uber-fx / dig (reflection):**
- Compile-time check — ลืม register provider, build fail ทันที ไม่ใช่ panic ตอน startup
- Generated code อ่านได้ ไม่ใช่ runtime magic — debug ง่าย
- Production binary ไม่มี Wire runtime — codegen เสร็จแล้ว Wire library หายไป

## Flow ของ dependency

```
   cmd/seed|server                          ← binary entry points
        │
        ▼
   wire/                                    ← Google Wire composition root
        │  (assembles the graph below)
        ▼
   domain/service/validator                 ← orchestrator (plain Go)
        │  (calls module.Apply in order, short-circuits on phase failure)
        ▼
   domain/module/{orderval,quote,balance,limit}
        ▲ (uses port)                      ▲ (implements port)
        │                                   │
        │                              domain/module/<x>/memory.go  (in-memory)
        │                              domain/repositories/*.go     (GORM/SQL)
        │
        └── domain/common/order              ← shared types only
```

ลูกศรชี้ "ใครรู้จักใคร":
- `common/order` ไม่ import จากใคร — leaf
- `module/*` import แค่ `common/order`
- `repositories/*` import จาก `module/*` (เพื่อ satisfy port) — ไม่ใช่ทางกลับ
- `service/validator` import ทุก `module/*` + `common/order`
- `wire/` import ทุกอย่าง — มันคือคนร้อย graph

`domain/` เป็น pure domain ไม่รู้จัก Wire; `wire/` import `domain/*` เพื่อ assemble dependency graph

## เพิ่มของใหม่ ไปอยู่ตรงไหน?

| สิ่งที่จะเพิ่ม | ไปอยู่ที่ |
|---|---|
| Business rule ใหม่ใน sub-domain เดิม | `domain/module/<name>/core.go` (เพิ่ม method + Config field) |
| Business sub-domain ใหม่ทั้งหมด (KYC, AML, FX rate) | `domain/module/<newname>/` (core.go + repository.go ถ้ามี port) |
| Production adapter ใหม่ (Postgres, MySQL, Redis) | `domain/repositories/<name>.go` ใน package `repositories` (เพิ่ม ORM model ใน `models.go` ถ้าต้อง) |
| In-memory test adapter ของ module เดิม | `domain/module/<existing>/memory.go` (อยู่ตรงนั้นแล้ว — แก้ struct) |
| Service ใหม่ทั้ง deployment unit | `domain/service/<name>/` + `cmd/<name>/main.go` + เพิ่ม injector ใน `wire/` |
| Cross-module type ใหม่ | `domain/common/<name>/` (เช่น `common/customer/`, `common/transaction/`) |
| HTTP endpoint ใหม่ | `cmd/server/main.go` — เพิ่ม `mux.HandleFunc(...)` |

## ทำไมไม่ใช้ layout อื่น?

- **Package เดียว flat:** business / infrastructure ปนกัน ขยายลำบาก
- **MVC-style (controllers/models/services):** สะท้อน *technical role* ไม่ใช่ *business domain* — เพิ่ม sub-domain ใหม่ต้องแทรกที่หลาย folder
- **`adapter/` แยก folder ออกจาก module** (เคยลอง): navigate ยาก เปิด module ดู rule แล้วต้อง jump folder อื่นเพื่อดู in-memory implementation
- **ทุก adapter อยู่ใน module/*** (ไม่มี repositories/ แยก): production adapters ที่ share schema/transaction/ORM กระจายตัวข้าม module — เปลี่ยน table layout ต้องแก้ทั้ง 2 module ทีละไฟล์ แทนที่จะแก้ใน `models.go` ที่เดียว
- **Wire ในไฟล์ใน service/ หรือ cmd/:** Wire เป็น infrastructure ไม่ใช่ domain — ใส่ใน service ผูก domain กับ codegen tool ใส่ใน cmd ผูกกับ binary เดียวที่ใช้

## Trade-offs ที่ยอมรับ

1. **2 ที่ ที่ adapter อยู่ได้ (module/memory.go vs repositories/):** เปิดทาง confusion เล็กๆ — แต่กฎชัด: in-memory test/demo อยู่ใน module, production-shaped (GORM/HTTP/Redis) อยู่ใน `repositories/`
2. **Wire codegen step:** เพิ่ม `make wire` + `make mock` ในวงจร dev — แลกกับ compile-time DI safety
3. **Service มี orchestration logic:** module ไม่รู้ลำดับ flow → service ฉลาดพอตัดสินใจ short-circuit — เพราะ flow contextual กับ deployment ที่เรียกใช้
4. **Result shared by reference:** ทุก module Apply() เขียน error เข้า `*Result` เดียวกัน → collect all errors ใน single pass แทนที่จะ short-circuit ทุก rule
