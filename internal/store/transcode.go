package store

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const transcodePrefix = "transcode:"

// CreateTranscodeJob creates a new transcode job.
// Returns ErrAlreadyExists if a job with this ID already exists.
func (s *Store) CreateTranscodeJob(ctx context.Context, job *domain.TranscodeJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal transcode job: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := transcodePrefix + job.ID

		// Check if already exists
		_, err := txn.Get([]byte(key))
		if err == nil {
			return ErrAlreadyExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("check existing: %w", err)
		}

		// Set primary key
		if err := txn.Set([]byte(key), data); err != nil {
			return fmt.Errorf("set job: %w", err)
		}

		// Set indexes
		if err := s.setTranscodeIndexes(txn, job); err != nil {
			return err
		}

		return nil
	})
}

// GetTranscodeJob retrieves a transcode job by ID.
func (s *Store) GetTranscodeJob(ctx context.Context, id string) (*domain.TranscodeJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := buildKey(transcodePrefix, id)
	defer releaseKey(key)

	var job domain.TranscodeJob
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get job: %w", err)
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &job)
		})
	})

	if err != nil {
		return nil, err
	}
	return &job, nil
}

// UpdateTranscodeJob updates an existing transcode job.
func (s *Store) UpdateTranscodeJob(ctx context.Context, job *domain.TranscodeJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal transcode job: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(transcodePrefix + job.ID)

		// Get old job to clean up indexes
		var oldJob domain.TranscodeJob
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get existing: %w", err)
		}

		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &oldJob)
		}); err != nil {
			return fmt.Errorf("unmarshal old job: %w", err)
		}

		// Delete old indexes
		if err := s.deleteTranscodeIndexes(txn, &oldJob); err != nil {
			return err
		}

		// Set new value
		if err := txn.Set(key, data); err != nil {
			return fmt.Errorf("set job: %w", err)
		}

		// Set new indexes
		if err := s.setTranscodeIndexes(txn, job); err != nil {
			return err
		}

		return nil
	})
}

// DeleteTranscodeJob deletes a transcode job by ID.
func (s *Store) DeleteTranscodeJob(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(transcodePrefix + id)

		// Get job to clean up indexes
		var job domain.TranscodeJob
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil // Idempotent
		}
		if err != nil {
			return fmt.Errorf("get job: %w", err)
		}

		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &job)
		}); err != nil {
			return fmt.Errorf("unmarshal job: %w", err)
		}

		// Delete indexes
		if err := s.deleteTranscodeIndexes(txn, &job); err != nil {
			return err
		}

		// Delete primary key
		return txn.Delete(key)
	})
}

// GetTranscodeJobByAudioFile finds a transcode job for a specific audio file.
// Note: This returns the first job found if multiple variants exist.
// Use GetTranscodeJobByAudioFileAndVariant for variant-specific lookups.
func (s *Store) GetTranscodeJobByAudioFile(ctx context.Context, audioFileID string) (*domain.TranscodeJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Use prefix search to find any variant for this audio file
	indexPrefix := []byte(transcodePrefix + "idx:audiofile:" + audioFileID + ":")

	var jobID string
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = indexPrefix
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		it.Seek(indexPrefix)
		if !it.ValidForPrefix(indexPrefix) {
			return ErrNotFound
		}

		return it.Item().Value(func(val []byte) error {
			jobID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return s.GetTranscodeJob(ctx, jobID)
}

// GetTranscodeJobByAudioFileAndVariant finds a transcode job by audio file ID and variant.
func (s *Store) GetTranscodeJobByAudioFileAndVariant(ctx context.Context, audioFileID string, variant domain.TranscodeVariant) (*domain.TranscodeJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build the index key manually since buildIndexKey only takes 3 params
	indexKey := []byte(transcodePrefix + "idx:audiofile:" + audioFileID + ":" + string(variant))

	var jobID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(indexKey)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			jobID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return s.GetTranscodeJob(ctx, jobID)
}

// ListTranscodeJobsByBook returns all transcode jobs for a book.
func (s *Store) ListTranscodeJobsByBook(ctx context.Context, bookID string) ([]*domain.TranscodeJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	indexPrefix := []byte(transcodePrefix + "idx:book:" + bookID)
	var jobs []*domain.TranscodeJob

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = indexPrefix
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(indexPrefix); it.ValidForPrefix(indexPrefix); it.Next() {
			var jobID string
			if err := it.Item().Value(func(val []byte) error {
				jobID = string(val)
				return nil
			}); err != nil {
				return err
			}

			job, err := s.GetTranscodeJob(ctx, jobID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return err
			}
			jobs = append(jobs, job)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// ListTranscodeJobsByStatus returns all jobs with the given status.
func (s *Store) ListTranscodeJobsByStatus(ctx context.Context, status domain.TranscodeStatus) ([]*domain.TranscodeJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	indexPrefix := []byte(transcodePrefix + "idx:status:" + string(status))
	var jobs []*domain.TranscodeJob

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = indexPrefix
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(indexPrefix); it.ValidForPrefix(indexPrefix); it.Next() {
			var jobID string
			if err := it.Item().Value(func(val []byte) error {
				jobID = string(val)
				return nil
			}); err != nil {
				return err
			}

			job, err := s.GetTranscodeJob(ctx, jobID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return err
			}
			jobs = append(jobs, job)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// ListPendingTranscodeJobs returns pending jobs ordered by priority (highest first).
func (s *Store) ListPendingTranscodeJobs(ctx context.Context) ([]*domain.TranscodeJob, error) {
	jobs, err := s.ListTranscodeJobsByStatus(ctx, domain.TranscodeStatusPending)
	if err != nil {
		return nil, err
	}

	// Sort by priority descending (higher priority first)
	slices.SortFunc(jobs, func(a, b *domain.TranscodeJob) int {
		return cmp.Compare(b.Priority, a.Priority)
	})

	return jobs, nil
}

// ListAllTranscodeJobs returns an iterator over all transcode jobs.
func (s *Store) ListAllTranscodeJobs(ctx context.Context) iter.Seq2[*domain.TranscodeJob, error] {
	return func(yield func(*domain.TranscodeJob, error) bool) {
		_ = s.db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = []byte(transcodePrefix)
			opts.PrefetchValues = true

			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek([]byte(transcodePrefix)); it.ValidForPrefix([]byte(transcodePrefix)); it.Next() {
				if ctx.Err() != nil {
					yield(nil, ctx.Err())
					return ctx.Err()
				}

				// Skip index keys
				key := string(it.Item().Key())
				if len(key) > len(transcodePrefix) {
					remainder := key[len(transcodePrefix):]
					if strings.HasPrefix(remainder, "idx:") {
						continue
					}
				}

				var job domain.TranscodeJob
				err := it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &job)
				})

				if err != nil {
					yield(nil, err)
					return err
				}

				if !yield(&job, nil) {
					return nil
				}
			}
			return nil
		})
	}
}

// DeleteTranscodeJobsByBook deletes all transcode jobs for a book.
// Returns the number of jobs deleted.
func (s *Store) DeleteTranscodeJobsByBook(ctx context.Context, bookID string) (int, error) {
	jobs, err := s.ListTranscodeJobsByBook(ctx, bookID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, job := range jobs {
		if err := s.DeleteTranscodeJob(ctx, job.ID); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// Index management helpers

func (s *Store) setTranscodeIndexes(txn *badger.Txn, job *domain.TranscodeJob) error {
	// Index by book ID
	bookKey := transcodePrefix + "idx:book:" + job.BookID + ":" + job.ID
	if err := txn.Set([]byte(bookKey), []byte(job.ID)); err != nil {
		return fmt.Errorf("set book index: %w", err)
	}

	// Index by audio file ID and variant (unique per variant)
	audioKey := transcodePrefix + "idx:audiofile:" + job.AudioFileID + ":" + string(job.Variant)
	if err := txn.Set([]byte(audioKey), []byte(job.ID)); err != nil {
		return fmt.Errorf("set audiofile index: %w", err)
	}

	// Index by status
	statusKey := transcodePrefix + "idx:status:" + string(job.Status) + ":" + job.ID
	if err := txn.Set([]byte(statusKey), []byte(job.ID)); err != nil {
		return fmt.Errorf("set status index: %w", err)
	}

	return nil
}

func (s *Store) deleteTranscodeIndexes(txn *badger.Txn, job *domain.TranscodeJob) error {
	// Delete book index
	bookKey := transcodePrefix + "idx:book:" + job.BookID + ":" + job.ID
	if err := txn.Delete([]byte(bookKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return fmt.Errorf("delete book index: %w", err)
	}

	// Delete audio file index (includes variant)
	audioKey := transcodePrefix + "idx:audiofile:" + job.AudioFileID + ":" + string(job.Variant)
	if err := txn.Delete([]byte(audioKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return fmt.Errorf("delete audiofile index: %w", err)
	}

	// Delete status index
	statusKey := transcodePrefix + "idx:status:" + string(job.Status) + ":" + job.ID
	if err := txn.Delete([]byte(statusKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return fmt.Errorf("delete status index: %w", err)
	}

	return nil
}
