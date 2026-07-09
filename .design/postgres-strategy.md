# PostgreSQL Support Strategy

Status: **draft / discussion** — strategy and tradeoffs only, not an implementation plan.
Owner: TBD
References: [issue #53](https://github.com/pdlc-os/fabric/issues/53), [PR #179](https://github.com/pdlc-os/fabric/pull/179) (Ent schema parity, in flight)

## 1. Goal

Let the Fabric Hub run on either SQLite or PostgreSQL, selectable by config, with the same schema, the same store API, and the same test suite passing against both.

The motivation is operational, not feature-driven:
- SQLite is ideal for solo / single-machine / dev / embedded-broker use. Keep it.
- PostgreSQL is required for hosted multi-replica deployments where multiple hub processes share state, where backups/replication need to be ops-standard, and where a single writer is unacceptable.

We are **not** trying to make Postgres mandatory, and we are **not** trying to optimize either backend for the other's workload.

## 2. Constraints worth naming up front

- The hub is alpha. We do not need migration paths for in-the-wild data; refactoring that breaks existing `hub.db` files is acceptable per `claude.md`.
- We have **two** persistence layers today, not one (see §3). Any strategy that ignores that fact is wrong.
- The Go SQLite driver in use (`modernc.org/sqlite`) is pure Go. We will not give that up — CGO would regress build/release ergonomics on macOS arm64 and Windows.
- CI today runs against in-memory SQLite only. Adding Postgres means adding a service container or testcontainers; that is not free.
- We must keep the current behavior: `fabric server` with no config still works out of the box.

## 3. Current state (the part that shapes the options)

Two databases are stitched together at startup:

```
cmd/server_foreground.go:initStore()
 ├── pkg/store/sqlite.New(hub.db)            // ~25 tables, raw SQL, 46 migrations
 └── pkg/ent/entc.OpenSQLite(hub.db_ent)     // 7 Ent entities (User, Group, Grove,
                                              //  Agent, AccessPolicy, PolicyBinding,
                                              //  GroupMembership)
       └── entadapter.NewCompositeStore(sqliteStore, entClient)
```

Things that make Postgres easier than feared:
- `pkg/ent/entc/client.go` already has `OpenPostgres(dsn)`. Unused, but wired.
- `lib/pq v1.11.2` is already in `go.mod`.
- `DatabaseConfig.Driver` already exists with values `"sqlite"`/`"postgres"` documented; only the switch in `initStore` is hardcoded.
- Ent entities have **no** Postgres-specific annotations (no `jsonb`, no `uuid`, no `gin`, no partial indexes). The schema is dialect-neutral by accident.
- The composite-store pattern (`pkg/store/entadapter/composite.go`) shadows users/agents/groves into Ent on demand, so principals already exist in both stores.

Things that make Postgres harder than hoped:
- `pkg/store/sqlite/sqlite.go` is **5,133 lines**, ~25 tables, 46 sequential migration strings, all `?` placeholders. This is the bulk of the port.
- A few SQLite-specific idioms exist and need rewrites:
  - `INSERT OR REPLACE` (sqlite.go:3429), `INSERT OR IGNORE` (github_installation.go:37, sqlite.go:963)
  - `json_each(...)`, `json_array(...)` for ancestry queries (sqlite.go:905, 1441)
  - Schema-qualified types: `TEXT`, `INTEGER` columns map fine, but `TEXT PRIMARY KEY` storing UUIDs is not what we'd choose for Postgres natively.
- Single-writer assumption is baked in: `db.SetMaxOpenConns(1)` (sqlite.go:50), with the comment "SQLite serializes writes anyway." Code paths may rely on this serialization without realizing it.
- 46 hand-managed migration scripts have no concept of dialect. There is no Atlas, no goose, no migrate/migrate.
- Tests use `sqlite.New(":memory:")` everywhere — fast, isolated, but Postgres-incompatible.
- PR #179 (in flight) expands Ent schemas to cover every existing raw-SQL table, with a `schemadiff` build-tag gate. That PR is the foundation Option A below depends on.

## 4. Strategic options

Three coherent endpoints. They are not mutually exclusive in time — A is in fact the long arc, B and C are tactics for getting there or sidestepping it.

### Option A — Consolidate behind Ent, then add the Postgres dialect

The path PR #179 starts. Make Ent the single source of truth for *all* hub state, retire `pkg/store/sqlite/`, then flip the dialect with a config switch.

**Sequence (high-level, not a plan):**
1. Land PR #179: Ent schemas become a superset of raw SQL tables. (Already drafted.)
2. Port each raw-SQL store method (`pkg/store/sqlite/*.go`) to use Ent instead of `s.db.QueryContext`. Per file, verify behavior with the existing test suite by switching the constructor over.
3. Delete `pkg/store/sqlite/` and the 46-string migration scaffold. Ent's `Schema.Create` (or Atlas, see §6) becomes the only migration mechanism.
4. Add a `case "postgres":` branch to `initStore` calling `entc.OpenPostgres(dsn)`. Run the same auto-migrate.
5. Stand up a Postgres test fixture (`testcontainers-go` or a `dockertest` helper) and parameterize `pkg/store/...` tests over `{sqlite, postgres}`.

**Pros**
- One schema, one ORM, one migration story. Today's split is already a maintenance tax.
- The dialect-agnostic boundary is exactly what Ent is built for. We stop hand-translating SQL forever.
- `Schema.SchemaType` annotations let us *opt into* Postgres-native types (`jsonb`, real `uuid`) without breaking SQLite.
- Tests parameterize cleanly; Postgres just becomes a second backend in the same matrix.

**Cons**
- The port is large. ~5,000 lines of raw SQL across stores written by humans, often with subtle semantics (boolean-as-int, JSON-as-text, manual upserts, listing pagination, soft-delete). Each one is a small refactor with a real risk surface.
- Ent's expression power has limits. A few queries today use SQLite-specific JSON traversal (`json_each`, `json_array`) that don't translate cleanly. We may need to drop into raw SQL via `client.QueryContext` for those, dialect-switched.
- Ent's auto-migrate is convenient but blunt. Anything fancier (online migration, downtime-free index builds, data backfills) wants Atlas — which is fine, but it's another moving piece.
- PR #179 itself is large; reviewing and landing it is the gate.
- Performance characterization on Postgres is unknown territory. We may discover N+1 patterns the SQLite driver tolerated silently.

### Option B — Two parallel Store implementations behind a shared interface

Keep `pkg/store/sqlite/` as-is. Add `pkg/store/postgres/` as a second concrete implementation of the `store.Store` interface. Have `initStore` pick one. Ent stays a permissions-only sidecar in both worlds, with `OpenPostgres` used when `Driver=postgres`.

**Pros**
- No risk to the existing SQLite path. We can ship Postgres support to a small audience without touching solo users.
- Clear blast radius. Each store implementation is independent; bugs in one don't affect the other.
- Lets us do Postgres-native things (e.g., real `JSONB`, partial indexes, `LISTEN/NOTIFY` for event fan-out) without contorting through Ent.
- Doesn't depend on PR #179 landing.

**Cons**
- Two source-of-truth schemas. Drift between them is *guaranteed* to happen and equally guaranteed to be missed in code review. We have already lived this pain with the raw-SQL ↔ Ent split — that's why PR #179 exists.
- Doubles the ongoing maintenance cost of every schema change. Every new feature touching state pays the tax twice.
- Tests have to run the full suite against both backends to catch drift, and that's the only safety net.
- Doesn't solve the underlying duplication (raw SQL + Ent) — it actively makes it worse.

This is the wrong long-term answer. It is mentioned because it's the *fastest* path to "Postgres works" if we needed it under deadline pressure.

### Option C — Hybrid: Ent for the new Postgres-only surface, leave SQLite raw-SQL alone

Treat Postgres as a *deployment-only* backend. Solo/local users stay on SQLite (raw + Ent-permissions, today's setup). Hosted/multi-replica deployments use Postgres, but only via Ent — the raw-SQL store is **not ported**, instead its tables move into Ent schemas (PR #179 work) and the raw-SQL implementation is deleted.

In other words: Option A's destination, but skipping the "make raw SQL work on Postgres" intermediate step. We never have a Postgres backend that supports the raw-SQL store; we only have a Postgres backend that runs against the post-PR-#179 unified Ent schema.

**Pros**
- Avoids the worst-case scenario of Option B (two raw-SQL implementations forever).
- Forces the consolidation work to happen before the dialect-flip, which is the right order anyway.
- Same single-backend-per-deployment model as A; clean operationally.

**Cons**
- Functionally identical to Option A from the user's perspective; the only difference is sequencing.
- If PR #179 stalls, this strategy stalls too. We have no fallback Postgres path.

## 5. Recommendation

**Option A**, sequenced in two distinct releases:

1. **Release N — consolidation only.** Land PR #179, port raw-SQL stores to Ent, delete `pkg/store/sqlite/`. Backend is still SQLite. Ship to hosted VMs. Bake for one release cycle on a backend we already understand. No Postgres code in the build yet.
2. **Release N+1 — dialect flip.** Add `case "postgres":` in `initStore`, ship the SQLite→Postgres migration tool (§7), parameterize the test suite over `{sqlite, postgres}`. Operators opt in per deployment.

The reasoning is structural, not aesthetic:

1. The raw-SQL/Ent split is already the most painful part of `pkg/store/`. Adding a second dialect on top of that split (Option B) compounds the worst property of the current code. We should not double down on the design we're already trying to escape.
2. Ent's dialect abstraction is the entire reason Ent exists. We've already paid for it (in code generation, build steps, learning curve) and we're getting almost none of the benefit. Postgres support is the obvious place to cash in.
3. The schema today is already dialect-neutral (no Postgres annotations, all UUIDs as `TEXT`, all JSON as `TEXT`). Migration to Postgres is a flag-flip, not a redesign. We can opt into `jsonb`/native `uuid`/etc. *later*, file-by-file, once the boundary works.
4. The work PR #179 represents is happening anyway. Treating Postgres support as the forcing function for that consolidation gives both efforts a clearer "done" definition.

### 5.1 Why two releases instead of one big change

All hosted Fabric installations today run SQLite on VMs. They have data. They will need to migrate. That fact reframes "safety":

- **Code-path safety** (don't break existing SQLite installations during the port). Two-release sequencing addresses this. Release N is "same backend, new code on top," which can be exercised on hosted VMs without anyone touching Postgres. If something regresses, the rollback target is a known SQLite binary, not "go back to a different ORM."
- **Data-migration safety** (move SQLite data to Postgres reliably, with a tool you can test in CI). This is what makes Option A *better* than Option C, not worse: at the moment of migration, both endpoints speak the same Ent schema. The migration tool is an entity-by-entity copy through the Ent client — small, reviewable, fixture-testable. Under C the source would still be raw-SQL tables and the destination would be Ent, which is a paradigm shift mid-migration with no shared code to test against.

Net: A's two-release sequencing gives us C's "don't change two things at once" property *and* keeps the simple migration tool that only A makes possible.

### 5.2 Honesty about timeline

This is a multi-month arc, not a sprint. Every week PR #179 is unmerged is a week the strategy is blocked. If timeline pressure appears, Option B becomes the temporary answer — but only with a written commitment to retire the raw-SQL Postgres path once Option A lands.

## 6. Cross-cutting concerns

These apply to whichever option we pick.

### 6.1 Migration tooling

Today: hand-rolled `migrationVN string` constants applied sequentially in `Migrate()`.
Ent default: `client.Schema.Create()` — declarative, idempotent, fine for dev.
Production-grade: **Atlas** (already an indirect dep via Ent) — versioned migrations, dialect-aware diffing, online-safe operations.

Recommendation: keep `Schema.Create` for dev/SQLite, adopt Atlas-managed versioned migrations once Postgres is live. Don't try to do both at once.

### 6.2 Connection pooling

SQLite: `SetMaxOpenConns(1)` is load-bearing for pragma consistency and writer serialization. Don't change it.
Postgres: needs sane defaults. Suggested starting point: `SetMaxOpenConns(20)`, `SetMaxIdleConns(5)`, `SetConnMaxLifetime(30m)`. Expose all three via `DatabaseConfig`.

### 6.3 Concurrency assumptions

SQLite serializes writes implicitly. Some hub code (e.g., `controlchannel`, `scheduler`) may assume "if I read then write, no one else wrote in between." Postgres MVCC will violate that. We should audit transactional boundaries (`BEGIN`/`COMMIT` usage) before flipping the dialect — not as part of the port, but as a parallel hardening pass. Likely candidates: anything that does read-modify-write on `agents`, `messages`, `notifications`.

### 6.4 Type mappings to revisit *after* Postgres works

Don't do these on day one. They are second-pass optimizations:

| Today (portable) | Postgres-native upgrade |
| --- | --- |
| `id TEXT` (UUID string) | `id UUID` |
| JSON columns as `TEXT` | `JSONB`, with GIN indexes on hot paths |
| `created_at TEXT` (RFC3339) | `TIMESTAMPTZ` |
| Ancestry via `json_each` | `ltree` or recursive CTE |

Each becomes a per-field `SchemaType` annotation; SQLite continues to use `TEXT`.

### 6.5 Driver choice for Postgres

`lib/pq` is already in `go.mod`. It works and is Ent-supported. `pgx` (in `database/sql` mode) is more actively maintained and gives us better performance and richer types if we eventually want them. **Recommendation: start with `lib/pq`** (zero new deps, smallest diff), revisit `pgx` only if we hit a specific limitation.

### 6.6 Test infrastructure

Two real options:
1. **`testcontainers-go`** — spins up a real Postgres in Docker per test package. Slow (~3-5s startup), realistic, requires Docker in CI.
2. **`pgtest` / `embedded-postgres`** — downloads a Postgres binary, runs in-process. Faster restart, no Docker, but binary download in CI.

Recommendation: `testcontainers-go`, gated behind a build tag (`//go:build integration`) so unit-test runs stay fast. Run the full `{sqlite, postgres}` matrix in CI on PRs that touch `pkg/store/...` or `pkg/ent/...`, plus nightly.

### 6.7 SQLite driver story

Stay on `modernc.org/sqlite`. The pure-Go property is more valuable than any perf delta from `mattn/go-sqlite3`, and we already have it working.

## 7. Migration tooling and testing

This section is the operational counterpart to §5.1. The two-release sequencing creates two distinct migrations, and each needs its own tested tool.

### 7.1 The two migrations

**Migration α — raw-SQL-on-SQLite → Ent-on-SQLite** (Release N).
This is the harder migration in concept, easier in execution: schema layout changes (Ent's table structure may differ from the hand-written DDL), but the dialect and the data file stay the same. Every existing hosted VM goes through this on upgrade.

**Migration β — Ent-on-SQLite → Ent-on-Postgres** (Release N+1, opt-in).
Same Ent schema on both sides; only the dialect changes. This is the migration that exists *because* we picked Option A — under any other option it would be much harder.

### 7.2 Migration α: how it works

Two viable shapes:

- **In-process on first boot.** Ent's `Schema.Create` is idempotent, but it does not move data between renamed/restructured tables. We add a one-shot upgrade routine: read from the old raw-SQL tables (still present in the file), write into the new Ent-managed tables, drop the old tables. Triggered by a schema-version sentinel.
- **External CLI.** `fabric server migrate --from sqlite-v46 --to ent` run by the operator before upgrading binaries. Safer (operator controls timing, can back up first) but adds an ops step.

Recommendation: **in-process upgrade with a mandatory backup hint logged at startup** (`"Detected v46 raw-SQL schema. Backing up to hub.db.bak.<ts> before migration."`). The hub already owns the file; we should own the migration. Operators can opt into manual mode with `--no-auto-migrate`.

### 7.3 Migration β: how it works

Because both endpoints use the same Ent schema, the tool is roughly:

```go
// Pseudocode — shape, not literal API.
src := entc.OpenSQLite(srcDSN)
dst := entc.OpenPostgres(dstDSN)
entc.AutoMigrate(ctx, dst)  // ensure schema exists

for _, ent := range entitiesInDependencyOrder {
    rows, _ := ent.QueryAll(ctx, src)
    ent.BulkCreate(ctx, dst, rows)
}
```

Ships as `fabric server migrate --from sqlite://... --to postgres://...`. Run once, manually, by the operator. Key properties to design in:

- **Idempotent.** Re-running on a partially populated destination skips rows that already exist (by primary key). This is what makes restart-after-failure tolerable.
- **Read-only on source.** Source is opened with `_query_only=1` pragma. Operators can keep the SQLite hub running until they cut over.
- **Dependency-ordered.** Insert groves before agents, users before group memberships, etc. The order is derivable from Ent's edge graph.
- **Single transaction per entity, not per row.** Postgres can swallow large `CreateBulk` batches; one txn per entity bounds memory and keeps progress reportable.
- **Verifies row counts.** After each entity, count source vs destination, fail loud on mismatch.

### 7.4 Testing both migrations in CI

This is the part the user flagged: "we probably want to be able to do some sort of repeatable migration test from SQLite." Concretely:

**Canonical fixture.** A single seeded SQLite database file (`testdata/hub-v46-fixture.db`), checked into the repo, that contains representative rows for every table — at least one per entity, with edge cases (NULL optional fields, max-length strings, JSON with nested structure, soft-deleted rows, etc.). This file is the "known-good source of truth" for both migrations. Updating it requires a deliberate PR.

**Test for Migration α** (in the `pkg/store/...` test package):
1. Copy the fixture to a temp path.
2. Open it with the new Ent-on-SQLite code path. Trigger upgrade.
3. Assert: every row from each pre-upgrade table is present in the corresponding Ent entity, with values intact.
4. Assert: the hub starts cleanly, basic queries (list groves, list agents, fetch a user) return the expected fixtures.
5. Run again. Assert idempotency (no duplicate rows, no errors).

**Test for Migration β** (gated `//go:build integration`):
1. Take the fixture, run Migration α to get to Ent-on-SQLite.
2. Spin up Postgres via `testcontainers-go`.
3. Run the migration tool.
4. For each entity, assert source-count == destination-count and a sampled row-by-row equality check.
5. Start a second hub instance pointed at Postgres; run the same handler-level smoke tests we ran on SQLite. Assert behavior parity.

**End-to-end (nightly).** A CI job that does α → β → run the full hub test suite against Postgres. Catches any test that implicitly assumed SQLite semantics.

### 7.5 Reversibility

Migration α is one-way: once the raw-SQL tables are dropped, you can't go back without restoring the backup file. That's acceptable because the backup is taken automatically and the new schema is well-tested before release.

Migration β is also one-way operationally (we don't ship a Postgres→SQLite tool), but the SQLite source is left untouched — operators can simply keep using it if they want to roll back. Recommend a `migrate --keep-source` default and a `--drop-source` flag for explicit cutover.

## 8. Open questions

1. **Timeline for PR #179.** Is it close to merge or stalled? The strategy hinges on it.
2. **Hosted-only Postgres, or also offered for local?** If only hosted, we can defer ergonomics like a `fabric server --postgres-url=...` flag. If we want local devs to easily try Postgres, we need a `docker compose up` story.
3. **Multi-writer requirements.** Does the hub today have any code path where two replicas would actually need to coordinate, or is "one hub process per Postgres" the v1? The latter is much simpler.
4. **Backup/restore model.** SQLite is "copy the file"; Postgres needs `pg_dump` / WAL-archiving / managed-service backups. Out of scope for this doc but flagging for ops planning.
5. **Sqlite stores that have no Ent equivalent yet.** PR #179 reportedly adds 20 new schemas. Is there anything in `pkg/store/sqlite/*.go` left uncovered? An audit before declaring Phase 1 complete is warranted.
6. **Migration α: in-process or external CLI?** §7.2 recommends in-process with auto-backup. Confirm operator ergonomics align with that choice before implementing.
7. **Canonical fixture ownership.** Who owns updates to `testdata/hub-v46-fixture.db` when a new entity or field lands? A pre-commit gate that regenerates it from a Go-defined fixture spec is probably worth the up-front cost over a binary blob in git.

## 9. Decision log

_To be filled in as decisions are made._

- _YYYY-MM-DD_ — strategy direction chosen (A / B / C)
- _YYYY-MM-DD_ — Postgres driver choice confirmed (`lib/pq` / `pgx`)
- _YYYY-MM-DD_ — migration tooling chosen (Ent `Schema.Create` / Atlas)
- _YYYY-MM-DD_ — test infra chosen (`testcontainers-go` / `embedded-postgres` / other)
