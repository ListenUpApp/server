package backupimport

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// fakeEntity is a minimal Syncable-shaped value used to exercise importEntity
// without dragging in the real store. Tests below stream these through
// importEntity and assert the helper's framing behavior (file lookup, parse
// errors, dry-run, soft-delete short-circuit, persist outcome routing).
type fakeEntity struct {
	domain.Syncable
	Name string `json:"name"`
}

// writeZip builds an in-memory zip containing the given files (path -> body)
// and writes it to a temp file, returning a *zip.ReadCloser the helpers can
// consume. The caller is responsible for closing the returned reader.
func writeZip(t *testing.T, files map[string]string) *zip.ReadCloser {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.zip")
	f, err := os.Create(path)
	require.NoError(t, err)

	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.Copy(w, bytes.NewReader([]byte(body)))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = zr.Close() })
	return zr
}

func newTestImporter() *Importer {
	return &Importer{
		logger: slog.New(slog.DiscardHandler),
	}
}

// TestImportEntity_FileNotFound is the case the prior per-entity importers
// all handled identically (and silently). Now that they share one helper, we
// pin the contract: a missing zip entry is not an error.
func TestImportEntity_FileNotFound(t *testing.T) {
	zr := writeZip(t, map[string]string{
		"other.jsonl": "",
	})

	imported, skipped, errs := importEntity(
		context.Background(),
		newTestImporter(),
		zr,
		RestoreOptions{Mode: RestoreModeFull},
		"entities/missing.jsonl",
		"missing",
		func(e *fakeEntity) string { return e.ID },
		nil,
		func(_ context.Context, _ *fakeEntity) persistOutcome {
			t.Fatal("persist must not be called when file is absent")
			return persistOutcome{}
		},
	)

	assert.Equal(t, 0, imported)
	assert.Equal(t, 0, skipped)
	assert.Empty(t, errs)
}

// TestImportEntity_DryRun ensures the persist callback is bypassed in dry-run
// mode but each well-formed entity still counts toward "imported".
func TestImportEntity_DryRun(t *testing.T) {
	zr := writeZip(t, map[string]string{
		"entities/fakes.jsonl": `{"id":"a","name":"x"}` + "\n" + `{"id":"b","name":"y"}` + "\n",
	})

	imported, skipped, errs := importEntity(
		context.Background(),
		newTestImporter(),
		zr,
		RestoreOptions{Mode: RestoreModeFull, DryRun: true},
		"entities/fakes.jsonl",
		"fakes",
		func(e *fakeEntity) string { return e.ID },
		nil,
		func(_ context.Context, _ *fakeEntity) persistOutcome {
			t.Fatal("persist must not be called in dry-run mode")
			return persistOutcome{}
		},
	)

	assert.Equal(t, 2, imported)
	assert.Equal(t, 0, skipped)
	assert.Empty(t, errs)
}

// TestImportEntity_SoftDeletedSkippedInMerge verifies the soft-delete short
// circuit fires only in merge mode and only when the isDeleted callback is
// supplied.
func TestImportEntity_SoftDeletedSkippedInMerge(t *testing.T) {
	zr := writeZip(t, map[string]string{
		"entities/fakes.jsonl": `{"id":"alive","name":"x"}` + "\n" + `{"id":"dead","name":"y"}` + "\n",
	})

	called := 0
	imported, skipped, errs := importEntity(
		context.Background(),
		newTestImporter(),
		zr,
		RestoreOptions{Mode: RestoreModeMerge, MergeStrategy: MergeKeepBackup},
		"entities/fakes.jsonl",
		"fakes",
		func(e *fakeEntity) string { return e.ID },
		func(e *fakeEntity) bool { return e.ID == "dead" },
		func(_ context.Context, e *fakeEntity) persistOutcome {
			called++
			assert.Equal(t, "alive", e.ID, "only the non-deleted entity should reach persist")
			return persistOutcome{}
		},
	)

	assert.Equal(t, 1, imported)
	assert.Equal(t, 1, skipped)
	assert.Empty(t, errs)
	assert.Equal(t, 1, called)
}

// TestImportEntity_PersistOutcomes covers all three branches of persistOutcome
// in a single pass: imported, skipped, and error-with-id.
func TestImportEntity_PersistOutcomes(t *testing.T) {
	zr := writeZip(t, map[string]string{
		"entities/fakes.jsonl": `{"id":"ok","name":"x"}` + "\n" +
			`{"id":"skip","name":"y"}` + "\n" +
			`{"id":"boom","name":"z"}` + "\n",
	})

	imported, skipped, errs := importEntity(
		context.Background(),
		newTestImporter(),
		zr,
		RestoreOptions{Mode: RestoreModeFull},
		"entities/fakes.jsonl",
		"fakes",
		func(e *fakeEntity) string { return e.ID },
		nil,
		func(_ context.Context, e *fakeEntity) persistOutcome {
			switch e.ID {
			case "ok":
				return persistOutcome{}
			case "skip":
				return persistOutcome{skipped: true}
			case "boom":
				return persistOutcome{err: errors.New("write failed")}
			}
			t.Fatalf("unexpected id %q", e.ID)
			return persistOutcome{}
		},
	)

	assert.Equal(t, 1, imported)
	assert.Equal(t, 1, skipped)
	require.Len(t, errs, 1)
	assert.Equal(t, "fakes", errs[0].EntityType)
	assert.Equal(t, "boom", errs[0].EntityID)
	assert.Equal(t, "write failed", errs[0].Error)
}

// TestImportEntity_ParseErrorsRecorded verifies that malformed lines emit a
// RestoreError with no EntityID (we don't have a parsed entity yet) and that
// streaming continues on subsequent valid lines.
func TestImportEntity_ParseErrorsRecorded(t *testing.T) {
	zr := writeZip(t, map[string]string{
		"entities/fakes.jsonl": `not-json` + "\n" + `{"id":"ok","name":"x"}` + "\n",
	})

	imported, _, errs := importEntity(
		context.Background(),
		newTestImporter(),
		zr,
		RestoreOptions{Mode: RestoreModeFull},
		"entities/fakes.jsonl",
		"fakes",
		func(e *fakeEntity) string { return e.ID },
		nil,
		func(_ context.Context, _ *fakeEntity) persistOutcome { return persistOutcome{} },
	)

	assert.Equal(t, 1, imported)
	require.Len(t, errs, 1)
	assert.Equal(t, "fakes", errs[0].EntityType)
	assert.Empty(t, errs[0].EntityID)
	assert.Contains(t, errs[0].Error, "parse error")
}

// TestApplyMergeStrategy pins the small decision table that decides whether
// an existing local entity gets overwritten on a merge restore.
func TestApplyMergeStrategy(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	cases := []struct {
		name     string
		opts     RestoreOptions
		backup   time.Time
		existing time.Time
		want     mergeVerdict
	}{
		{"full mode always updates", RestoreOptions{Mode: RestoreModeFull}, earlier, now, mergeVerdictUpdate},
		{"keep_local skips", RestoreOptions{Mode: RestoreModeMerge, MergeStrategy: MergeKeepLocal}, now, earlier, mergeVerdictSkip},
		{"keep_backup updates", RestoreOptions{Mode: RestoreModeMerge, MergeStrategy: MergeKeepBackup}, earlier, now, mergeVerdictUpdate},
		{"newest: backup newer updates", RestoreOptions{Mode: RestoreModeMerge, MergeStrategy: MergeNewest}, now, earlier, mergeVerdictUpdate},
		{"newest: backup older skips", RestoreOptions{Mode: RestoreModeMerge, MergeStrategy: MergeNewest}, earlier, now, mergeVerdictSkip},
		{"newest: equal skips", RestoreOptions{Mode: RestoreModeMerge, MergeStrategy: MergeNewest}, now, now, mergeVerdictSkip},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyMergeStrategy(tc.opts, tc.backup, tc.existing)
			assert.Equal(t, tc.want, got)
		})
	}
}
