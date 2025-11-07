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
		Format:     meta.Format.String(),
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
		Narrator:   meta.Narrator,
		Publisher:  meta.Publisher,
		Series:     meta.Series,
		SeriesPart: meta.SeriesPart,
		ISBN:       meta.ISBN,
		ASIN:       meta.ASIN,

		// Description from comment
		Description: meta.Comment,
	}

	// Convert chapters
	for _, ch := range meta.Chapters {
		result.Chapters = append(result.Chapters, Chapter{
			Index:     ch.Index,
			Title:     ch.Title,
			StartTime: ch.StartTime,
			EndTime:   ch.EndTime,
		})
	}

	return result, nil
}
