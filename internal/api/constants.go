package api

// API limits and constants.
const (
	// MaxUploadSize is the maximum allowed size for file uploads (10 MB).
	MaxUploadSize = 10 << 20
)

// Cache-Control header values.
const (
	CacheOneWeek       = "public, max-age=604800"
	CacheOneDay        = "public, max-age=86400"
	CacheOneDayPrivate = "private, max-age=86400"
	CacheNoStore       = "no-cache"
)
