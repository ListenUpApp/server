// Package color provides color generation utilities for user avatars.
package color

import "fmt"

// ForUser generates a consistent hex color for a user based on their ID.
// Uses a deterministic hash to ensure the same user always gets the same color.
// Colors are selected from a pleasant palette with fixed saturation and lightness.
func ForUser(userID string) string {
	h := 0
	for _, c := range userID {
		h = 31*h + int(c)
	}
	if h < 0 {
		h = -h
	}
	hue := float64(h % 360)

	// Convert HSL to RGB (S=0.4, L=0.65 for pleasant, readable colors)
	r, g, b := hslToRGB(hue, 0.4, 0.65)

	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

// hslToRGB converts HSL color space to RGB.
// h: hue (0-360), s: saturation (0-1), l: lightness (0-1)
// Returns RGB values (0-255).
func hslToRGB(h, s, l float64) (r, g, b uint8) {
	h /= 360.0

	var r1, g1, b1 float64

	if s == 0 {
		// Achromatic (gray)
		r1, g1, b1 = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q

		r1 = hueToRGB(p, q, h+1.0/3.0)
		g1 = hueToRGB(p, q, h)
		b1 = hueToRGB(p, q, h-1.0/3.0)
	}

	r = uint8(r1 * 255)
	g = uint8(g1 * 255)
	b = uint8(b1 * 255)
	return
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}
