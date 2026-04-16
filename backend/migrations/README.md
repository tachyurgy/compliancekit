# ComplianceKit — Database Migrations

This directory holds all Postgres schema migrations for ComplianceKit. We use
[`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) — the
same CLI is embedded in the Go binary (so `compliancekit migrate up` works in
production) and is also available as a standalone tool locally.

## Naming convention

```
NNNNNN_short_snake_case_description.up.sql
NNNNNN_short_snake_case_description.down.sql
```

- `NNNNNN` is a zero-padded 6-digit sequence (`000001`, `000002`, …). Do NOT
  reuse numbers, do NOT skip numbers, do NOT timestamp. Sequential integers
  make merge conflicts visible during PR review.
- Both `.up.sql` and `.down.sql` are mandatory. A migration without a tested
  down step will be rejected.
- Filename description is lowercase snake_case; match it to the primary verb
  of the migration (`add_foo`, `drop_bar`, `rename_baz`).
- One logical change per migration. If you find yourself writing "and" in the
  filename, split it.

## Writing a migration

Every migration file must:

1. Start with a top-of-file comment explaining **why** (not just what — the
   SQL is the what).
2. Wrap the whole migration in `BEGIN; ... COMMIT;`.
3. Add an `updated_at` column and `set_updated_at` trigger to any new table
   that will be mutated (the trigger function is defined in `000001`; just
   attach it).
4. Use `CHECK` constraints for enum-like fields rather than native Postgres
   `CREATE TYPE ... AS ENUM`. Native enums require `ALTER TYPE` with table
   locks to add a value — `CHECK` is trivially mutable.
5. Pick `ON DELETE` behavior explicitly and justify non-obvious choices in
   a comment. Defaults we use:
   - `providers` → child tables: `ON DELETE CASCADE` (tenant purge)
   - `users` → audit rows: `ON DELETE SET NULL` (preserve audit trail)
   - `policy_versions` → acceptances/signatures: `ON DELETE RESTRICT`
     (never lose legal records)
6. Add every index the new table needs in the same migration. A table
   landing in prod without its indexes causes production incidents.

## Commands

```bash
# Create a new migration (Makefile shortcut)
make migrate-new name=add_foo_bar

# Apply all pending migrations
make migrate-up

# Roll back one migration
make migrate-down

# Show current version
make migrate-version

# Force a dirty state to a known version (emergency only!)
make migrate-force version=N
```

Under the hood these call `migrate -database "$DATABASE_URL" -path ./migrations`.

## Rollback policy

- **Local/dev:** roll back freely, redo often.
- **Staging:** roll back is expected during the PR cycle.
- **Production:** we roll **forward**, not back. A bad migration gets a new
  migration that undoes it (`000042_revert_bad_change.up.sql`). Down migrations
  exist only for local dev ergonomics and for the (rare) emergency where we
  haven't yet committed real data using the new schema.
- If you think you need to run `migrate down` in production, stop and page
  the on-call engineer. Data loss is much more likely than you think.

## Zero-downtime migration rules

When the migration will run against a live production database:

- **Add columns** as `NULL` or with a constant default, never a computed one.
- **Backfill** large columns in batched `UPDATE`s from a one-off script, not
  inside the migration.
- **Create indexes** with `CREATE INDEX CONCURRENTLY` for tables > ~1M rows
  (note: cannot run inside a transaction; use a dedicated migration file
  without `BEGIN`/`COMMIT` wrapping).
- **Rename** a column in three steps: add new, dual-write in app, drop old.
  Never rename in one shot in a running system.
- **Drop** a column only after one full release cycle with the code no longer
  referencing it.

## The built-in helper

`000001` installs a `set_updated_at()` trigger function. Reuse it:

```sql
CREATE TRIGGER my_table_set_updated_at
BEFORE UPDATE ON my_table
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```
