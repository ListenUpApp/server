package audio

import (
	"context"

	"github.com/listenupapp/listenup-server/pkg/audiometa/m4a"
)

type NativeParser struct{}

func NewNativeParser() *NativeParser {
	return &NativeParser{}
}

func (p *NativeParser) Parse(ctx context.Context, path string) (*Metadata, error) {
	// Use native parser
	meta, err := m4a.Parse(path)
	if err != nil {
		return nil, err
	}

	// Convert to scanner's Metadata format
	result := &Metadata{
		Format:     string(meta.Format),
		Duration:   meta.Duration,
		Bitrate:    meta.BitRate,
		SampleRate: meta.SampleRate,
		Channels:   meta.Channels,
		Codec:      meta.Codec,

		// Standard tags
		Title:       meta.Title,
		Album:       meta.Album,
		Artist:      meta.Artist,
		AlbumArtist: meta.Artist, // M4B doesn't distinguish
		Composer:    meta.Composer,
		Genre:       meta.Genre,
		Year:        meta.Year,

		// Audiobook-specific
		Narrator:    meta.Composer, // Composer field = Narrator in audiobooks
		Description: meta.Comment,  // Comment often contains description

		// TODO: Add when Phase 2 is done
		// Chapters:    convertChapters(meta.Chapters),
	}

	return result, nil
}
