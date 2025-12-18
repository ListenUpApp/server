package domain

import "time"

// TranscodeStatus represents the state of a transcode job.
type TranscodeStatus string

const (
	TranscodeStatusPending   TranscodeStatus = "pending"
	TranscodeStatusRunning   TranscodeStatus = "running"
	TranscodeStatusCompleted TranscodeStatus = "completed"
	TranscodeStatusFailed    TranscodeStatus = "failed"
)

// TranscodeVariant identifies the output format variant.
type TranscodeVariant string

const (
	TranscodeVariantStereo  TranscodeVariant = "stereo"  // 2-channel AAC
	TranscodeVariantSpatial TranscodeVariant = "spatial" // 6-channel AAC (5.1)
)

// TranscodeJob represents a transcoding operation for an audio file.
// Jobs are created when the scanner detects audio in a format that some
// devices cannot play natively (e.g., Dolby AC-3, E-AC-3, DTS).
type TranscodeJob struct {
	ID          string `json:"id"`
	BookID      string `json:"book_id"`
	AudioFileID string `json:"audio_file_id"`

	// Source file information
	SourcePath  string `json:"source_path"`
	SourceCodec string `json:"source_codec"`
	SourceHash  string `json:"source_hash"` // For invalidation when source changes

	// Target format (AAC in HLS for progressive playback and universal compatibility)
	OutputPath  string `json:"output_path,omitempty"`
	OutputCodec string `json:"output_codec"` // "aac"
	OutputSize  int64  `json:"output_size,omitempty"`

	// Output variant (stereo or spatial)
	Variant TranscodeVariant `json:"variant"` // stereo or spatial

	// Job state
	Status   TranscodeStatus `json:"status"`
	Progress int             `json:"progress"` // 0-100
	Priority int             `json:"priority"` // Higher = more urgent (1=background, 10=user-requested)
	Error    string          `json:"error,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ProblematicCodecs lists audio codecs that require transcoding for universal playback.
// These formats require hardware decoders that not all devices have.
//
// Note: Some codecs (ac4) are proprietary. Standard FFmpeg lacks decoders,
// but librempeg includes them. If transcoding fails due to missing decoder,
// the job is marked failed and the client knows playback isn't possible.
var ProblematicCodecs = map[string]bool{
	"ac3":    true, // Dolby Digital
	"eac3":   true, // Dolby Digital Plus
	"ac4":    true, // Dolby AC-4 (used for Dolby Atmos) - requires librempeg to decode
	"ac-4":   true, // Dolby AC-4 (ffprobe reports with hyphen)
	"truehd": true, // Dolby TrueHD
	"dts":    true, // DTS
	"wma":    true, // Windows Media Audio
}

// NeedsTranscode returns true if the given codec requires transcoding.
func NeedsTranscode(codec string) bool {
	return ProblematicCodecs[codec]
}

// MarkRunning transitions the job to running state.
func (j *TranscodeJob) MarkRunning() {
	j.Status = TranscodeStatusRunning
	now := time.Now()
	j.StartedAt = &now
	j.Progress = 0
}

// MarkCompleted transitions the job to completed state.
func (j *TranscodeJob) MarkCompleted(outputPath string, outputSize int64) {
	j.Status = TranscodeStatusCompleted
	j.OutputPath = outputPath
	j.OutputSize = outputSize
	j.Progress = 100
	now := time.Now()
	j.CompletedAt = &now
}

// MarkFailed transitions the job to failed state with an error message.
func (j *TranscodeJob) MarkFailed(err string) {
	j.Status = TranscodeStatusFailed
	j.Error = err
	now := time.Now()
	j.CompletedAt = &now
}

// SetProgress updates the job's progress percentage.
func (j *TranscodeJob) SetProgress(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	j.Progress = percent
}

// BumpPriority increases the job's priority for user-requested playback.
func (j *TranscodeJob) BumpPriority() {
	if j.Priority < 10 {
		j.Priority = 10
	}
}
