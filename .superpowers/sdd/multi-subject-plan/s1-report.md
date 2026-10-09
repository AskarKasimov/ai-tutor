# S1 implementation report

## Files changed

- `services/backend/internal/shared/postgres/migrations/00004_subjects.sql`
- `services/backend/internal/shared/postgres/migrate_test.go`

## Schema choices

- Added stable subject ID `subject:intro-to-ml` with display name `Введение в ML`.
- Added nullable `subjects.active_revision`, referencing the globally unique `competency_map_imports.revision`. The existing global revision counter remains in `competency_map_state`.
- Associated existing imports with ML and replaced the global singleton uniqueness constraint on imports with one import per subject.
- Added `subject_id` and `subject_name_snapshot` to variants and backfilled existing variants to ML. Existing task/profile snapshots are untouched.
- `subject_id` defaults to the ML ID on imports and variants to preserve the current singleton API through S2/S3. These are transitional compatibility defaults; S2/S3 must remove implicit subject selection for future operations.
- No query text changed, so SQLC generation was not needed.

## Migration regression test

Added `TestSubjectsMigrationBackfillsCurrentMapAndVariants`. It applies migrations 00001–00003 in a fresh isolated test schema, seeds an active import and historical variant, then applies the full migration set and verifies ML backfill, active revision, unchanged task snapshot, global revision, and inserting a second distinct subject without changing ML.

The test was run against the old schema before adding migration 00004 and failed as expected because `subjects` did not exist. It passed after the migration was added.

## Commands and results

- `TEST_DATABASE_URL='postgres://ai_tutor_test:local_test_only@127.0.0.1:50277/ai_tutor_test?sslmode=disable' go test ./internal/shared/postgres -run TestSubjectsMigrationBackfillsCurrentMapAndVariants -count=1` — passed.
- `TEST_DATABASE_URL='postgres://ai_tutor_test:local_test_only@127.0.0.1:50277/ai_tutor_test?sslmode=disable' go test ./...` — passed across backend packages with integration tests enabled.
- `go vet ./...` — passed.
- `git diff --check` — passed.
- `bash scripts/generate-sql.sh` — not run because no SQL queries changed.

## Concerns

Transitional ML defaults intentionally keep current imports and variant creation working until S2/S3. Those stages must explicitly remove the defaults when subject-scoped behavior is implemented; leaving them would silently route omitted subject IDs to ML.
