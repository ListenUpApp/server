package images

import (
	"fmt"
	"image"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"os"

	"github.com/bbrks/go-blurhash"
	_ "golang.org/x/image/webp" // Register WebP decoder
)

// blurHashSize is the target size for BlurHash computation.
// BlurHash doesn't need high resolution - a small thumbnail produces nearly identical results.
// Using 64x64 reduces computation time from seconds to milliseconds.
const blurHashSize = 64

// ComputeBlurHash generates a BlurHash string from an image file.
// Uses 4x3 components for a good balance of size (~20-30 chars) and detail.
// The image is resized to a small thumbnail first for performance.
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

	// Resize to small thumbnail for fast BlurHash computation.
	// BlurHash is a low-resolution placeholder, so we don't need the full image.
	thumbnail := resizeForBlurHash(img)

	// 4 horizontal, 3 vertical components - sweet spot for book covers
	hash, err := blurhash.Encode(4, 3, thumbnail)
	if err != nil {
		return "", fmt.Errorf("encode blurhash: %w", err)
	}

	return hash, nil
}

// resizeForBlurHash creates a small thumbnail suitable for BlurHash computation.
// Uses simple nearest-neighbor scaling which is fast and sufficient for BlurHash.
func resizeForBlurHash(img image.Image) image.Image {
	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// If image is already small enough, use it directly
	if srcWidth <= blurHashSize && srcHeight <= blurHashSize {
		return img
	}

	// Calculate target dimensions maintaining aspect ratio
	var dstWidth, dstHeight int
	if srcWidth > srcHeight {
		dstWidth = blurHashSize
		dstHeight = (srcHeight * blurHashSize) / srcWidth
		if dstHeight < 1 {
			dstHeight = 1
		}
	} else {
		dstHeight = blurHashSize
		dstWidth = (srcWidth * blurHashSize) / srcHeight
		if dstWidth < 1 {
			dstWidth = 1
		}
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	// Simple box scaling - fast and sufficient for BlurHash
	xRatio := float64(srcWidth) / float64(dstWidth)
	yRatio := float64(srcHeight) / float64(dstHeight)

	for y := 0; y < dstHeight; y++ {
		for x := 0; x < dstWidth; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			dst.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}
