// Package scanner provides functionality for discovering, analyzing, and cataloging audiobook files.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/listenupapp/listenup-server/internal/scanner/audio"
)

// Analyzer analyzes audio files and extracts metadata.
type Analyzer struct {
	logger *slog.Logger
	parser audio.Parser
}

// NewAnalyzer creates a new analyzer.
func NewAnalyzer(logger *slog.Logger) *Analyzer {
	return &Analyzer{
		logger: logger,
		parser: audio.NewNativeParser(),
	}
}

// AnalyzeOptions configures analysis behavior.
type AnalyzeOptions struct {
	// Number of concurrent workers.
	Workers int

	// Skip files that haven't changed (based on modtime/size.
	UseCache bool
}

// Analyze analyzes audio files and extracts metadata concurrently.
//
//nolint:gocyclo // Acceptable complexity for concurrent worker pool coordination
func (a *Analyzer) Analyze(ctx context.Context, files []AudioFileData, opts AnalyzeOptions) ([]AudioFileData, error) {
	// Handle empty input.
	if len(files) == 0 {
		return []AudioFileData{}, nil
	}

	// Default workers to runtime.NumCPU() if not specified.
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Create job and result channels.
	type job struct {
		file  AudioFileData
		index int
	}

	type result struct {
		err   error
		file  AudioFileData
		index int
	}

	jobs := make(chan job, len(files))
	results := make(chan result, len(files))

	// Start workers.
	for range workers {
		go func() {
			for j := range jobs {
				// Check context cancellation.
				select {
				case <-ctx.Done():
					results <- result{file: j.file, index: j.index, err: ctx.Err()}
					return
				default:
				}

				// Parse metadata.
				metadata, err := a.parser.Parse(ctx, j.file.Path)
				if err != nil {
					a.logger.Error("failed to parse file", "path", j.file.Path, "error", err)
					// Continue without metadata rather than failing.
					results <- result{file: j.file, index: j.index, err: err}
					continue
				}

				// Convert audio.Metadata to AudioMetadata.
				j.file.Metadata = convertMetadata(metadata)
				results <- result{file: j.file, index: j.index}
			}
		}()
	}

	// Send jobs.
	for i, file := range files {
		select {
		case jobs <- job{file: file, index: i}:
		case <-ctx.Done():
			close(jobs)
			return nil, ctx.Err()
		}
	}
	close(jobs)

	// Collect results.
	parsedFiles := make([]AudioFileData, len(files))

	for range len(files) {
		select {
		case r := <-results:
			parsedFiles[r.index] = r.file
			// Check if it's a context error (should fail fast).
			if r.err != nil && (errors.Is(r.err, context.Canceled) || errors.Is(r.err, context.DeadlineExceeded)) {
				return nil, r.err
			}
			// Otherwise individual file errors are logged by workers and gracefully ignored.
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return parsedFiles, nil
}

// ItemType represents the type of library item.
type ItemType int

// ItemType constants define the different types of audiobook items.
const (
	// ItemTypeSingleFile represents a single audio file (M4B or single MP3).
	ItemTypeSingleFile ItemType = iota
	// ItemTypeMultiFile represents multiple files (MP3 album/audiobook).
	ItemTypeMultiFile
)

// AnalyzeItems analyzes library items with multi-file classification.
func (a *Analyzer) AnalyzeItems(ctx context.Context, items []LibraryItemData) ([]LibraryItemData, error) {
	if len(items) == 0 {
		return []LibraryItemData{}, nil
	}

	// Process each item.
	results := make([]LibraryItemData, len(items))

	for i := range items {
		item := &items[i]
		// Classify item.
		itemType := classifyItem(*item)

		// Analyze based on type.
		switch itemType {
		case ItemTypeSingleFile:
			// Single file - parse it.
			if len(item.AudioFiles) > 0 {
				metadata, parseErr := a.parser.Parse(ctx, item.AudioFiles[0].Path)
				if parseErr != nil {
					a.logger.Error("failed to parse single file",
						"path", item.AudioFiles[0].Path,
						"error", parseErr)
				} else {
					item.AudioFiles[0].Metadata = convertMetadata(metadata)
					// Build item-level BookMetadata from audio metadata.
					item.Metadata = buildBookMetadata(item.AudioFiles[0].Metadata)
				}
			}

		case ItemTypeMultiFile:
			// Multiple files - aggregate them.
			paths := make([]string, len(item.AudioFiles))
			for j, file := range item.AudioFiles {
				paths[j] = file.Path
			}

			metadata, parseErr := a.parser.ParseMultiFile(ctx, paths)
			if parseErr != nil {
				a.logger.Error("failed to parse multi-file item",
					"path", item.Path,
					"files", len(paths),
					"error", parseErr)
			} else if len(item.AudioFiles) > 0 {
				// Store aggregated metadata in first file.
				item.AudioFiles[0].Metadata = convertMetadata(metadata)
				// Build item-level BookMetadata from aggregated audio metadata.
				item.Metadata = buildBookMetadata(item.AudioFiles[0].Metadata)
			}
		}

		results[i] = *item

		// Check for context cancellation.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return results, nil
}

// classifyItem determines if an item is single-file or multi-file.
func classifyItem(item LibraryItemData) ItemType {
	audioCount := len(item.AudioFiles)

	// No audio files or single file.
	if audioCount <= 1 {
		return ItemTypeSingleFile
	}

	// Check file types.
	hasMP3 := false
	hasM4B := false

	for _, file := range item.AudioFiles {
		ext := strings.ToLower(filepath.Ext(file.Path))
		switch ext {
		case ".mp3":
			hasMP3 = true
		case ".m4b", ".m4a":
			hasM4B = true
		}
	}

	// Multiple MP3 files = multi-file audiobook/album.
	if hasMP3 && audioCount > 1 {
		return ItemTypeMultiFile
	}

	// Multiple M4B files = error condition, but treat as single file.
	if hasM4B && audioCount > 1 {
		// Log warning?
		return ItemTypeSingleFile
	}

	return ItemTypeSingleFile
}

// convertMetadata converts audio.Metadata to AudioMetadata.
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

	// Convert chapters.
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

// splitContributors splits a contributor string by semicolons.
// Handles patterns like "Homer; Emily Wilson - translator".
func splitContributors(input string) []string {
	var result []string
	parts := strings.Split(input, ";")
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// resolveBookTitle determines the correct book title based on format and available metadata.
//
// For M4B/M4A/MP4 files (single-file audiobooks):
//   - The Title tag contains the book title
//   - Chapter information is stored in chapter atoms, not the title
//
// For MP3 albums (multi-file audiobooks):
//   - The Title tag contains the track/chapter name
//   - The Album tag contains the book title
//
// Falls back to Album if Title is empty, or Title if Album is empty.
func resolveBookTitle(audioMeta *AudioMetadata) string {
	format := strings.ToLower(audioMeta.Format)

	// M4B, M4A, MP4 formats: prefer Title tag
	if format == "m4b" || format == "m4a" || format == "mp4" {
		if audioMeta.Title != "" {
			return audioMeta.Title
		}
		return audioMeta.Album
	}

	// MP3 and other formats: prefer Album tag (Title is track name)
	if audioMeta.Album != "" {
		return audioMeta.Album
	}
	return audioMeta.Title
}

// sanitizeString removes null bytes and other control characters that can cause
// issues in databases and JSON parsing. Some audio metadata parsers include
// null terminators in strings.
func sanitizeString(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 0 { // null byte
			return -1 // drop it
		}
		return r
	}, s)
}

// iso639_2to1 maps ISO 639-2 (3-letter) codes to ISO 639-1 (2-letter) codes.
//
//nolint:gochecknoglobals // Static lookup table for language normalization
var iso639_2to1 = map[string]string{
	"eng": "en", "spa": "es", "fra": "fr", "deu": "de", "ita": "it",
	"por": "pt", "nld": "nl", "rus": "ru", "jpn": "ja", "zho": "zh",
	"kor": "ko", "ara": "ar", "hin": "hi", "pol": "pl", "swe": "sv",
	"nor": "no", "dan": "da", "fin": "fi", "tur": "tr", "ell": "el",
	"heb": "he", "ces": "cs", "hun": "hu", "ron": "ro", "tha": "th",
	"vie": "vi", "ind": "id", "msa": "ms", "ukr": "uk", "cat": "ca",
	"hrv": "hr", "slk": "sk", "bul": "bg", "lit": "lt", "lav": "lv",
	"est": "et", "slv": "sl", "srp": "sr", "fas": "fa", "ben": "bn",
	"tam": "ta", "tel": "te", "mar": "mr", "guj": "gu", "kan": "kn",
	"mal": "ml", "pan": "pa", "urd": "ur", "nep": "ne", "sin": "si",
	"mya": "my", "khm": "km", "lao": "lo", "amh": "am", "swa": "sw",
	"afr": "af", "zul": "zu", "xho": "xh", "hau": "ha", "yor": "yo",
	"ibo": "ig", "cym": "cy", "gle": "ga", "gla": "gd", "eus": "eu",
	"glg": "gl", "isl": "is", "mkd": "mk", "bos": "bs", "sqi": "sq",
	"hye": "hy", "kat": "ka", "kaz": "kk", "uzb": "uz", "azj": "az",
	"mon": "mn", "tgl": "tl", "fil": "tl", "jav": "jv", "sun": "su",
	// Alternative ISO 639-2/B codes (bibliographic)
	"ger": "de", "fre": "fr", "dut": "nl", "chi": "zh", "cze": "cs",
	"gre": "el", "per": "fa", "rum": "ro", "slo": "sk", "alb": "sq",
	"arm": "hy", "baq": "eu", "bur": "my", "geo": "ka", "ice": "is",
	"mac": "mk", "may": "ms", "tib": "bo", "wel": "cy",
}

// languageNameToCode maps common language names to ISO 639-1 codes.
//
//nolint:gochecknoglobals // Static lookup table for language normalization
var languageNameToCode = map[string]string{
	"english": "en", "spanish": "es", "french": "fr", "german": "de",
	"italian": "it", "portuguese": "pt", "dutch": "nl", "russian": "ru",
	"japanese": "ja", "chinese": "zh", "korean": "ko", "arabic": "ar",
	"hindi": "hi", "polish": "pl", "swedish": "sv", "norwegian": "no",
	"danish": "da", "finnish": "fi", "turkish": "tr", "greek": "el",
	"hebrew": "he", "czech": "cs", "hungarian": "hu", "romanian": "ro",
	"thai": "th", "vietnamese": "vi", "indonesian": "id", "malay": "ms",
	"ukrainian": "uk", "catalan": "ca", "croatian": "hr", "slovak": "sk",
	"bulgarian": "bg", "lithuanian": "lt", "latvian": "lv", "estonian": "et",
	"slovenian": "sl", "serbian": "sr", "persian": "fa", "farsi": "fa",
	"bengali": "bn", "tamil": "ta", "telugu": "te", "marathi": "mr",
	"gujarati": "gu", "kannada": "kn", "malayalam": "ml", "punjabi": "pa",
	"urdu": "ur", "nepali": "ne", "sinhala": "si", "burmese": "my",
	"khmer": "km", "lao": "lo", "amharic": "am", "swahili": "sw",
	"afrikaans": "af", "zulu": "zu", "xhosa": "xh", "hausa": "ha",
	"yoruba": "yo", "igbo": "ig", "welsh": "cy", "irish": "ga",
	"scottish gaelic": "gd", "basque": "eu", "galician": "gl", "icelandic": "is",
	"macedonian": "mk", "bosnian": "bs", "albanian": "sq", "armenian": "hy",
	"georgian": "ka", "kazakh": "kk", "uzbek": "uz", "azerbaijani": "az",
	"mongolian": "mn", "tagalog": "tl", "filipino": "tl", "javanese": "jv",
	"sundanese": "su", "mandarin": "zh", "cantonese": "zh", "tibetan": "bo",
}

// validISO639_1 contains all valid ISO 639-1 codes for validation.
//
//nolint:gochecknoglobals // Static lookup table for language normalization
var validISO639_1 = map[string]bool{
	"aa": true, "ab": true, "ae": true, "af": true, "ak": true, "am": true,
	"an": true, "ar": true, "as": true, "av": true, "ay": true, "az": true,
	"ba": true, "be": true, "bg": true, "bh": true, "bi": true, "bm": true,
	"bn": true, "bo": true, "br": true, "bs": true, "ca": true, "ce": true,
	"ch": true, "co": true, "cr": true, "cs": true, "cu": true, "cv": true,
	"cy": true, "da": true, "de": true, "dv": true, "dz": true, "ee": true,
	"el": true, "en": true, "eo": true, "es": true, "et": true, "eu": true,
	"fa": true, "ff": true, "fi": true, "fj": true, "fo": true, "fr": true,
	"fy": true, "ga": true, "gd": true, "gl": true, "gn": true, "gu": true,
	"gv": true, "ha": true, "he": true, "hi": true, "ho": true, "hr": true,
	"ht": true, "hu": true, "hy": true, "hz": true, "ia": true, "id": true,
	"ie": true, "ig": true, "ii": true, "ik": true, "io": true, "is": true,
	"it": true, "iu": true, "ja": true, "jv": true, "ka": true, "kg": true,
	"ki": true, "kj": true, "kk": true, "kl": true, "km": true, "kn": true,
	"ko": true, "kr": true, "ks": true, "ku": true, "kv": true, "kw": true,
	"ky": true, "la": true, "lb": true, "lg": true, "li": true, "ln": true,
	"lo": true, "lt": true, "lu": true, "lv": true, "mg": true, "mh": true,
	"mi": true, "mk": true, "ml": true, "mn": true, "mr": true, "ms": true,
	"mt": true, "my": true, "na": true, "nb": true, "nd": true, "ne": true,
	"ng": true, "nl": true, "nn": true, "no": true, "nr": true, "nv": true,
	"ny": true, "oc": true, "oj": true, "om": true, "or": true, "os": true,
	"pa": true, "pi": true, "pl": true, "ps": true, "pt": true, "qu": true,
	"rm": true, "rn": true, "ro": true, "ru": true, "rw": true, "sa": true,
	"sc": true, "sd": true, "se": true, "sg": true, "si": true, "sk": true,
	"sl": true, "sm": true, "sn": true, "so": true, "sq": true, "sr": true,
	"ss": true, "st": true, "su": true, "sv": true, "sw": true, "ta": true,
	"te": true, "tg": true, "th": true, "ti": true, "tk": true, "tl": true,
	"tn": true, "to": true, "tr": true, "ts": true, "tt": true, "tw": true,
	"ty": true, "ug": true, "uk": true, "ur": true, "uz": true, "ve": true,
	"vi": true, "vo": true, "wa": true, "wo": true, "xh": true, "yi": true,
	"yo": true, "za": true, "zh": true, "zu": true,
}

// normalizeLanguage converts various language representations to ISO 639-1 codes.
// It handles:
//   - ISO 639-1 codes: "en" -> "en"
//   - ISO 639-2 codes: "eng" -> "en"
//   - Locale codes: "en-US", "en_GB" -> "en"
//   - Language names: "English", "ENGLISH" -> "en"
//
// Returns empty string for unrecognized values.
func normalizeLanguage(raw string) string {
	if raw == "" {
		return ""
	}

	// Sanitize and normalize case.
	s := strings.ToLower(strings.TrimSpace(sanitizeString(raw)))
	if s == "" {
		return ""
	}

	// Handle locale codes (e.g., "en-US", "en_GB").
	// Split on common separators and use the first part.
	if idx := strings.IndexAny(s, "-_"); idx > 0 {
		s = s[:idx]
	}

	// Check if already valid ISO 639-1.
	if len(s) == 2 && validISO639_1[s] {
		return s
	}

	// Try ISO 639-2 (3-letter) to ISO 639-1 mapping.
	if len(s) == 3 {
		if code, ok := iso639_2to1[s]; ok {
			return code
		}
	}

	// Try language name mapping.
	if code, ok := languageNameToCode[s]; ok {
		return code
	}

	// Unrecognized - return empty.
	return ""
}

// buildBookMetadata converts AudioMetadata to BookMetadata.
// This aggregates audio file metadata into item-level metadata for book creation.
func buildBookMetadata(audioMeta *AudioMetadata) *BookMetadata {
	if audioMeta == nil {
		return nil
	}

	// Determine book title based on format.
	// For M4B/M4A files, the Title tag is the book title (chapters are in chapter atoms).
	// For MP3 albums, the Title tag is the track/chapter name, so use Album instead.
	title := sanitizeString(resolveBookTitle(audioMeta))

	bookMeta := &BookMetadata{
		Title:       title,
		Subtitle:    sanitizeString(audioMeta.Subtitle),
		Description: htmlToMarkdown(sanitizeString(audioMeta.Description)),
		Publisher:   sanitizeString(audioMeta.Publisher),
		Language:    normalizeLanguage(audioMeta.Language),
		ISBN:        sanitizeString(audioMeta.ISBN),
		ASIN:        sanitizeString(audioMeta.ASIN),
		Explicit:    false,
		Abridged:    false,
		Chapters:    audioMeta.Chapters,
	}

	// Convert year to string.
	if audioMeta.Year > 0 {
		bookMeta.PublishYear = fmt.Sprintf("%d", audioMeta.Year)
	}

	// Convert Artist to Authors array (split by semicolon for multiple).
	if audioMeta.Artist != "" {
		bookMeta.Authors = splitContributors(sanitizeString(audioMeta.Artist))
	}

	// Convert Narrator to Narrators array (split by semicolon for multiple).
	if audioMeta.Narrator != "" {
		bookMeta.Narrators = splitContributors(sanitizeString(audioMeta.Narrator))
	}

	// Convert Genre to Genres array (split by semicolon for multiple).
	if audioMeta.Genre != "" {
		genres := strings.Split(sanitizeString(audioMeta.Genre), ";")
		for _, genre := range genres {
			if trimmed := strings.TrimSpace(genre); trimmed != "" {
				bookMeta.Genres = append(bookMeta.Genres, trimmed)
			}
		}
	}

	// Convert Series to SeriesInfo array.
	if audioMeta.Series != "" {
		bookMeta.Series = []SeriesInfo{
			{
				Name:     sanitizeString(audioMeta.Series),
				Sequence: sanitizeString(audioMeta.SeriesPart),
			},
		}
	}

	return bookMeta
}

// BuildBookMetadataExported is an exported version of buildBookMetadata for diagnostic tools.
func BuildBookMetadataExported(audioMeta *AudioMetadata) *BookMetadata {
	return buildBookMetadata(audioMeta)
}
