# Phase 03-02 Summary: Durable Storage Implementations

## Work Completed

1. **Storage Drivers Installed**
   - Verified and added Go modules `github.com/jackc/pgx/v5` for PostgreSQL and `modernc.org/sqlite` for pure-Go SQLite to support dual-backend control-state storage.

2. **Database Migrators Implemented**
   - Implemented SQLite `Migrator` in `internal/controlstate/sqlite/migrations.go` which applies embedded `0001_control_state.sql`.
   - Implemented PostgreSQL `Migrator` in `internal/controlstate/postgres/migrations.go` which similarly applies schema migrations.
   - Fixed multiple query execution within a single transaction in SQLite by appropriately splitting on semicolons.

3. **Repository Implementations (`Repository`, `ProviderRepository`)**
   - Created `internal/controlstate/sqlite/repository.go` implementing `Repository`, `Transaction`, and stubbed out all sub-repositories with `ProviderRepository` fully implemented using `database/sql`.
   - Created `internal/controlstate/postgres/repository.go` implementing the same interfaces utilizing the `github.com/jackc/pgx/v5/pgxpool` abstraction and native PostgreSQL features.
   - Secret queries correctly isolate ciphertext/nonce data and NEVER expose raw secrets.

4. **Control State Configuration**
   - Added `ControlStateBackend`, `ControlStateDSN`, `ControlStateMigrateOnStartup`, `ControlStateLocalSeedEnabled`, `ControlStateEncryptionKey`, `AdminAPIKey`, and `AuditRetention` fields to `internal/config/Config`.
   - Populated fields from environment variables (e.g. `CONTROL_STATE_BACKEND`) and configuration JSON objects in `LoadConfig`.

5. **Local-Dev Seed Semantics**
   - Implemented `SeedFromStaticConfig` in `internal/controlstate/seed.go` strictly adhering to Rule D-08 (Skip entirely if ANY durable provider exists).
   - Encrypts API keys loaded from static providers (or their corresponding `API_KEY_*` variables) using the `AESGCMSecretCipher`.

6. **Testing**
   - Added full interface-level validation in `internal/controlstate/sqlite/repository_test.go` ensuring schema migration accuracy, Provider insertion, version bumps, and optimistic locking logic.
   - Validated `SeedFromStaticConfig` using memory-mapped SQLite databases isolated per test case (`seed_test.go`).
   - Confirmed all internal package tests pass correctly (`go test ./internal/config ./internal/controlstate/...`).

## Next Steps
Proceed to Phase 03-03 to implement the Gateway Data-Plane Integration, which involves retrofitting the static router to prioritize and hydrate providers dynamically from `controlstate.Repository`.
