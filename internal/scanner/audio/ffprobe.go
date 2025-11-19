// Package audio provides audio file metadata extraction and parsing utilities.
package audio

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FFprobeParser uses ffprobe to parse audio metadata.
type FFprobeParser struct{}

// NewFFprobeParser creates a new ffprobe parser.
func NewFFprobeParser() *FFprobeParser {
	return &FFprobeParser{}
}

// ParseMultiFile parses the first file in a multi-file audiobook.
func (p *FFprobeParser) ParseMultiFile(ctx context.Context, paths []string) (*Metadata, error) {
	// FFprobe doesn't support multi-file aggregation.
	// Fall back to parsing the first file.
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}
	return p.Parse(ctx, paths[0])
}

// Parse extracts metadata using ffprobe.
func (p *FFprobeParser) Parse(ctx context.Context, path string) (*Metadata, error) {
	// Run ffprobe command.
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_chapters",
		"-show_streams",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse JSON output.
	var ffprobeData ffprobeOutput
	if err := json.Unmarshal(output, &ffprobeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	// Convert to our Metadata structure.
	metadata := &Metadata{}

	// Extract information from different sections.
	p.parseFormatInfo(&ffprobeData.Format, metadata)
	p.parseAudioStream(ffprobeData.Streams, metadata)
	p.parseTags(ffprobeData.Format.Tags, metadata)
	metadata.Chapters = p.parseChapters(ffprobeData.Chapters)

	return metadata, nil
}

// parseFormatInfo extracts format-level information (format name, duration, bitrate).
func (p *FFprobeParser) parseFormatInfo(format *ffprobeFormat, metadata *Metadata) {
	// Extract format name.
	if format.FormatName != "" {
		// Get first format (e.g., "mp3" from "mp3,mp2").
		formats := strings.Split(format.FormatName, ",")
		metadata.Format = formats[0]
	}

	// Parse duration.
	if format.Duration != "" {
		if dur, err := strconv.ParseFloat(format.Duration, 64); err == nil {
			metadata.Duration = time.Duration(dur * float64(time.Second))
		}
	}

	// Parse bitrate.
	if format.BitRate != "" {
		if br, err := strconv.Atoi(format.BitRate); err == nil {
			metadata.Bitrate = br
		}
	}
}

// parseAudioStream extracts audio stream information (codec, sample rate, channels).
func (p *FFprobeParser) parseAudioStream(streams []ffprobeStream, metadata *Metadata) {
	for _, stream := range streams {
		if stream.CodecType == "audio" {
			metadata.Codec = stream.CodecName

			// Parse sample rate.
			if stream.SampleRate != "" {
				if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
					metadata.SampleRate = sr
				}
			}

			metadata.Channels = stream.Channels
			break
		}
	}
}

// parseTags extracts metadata tags (title, artist, year, etc.).
func (p *FFprobeParser) parseTags(tags map[string]string, metadata *Metadata) {
	if tags == nil {
		return
	}

	// Basic string tags.
	metadata.Title = tags["title"]
	metadata.Album = tags["album"]
	metadata.Artist = tags["artist"]
	metadata.AlbumArtist = tags["album_artist"]
	metadata.Composer = tags["composer"]
	metadata.Genre = tags["genre"]
	metadata.Publisher = tags["publisher"]
	metadata.Language = tags["language"]
	metadata.ISBN = tags["isbn"]
	metadata.ASIN = tags["asin"]
	metadata.Subtitle = tags["subtitle"]
	metadata.Series = tags["series"]
	metadata.SeriesPart = tags["series-part"]

	// Description can come from comment or description tag.
	p.parseDescription(tags, metadata)

	// Narrator often stored in composer.
	if narrator := tags["narrator"]; narrator != "" {
		metadata.Narrator = narrator
	}

	// Parse year/date.
	p.parseYear(tags, metadata)

	// Parse track/disc numbers.
	p.parseTrackNumber(tags, metadata)
	p.parseDiscNumber(tags, metadata)
}

// parseDescription extracts description from comment or description tags.
func (p *FFprobeParser) parseDescription(tags map[string]string, metadata *Metadata) {
	if desc := tags["comment"]; desc != "" {
		metadata.Description = desc
	} else if desc := tags["description"]; desc != "" {
		metadata.Description = desc
	}
}

// parseYear extracts year from date or year tags.
func (p *FFprobeParser) parseYear(tags map[string]string, metadata *Metadata) {
	if year := tags["date"]; year != "" {
		if y, err := strconv.Atoi(year); err == nil {
			metadata.Year = y
		}
	} else if year := tags["year"]; year != "" {
		if y, err := strconv.Atoi(year); err == nil {
			metadata.Year = y
		}
	}
}

// parseTrackNumber extracts track number and total from "track" tag (handles "1/10" format).
func (p *FFprobeParser) parseTrackNumber(tags map[string]string, metadata *Metadata) {
	track := tags["track"]
	if track == "" {
		return
	}

	// Handle "1/10" format.
	parts := strings.Split(track, "/")
	if t, err := strconv.Atoi(parts[0]); err == nil {
		metadata.Track = t
	}
	if len(parts) > 1 {
		if tt, err := strconv.Atoi(parts[1]); err == nil {
			metadata.TrackTotal = tt
		}
	}
}

// parseDiscNumber extracts disc number and total from "disc" tag (handles "1/2" format).
func (p *FFprobeParser) parseDiscNumber(tags map[string]string, metadata *Metadata) {
	disc := tags["disc"]
	if disc == "" {
		return
	}

	// Handle "1/2" format.
	parts := strings.Split(disc, "/")
	if d, err := strconv.Atoi(parts[0]); err == nil {
		metadata.Disc = d
	}
	if len(parts) > 1 {
		if dt, err := strconv.Atoi(parts[1]); err == nil {
			metadata.DiscTotal = dt
		}
	}
}

// parseChapters extracts chapter information.
func (p *FFprobeParser) parseChapters(chapters []ffprobeChapter) []Chapter {
	result := make([]Chapter, 0, len(chapters))

	for _, ch := range chapters {
		chapter := Chapter{
			Index: ch.ID,
		}

		// Parse start/end times.
		if ch.StartTime != "" {
			if start, err := strconv.ParseFloat(ch.StartTime, 64); err == nil {
				chapter.StartTime = time.Duration(start * float64(time.Second))
			}
		}

		if ch.EndTime != "" {
			if end, err := strconv.ParseFloat(ch.EndTime, 64); err == nil {
				chapter.EndTime = time.Duration(end * float64(time.Second))
			}
		}

		// Extract chapter title.
		if ch.Tags != nil {
			chapter.Title = ch.Tags["title"]
		}

		result = append(result, chapter)
	}

	return result
}

// ffprobeOutput represents ffprobe JSON output.
type ffprobeOutput struct {
	Format   ffprobeFormat    `json:"format"`
	Streams  []ffprobeStream  `json:"streams"`
	Chapters []ffprobeChapter `json:"chapters"`
}

type ffprobeFormat struct {
	Tags       map[string]string `json:"tags"`
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

type ffprobeChapter struct {
	Tags      map[string]string `json:"tags"`
	TimeBase  string            `json:"time_base"`
	StartTime string            `json:"start_time"`
	EndTime   string            `json:"end_time"`
	ID        int               `json:"id"`
	Start     int64             `json:"start"`
	End       int64             `json:"end"`
}
