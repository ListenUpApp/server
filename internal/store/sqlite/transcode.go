package sqlite

import (
	"context"
	"database/sql"
	"iter"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// transcodeJobColumns is the ordered list of columns selected in transcode job queries.
// Must match the scan order in scanTranscodeJob.
const transcodeJobColumns = `id, book_id, audio_file_id,
	source_path, source_codec, source_hash,
	output_path, output_codec, output_size,
	variant, status, progress, priority, error,
	created_at, started_at, completed_at`

// scanTranscodeJob scans a sql.Row (or sql.Rows via its Scan method) into a domain.TranscodeJob.
func scanTranscodeJob(scanner interface{ Scan(dest ...any) error }) (*domain.TranscodeJob, error) {
	var j domain.TranscodeJob

	var (
		createdAt   string
		startedAt   sql.NullString
		completedAt sql.NullString
	)

	err := scanner.Scan(
		&j.ID,
		&j.BookID,
		&j.AudioFileID,
		&j.SourcePath,
		&j.SourceCodec,
		&j.SourceHash,
		&j.OutputPath,
		&j.OutputCodec,
		&j.OutputSize,
		&j.Variant,
		&j.Status,
		&j.Progress,
		&j.Priority,
		&j.Error,
		&createdAt,
		&startedAt,
		&completedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	j.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	j.StartedAt, err = parseNullableTime(startedAt)
	if err != nil {
		return nil, err
	}
	j.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return nil, err
	}

	return &j, nil
}

// CreateTranscodeJob inserts a new transcode job into the database.
// Returns store.ErrAlreadyExists on duplicate ID or (audio_file_id, variant) pair.
func (s *Store) CreateTranscodeJob(ctx context.Context, job *domain.TranscodeJob) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO transcode_jobs (
			id, book_id, audio_file_id,
			source_path, source_codec, source_hash,
			output_path, output_codec, output_size,
			variant, status, progress, priority, error,
			created_at, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID,
		job.BookID,
		job.AudioFileID,
		job.SourcePath,
		job.SourceCodec,
		job.SourceHash,
		job.OutputPath,
		job.OutputCodec,
		job.OutputSize,
		string(job.Variant),
		string(job.Status),
		job.Progress,
		job.Priority,
		job.Error,
		formatTime(job.CreatedAt),
		nullTimeString(job.StartedAt),
		nullTimeString(job.CompletedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetTranscodeJob retrieves a transcode job by ID.
// Returns store.ErrNotFound if the job does not exist.
func (s *Store) GetTranscodeJob(ctx context.Context, id string) (*domain.TranscodeJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+transcodeJobColumns+` FROM transcode_jobs WHERE id = ?`, id)

	job, err := scanTranscodeJob(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// UpdateTranscodeJob performs a full row update on an existing transcode job.
// Returns store.ErrNotFound if the job does not exist.
func (s *Store) UpdateTranscodeJob(ctx context.Context, job *domain.TranscodeJob) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE transcode_jobs SET
			book_id = ?,
			audio_file_id = ?,
			source_path = ?,
			source_codec = ?,
			source_hash = ?,
			output_path = ?,
			output_codec = ?,
			output_size = ?,
			variant = ?,
			status = ?,
			progress = ?,
			priority = ?,
			error = ?,
			created_at = ?,
			started_at = ?,
			completed_at = ?
		WHERE id = ?`,
		job.BookID,
		job.AudioFileID,
		job.SourcePath,
		job.SourceCodec,
		job.SourceHash,
		job.OutputPath,
		job.OutputCodec,
		job.OutputSize,
		string(job.Variant),
		string(job.Status),
		job.Progress,
		job.Priority,
		job.Error,
		formatTime(job.CreatedAt),
		nullTimeString(job.StartedAt),
		nullTimeString(job.CompletedAt),
		job.ID,
	)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteTranscodeJob deletes a transcode job by ID.
// Returns store.ErrNotFound if the job does not exist.
func (s *Store) DeleteTranscodeJob(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM transcode_jobs WHERE id = ?`, id)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// GetTranscodeJobByAudioFile retrieves the first transcode job for a given audio file ID.
// Returns store.ErrNotFound if no job exists for the audio file.
func (s *Store) GetTranscodeJobByAudioFile(ctx context.Context, audioFileID string) (*domain.TranscodeJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+transcodeJobColumns+` FROM transcode_jobs WHERE audio_file_id = ? LIMIT 1`,
		audioFileID)

	job, err := scanTranscodeJob(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// GetTranscodeJobByAudioFileAndVariant retrieves a transcode job by audio file ID and variant.
// Returns store.ErrNotFound if no matching job exists.
func (s *Store) GetTranscodeJobByAudioFileAndVariant(ctx context.Context, audioFileID string, variant domain.TranscodeVariant) (*domain.TranscodeJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+transcodeJobColumns+` FROM transcode_jobs
		WHERE audio_file_id = ? AND variant = ?`,
		audioFileID, string(variant))

	job, err := scanTranscodeJob(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// ListTranscodeJobsByBook returns all transcode jobs for a given book, ordered by created_at.
func (s *Store) ListTranscodeJobsByBook(ctx context.Context, bookID string) ([]*domain.TranscodeJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+transcodeJobColumns+` FROM transcode_jobs
		WHERE book_id = ? ORDER BY created_at ASC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*domain.TranscodeJob
	for rows.Next() {
		job, err := scanTranscodeJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// ListTranscodeJobsByStatus returns all transcode jobs with the given status, ordered by priority desc, created_at asc.
func (s *Store) ListTranscodeJobsByStatus(ctx context.Context, status domain.TranscodeStatus) ([]*domain.TranscodeJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+transcodeJobColumns+` FROM transcode_jobs
		WHERE status = ? ORDER BY priority DESC, created_at ASC`,
		string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*domain.TranscodeJob
	for rows.Next() {
		job, err := scanTranscodeJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// ListPendingTranscodeJobs returns all transcode jobs with status "pending",
// ordered by priority desc, created_at asc.
func (s *Store) ListPendingTranscodeJobs(ctx context.Context) ([]*domain.TranscodeJob, error) {
	return s.ListTranscodeJobsByStatus(ctx, domain.TranscodeStatusPending)
}

// ListAllTranscodeJobs returns an iterator over all transcode jobs, ordered by created_at.
func (s *Store) ListAllTranscodeJobs(ctx context.Context) iter.Seq2[*domain.TranscodeJob, error] {
	return func(yield func(*domain.TranscodeJob, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+transcodeJobColumns+` FROM transcode_jobs ORDER BY created_at ASC`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			job, err := scanTranscodeJob(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(job, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// DeleteTranscodeJobsByBook deletes all transcode jobs for a given book
// and returns the number of jobs deleted.
func (s *Store) DeleteTranscodeJobsByBook(ctx context.Context, bookID string) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM transcode_jobs WHERE book_id = ?`, bookID)
	if err != nil {
		return 0, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
