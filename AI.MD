> **Build a Go (Golang) Database Bridge API with Admin UI**
>
> Create a complete Go solution called `dbbridge` that acts as a middleware gateway between client applications and legacy databases via ODBC.
>
> **Solution Structure:**
> ```
> dbbridge/
> ├── cmd/
> │   └── dbbridge/       # Main entry point
> ├── internal/
> │   ├── api/            # HTTP Handlers (Echo/Chi) & Router
> │   ├── config/         # Configuration loading
> │   ├── core/           # Domain entities & interfaces
> │   ├── data/           # SQLite repositories
> │   └── service/        # Business logic (Service Layer)
> ├── web/                # HTML Templates & Static Assets
> └── .env                # Environment variables
> ```
>
> **Security & Encryption:**
> - `.env` file at server stores `DBBRIDGE_KEY=xxx` — not in Git
> - **DB Password** stored AES-256 encrypted in SQLite, decryptable only with key from `.env`
> - **API Key** stored as HMAC-SHA256 hash in SQLite — one-way. Plain text shown only once.
> - If `.env` missing or key invalid at startup → service **fails to start**.
>
> **Core Features:**
>
> 1. **REST API** — Single endpoint:
>    ```
>    POST /api/{connectionId}/{queryId}
>    Headers: X-API-Key: xxx
>    Body: { "param1": "value1", "param2": "value2" }
>    ```
>
> 2. **Query Registry** — Queries stored in SQLite.
>    Query written with placeholders `{varname}`:
>    ```sql
>    SELECT * FROM orders WHERE customer_id = {customer_id}
>    ```
>    Bridge Service Layer steps:
>    - **Step 1** — Scan `{varname}`, replace with `?`, collect values from body.
>    - **Step 2** — Send to ODBC with positional parameter binding.
>
>    ```sql
>    -- result:
>    SELECT * FROM orders WHERE customer_id = ?
>    -- binding: [0]="123"
>    ```
>
> 3. **Connection Management** — Multiple ODBC connections. Connection string has password encrypted at rest.
>    - Target: SQL Anywhere 10 via ODBC
>    - Connection string format: `Driver={Sql Anywhere 10};...;pwd={DECRYPTED_PASSWORD};...`
>
> 4. **User & API Key Management** — Per-user API keys.
>
> 5. **Admin UI** — Server-Side Rendered (HTML/Template) at `/admin` to manage Connections, Queries, Users, and Audit Logs.
>
> **Technical Requirements:**
> - **Go 1.20** (Last version supporting Windows 7)
> - SQLite for config/metadata
> - `database/sql` + `alexbrainman/odbc` for DB execution
> - **Service Layer Architecture**: Separate business logic from HTTP handlers and Data access.
>   - Use **DTOs (Data Transfer Objects)** for data exchange between API and Service layers.
> - Parameter missing and no default → return `400`.
> - Startup validation: check `.env` and `DBBRIDGE_KEY`.
>
> **API Response format:**
> ```json
> {
>   "success": true,
>   "data": [...]
> }
> ```
>
> Start by scaffolding the project structure, SQLite repositories, Service Layer (Encryption, Executor), then API, then Admin UI.
