package scanner

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/listenupapp/listenup-server/internal/scanner/audio"
)

// Analyzer analyzes audio files and extracts metadata
type Analyzer struct {
	logger *slog.Logger
	parser audio.Parser
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(logger *slog.Logger) *Analyzer {
	return &Analyzer{
		logger: logger,
		parser: audio.NewNativeParser(),
	}
}

// AnalyzeOptions configures analysis behavior
type AnalyzeOptions struct {
	// Number of concurrent workers
	Workers int

	// Skip files that haven't changed (based on modtime/size
	UseCache bool
}

func (a *Analyzer) Analyze(ctx context.Context, files []AudioFileData, opts AnalyzeOptions) ([]AudioFileData, error) {
	// Handle empty input
	if len(files) == 0 {
		return []AudioFileData{}, nil
	}

	// Default workers to runtime.NumCPU() if not specified
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Create job and result channels
	type job struct {
		file  AudioFileData
		index int
	}

	type result struct {
		file  AudioFileData
		index int
		err   error
	}

	jobs := make(chan job, len(files))
	results := make(chan result, len(files))

	// Start workers
	for i := 0; i < workers; i++ {
		go func() {
			for j := range jobs {
				// Check context cancellation
				select {
				case <-ctx.Done():
					results <- result{file: j.file, index: j.index, err: ctx.Err()}
					return
				default:
				}

				// Parse metadata
				metadata, err := a.parser.Parse(ctx, j.file.Path)
				if err != nil {
					a.logger.Error("failed to parse file", "path", j.file.Path, "error", err)
					// Continue without metadata rather than failing
					results <- result{file: j.file, index: j.index, err: err}
					continue
				}

				// Convert audio.Metadata to AudioMetadata
				j.file.Metadata = convertMetadata(metadata)
				results <- result{file: j.file, index: j.index}
			}
		}()
	}

	// Send jobs
	for i, file := range files {
		select {
		case jobs <- job{file: file, index: i}:
		case <-ctx.Done():
			close(jobs)
			return nil, ctx.Err()
		}
	}
	close(jobs)

	// Collect results
	parsedFiles := make([]AudioFileData, len(files))
	var firstErr error

	for i := 0; i < len(files); i++ {
		select {
		case r := <-results:
			parsedFiles[r.index] = r.file
			if r.err != nil && firstErr == nil {
				// Check if it's a context error (should fail fast)
				if r.err == context.Canceled || r.err == context.DeadlineExceeded {
					return nil, r.err
				}
				// Otherwise just log and continue
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return parsedFiles, firstErr
}

// convertMetadata converts audio.Metadata to AudioMetadata
func convertMetadata(src *audio.Metadata) *AudioMetadata {
	if src == nil {
		return nil
	}

	dst := &AudioMetadata{
		Format:      src.Format,
		Duration:    src.Duration,
		Bitrate:     src.Bitrate,
		SampleRate:  src.SampleRate,
		Channels:    src.Channels,
		Codec:       src.Codec,
		Title:       src.Title,
		Album:       src.Album,
		Artist:      src.Artist,
		AlbumArtist: src.AlbumArtist,
		Composer:    src.Composer,
		Genre:       src.Genre,
		Year:        src.Year,
		Track:       src.Track,
		TrackTotal:  src.TrackTotal,
		Disc:        src.Disc,
		DiscTotal:   src.DiscTotal,
		Narrator:    src.Narrator,
		Publisher:   src.Publisher,
		Description: src.Description,
		Subtitle:    src.Subtitle,
		Series:      src.Series,
		SeriesPart:  src.SeriesPart,
		ISBN:        src.ISBN,
		ASIN:        src.ASIN,
		Language:    src.Language,
		HasCover:    src.HasCover,
		CoverMIME:   src.CoverMIME,
	}

	// Convert chapters
	for _, ch := range src.Chapters {
		dst.Chapters = append(dst.Chapters, Chapter{
			ID:        ch.Index,
			Title:     ch.Title,
			StartTime: ch.StartTime,
			EndTime:   ch.EndTime,
		})
	}

	return dst
}
