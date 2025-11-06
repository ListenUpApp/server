package audio

import "context"

// FFprobeParser uses ffprobe to parse audio metadata
type FFprobeParser struct{}

// NewFFprobeParser creates a new ffprobe parser
func NewFFprobeParser() *FFprobeParser {
	return &FFprobeParser{}
}

// Parse extracts metadata using ffprobe
func (p *FFprobeParser) Parse(ctx context.Context, path string) (*Metadata, error) {
	// TODO: implement
	//
	// Command: ffprobe -v quiet -print_format json -show_format -show_chapters -show_streams
	// Parse JSON output
	// Convert to our Metadata structure
	// Handle errors gracefully

	return nil, nil
}

// ffprobeOutput represents ffprobe JSON output
type ffprobeOutput struct {
	Format   ffprobeFormat    `json:"format"`
	Streams  []ffprobeStream  `json:"streams"`
	Chapters []ffprobeChapter `json:"chapters"`
}

type ffprobeFormat struct {
	Filename string            `json:"filename"`
	Duration string            `json:"duration"`
	Size     string            `json:"size"`
	BitRate  string            `json:"bit_rate"`
	Tags     map[string]string `json:"tags"`
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
