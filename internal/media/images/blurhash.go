package images

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/bbrks/go-blurhash"
	_ "golang.org/x/image/webp"
)

// ComputeBlurHash generates a BlurHash string from an image file.
// Uses 4x3 components for a good balance of size (~20-30 chars) and detail.
// Returns empty string and error on failure.
func ComputeBlurHash(imagePath string) (string, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("open image: %w", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	// 4 horizontal, 3 vertical components - sweet spot for book covers
	hash, err := blurhash.Encode(4, 3, img)
	if err != nil {
		return "", fmt.Errorf("encode blurhash: %w", err)
	}

	return hash, nil
}
