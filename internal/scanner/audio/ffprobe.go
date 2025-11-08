package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FFprobeParser uses ffprobe to parse audio metadata
type FFprobeParser struct{}

// NewFFprobeParser creates a new ffprobe parser
func NewFFprobeParser() *FFprobeParser {
	return &FFprobeParser{}
}

func (p *FFprobeParser) ParseMultiFile(ctx context.Context, paths []string) (*Metadata, error) {
	// FFprobe doesn't support multi-file aggregation
	// Fall back to parsing the first file
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}
	return p.Parse(ctx, paths[0])
}

// Parse extracts metadata using ffprobe
func (p *FFprobeParser) Parse(ctx context.Context, path string) (*Metadata, error) {
	// Run ffprobe command
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

	// Parse JSON output
	var ffprobeData ffprobeOutput
	if err := json.Unmarshal(output, &ffprobeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	// Convert to our Metadata structure
	metadata := &Metadata{}

	// Extract format information
	if ffprobeData.Format.FormatName != "" {
		// Get first format (e.g., "mp3" from "mp3,mp2")
		formats := strings.Split(ffprobeData.Format.FormatName, ",")
		metadata.Format = formats[0]
	}

	// Parse duration
	if ffprobeData.Format.Duration != "" {
		if dur, err := strconv.ParseFloat(ffprobeData.Format.Duration, 64); err == nil {
			metadata.Duration = time.Duration(dur * float64(time.Second))
		}
	}

	// Parse bitrate
	if ffprobeData.Format.BitRate != "" {
		if br, err := strconv.Atoi(ffprobeData.Format.BitRate); err == nil {
			metadata.Bitrate = br
		}
	}

	// Extract audio stream information
	for _, stream := range ffprobeData.Streams {
		if stream.CodecType == "audio" {
			metadata.Codec = stream.CodecName

			// Parse sample rate
			if stream.SampleRate != "" {
				if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
					metadata.SampleRate = sr
				}
			}

			metadata.Channels = stream.Channels
			break
		}
	}

	// Extract tags
	tags := ffprobeData.Format.Tags
	if tags != nil {
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

		// Description can come from comment or description tag
		if desc := tags["comment"]; desc != "" {
			metadata.Description = desc
		} else if desc := tags["description"]; desc != "" {
			metadata.Description = desc
		}

		// Narrator often stored in composer
		if narrator := tags["narrator"]; narrator != "" {
			metadata.Narrator = narrator
		}

		// Parse year/date
		if year := tags["date"]; year != "" {
			if y, err := strconv.Atoi(year); err == nil {
				metadata.Year = y
			}
		} else if year := tags["year"]; year != "" {
			if y, err := strconv.Atoi(year); err == nil {
				metadata.Year = y
			}
		}

		// Parse track/disc numbers
		if track := tags["track"]; track != "" {
			// Handle "1/10" format
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

		if disc := tags["disc"]; disc != "" {
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
	}

	// Extract chapters
	for _, ch := range ffprobeData.Chapters {
		chapter := Chapter{
			Index: ch.ID,
		}

		// Parse start/end times
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

		// Extract chapter title
		if ch.Tags != nil {
			chapter.Title = ch.Tags["title"]
		}

		metadata.Chapters = append(metadata.Chapters, chapter)
	}

	return metadata, nil
}

// ffprobeOutput represents ffprobe JSON output
type ffprobeOutput struct {
	Format   ffprobeFormat    `json:"format"`
	Streams  []ffprobeStream  `json:"streams"`
	Chapters []ffprobeChapter `json:"chapters"`
}

type ffprobeFormat struct {
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

type ffprobeChapter struct {
	ID        int               `json:"id"`
	TimeBase  string            `json:"time_base"`
	Start     int64             `json:"start"`
	End       int64             `json:"end"`
	StartTime string            `json:"start_time"`
	EndTime   string            `json:"end_time"`
	Tags      map[string]string `json:"tags"`
}
