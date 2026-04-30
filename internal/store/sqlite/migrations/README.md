# Database migrations

This directory holds SQL migration files applied via [goose v3].

## Adding a migration

```bash
go run ./cmd/migrate create <descriptive_name>
```

This generates `<UTC-timestamp>_<descriptive_name>.sql` with `-- +goose Up`
and `-- +goose Down` sections. Edit both, then either boot the server
(migrations apply automatically on `Open()`) or run:

```bash
go run ./cmd/migrate up
```

## Filename convention

`YYYYMMDDHHMMSS_<descriptive_name>.sql` — UTC timestamp prefix. Timestamps
avoid merge conflicts when two contributors author migrations in parallel.
The generator above produces them; don't hand-roll filenames.

## SQLite ALTER limitations

SQLite supports `ADD COLUMN`, `RENAME COLUMN`, `RENAME TABLE`, and (3.35+)
`DROP COLUMN`. It does **not** support changing column types, defaults, or
constraints in place. For those, use the table-rebuild recipe:

```sql
-- +goose Up
CREATE TABLE foo_new (
    -- new column definitions
);
INSERT INTO foo_new SELECT ... FROM foo;
DROP TABLE foo;
ALTER TABLE foo_new RENAME TO foo;
-- recreate any indexes that were on foo
```

Goose runs each migration in a transaction by default, so a failure
mid-recipe leaves the previous schema intact.

## Inspecting a database's version

```bash
go run ./cmd/migrate status     # human-readable table
go run ./cmd/migrate version    # current applied version number
```

Or directly:

```bash
sqlite3 listenup.db "SELECT * FROM goose_db_version"
```

## Why the baseline migration's Down is a no-op

`20260430000000_init.sql` is the schema baseline. Its `-- +goose Down`
section is intentionally `SELECT 1;` — `goose down` from the baseline
would otherwise drop every table, which is the kind of destruction a
one-key mistake shouldn't trigger. Recovery from a corrupted database
is "delete the file and let the server re-create it on next boot."

[goose v3]: https://github.com/pressly/goose
