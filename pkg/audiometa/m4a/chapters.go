package m4a

import (
	"time"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	"github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// parseChapters extracts chapter markers from the chpl atom
func parseChapters(sr *binary.SafeReader, moovAtom *Atom, fileDuration time.Duration) ([]audiometa.Chapter, error) {
	// Find udta atom (user data)
	// Path: moov -> udta
	udtaAtom, err := findAtom(sr, moovAtom.DataOffset(), moovAtom.DataOffset()+int64(moovAtom.DataSize()), "udta")
	if err != nil {
		// No udta - no chapters
		return nil, nil
	}

	// Find chpl atom (chapter list)
	// Path: udta -> chpl
	chplAtom, err := findAtom(sr, udtaAtom.DataOffset(), udtaAtom.DataOffset()+int64(udtaAtom.DataSize()), "chpl")
	if err != nil {
		// No chpl - no chapters
		return nil, nil
	}

	// Parse chpl atom
	offset := chplAtom.DataOffset()

	// Read version (1 byte)
	_, err = binary.Read[uint8](sr, offset, "chpl version")
	if err != nil {
		return nil, err
	}
	offset += 1

	// Skip flags (3 bytes)
	offset += 3

	// Skip reserved (4 bytes)
	offset += 4

	// Read chapter count (1 byte)
	chapterCount, err := binary.Read[uint8](sr, offset, "chapter count")
	if err != nil {
		return nil, err
	}
	offset += 1

	if chapterCount == 0 {
		return nil, nil
	}

	chapters := make([]audiometa.Chapter, 0, chapterCount)

	// Read each chapter
	for i := uint8(0); i < chapterCount; i++ {
		// Read start time (8 bytes, in 100-nanosecond units)
		startTime100ns, err := binary.Read[uint64](sr, offset, "chapter start time")
		if err != nil {
			return nil, err
		}
		offset += 8

		// Convert to time.Duration (100-nanosecond units -> nanoseconds)
		startTime := time.Duration(startTime100ns * 100)

		// Read title length (1 byte)
		titleLen, err := binary.Read[uint8](sr, offset, "chapter title length")
		if err != nil {
			return nil, err
		}
		offset += 1

		// Read title (N bytes)
		var title string
		if titleLen > 0 {
			titleBytes := make([]byte, titleLen)
			if err := sr.ReadAt(titleBytes, offset, "chapter title"); err != nil {
				return nil, err
			}
			offset += int64(titleLen)
			title = string(titleBytes)
		}

		chapter := audiometa.Chapter{
			Index:     int(i + 1),
			Title:     title,
			StartTime: startTime,
		}

		chapters = append(chapters, chapter)
	}

	// Calculate end times
	// Each chapter ends where the next one starts
	// Last chapter ends at file duration
	for i := 0; i < len(chapters); i++ {
		if i < len(chapters)-1 {
			// End time is the start of the next chapter
			chapters[i].EndTime = chapters[i+1].StartTime
		} else {
			// Last chapter ends at file duration

			chapters[i].EndTime = fileDuration
		}
	}

	return chapters, nil
}
