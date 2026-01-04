package api

// MapSlice transforms a slice using the provided mapper function.
// Useful for converting domain objects to response types.
func MapSlice[T, R any](items []T, mapper func(T) R) []R {
	result := make([]R, len(items))
	for i, item := range items {
		result[i] = mapper(item)
	}
	return result
}

// DefaultLimit returns the provided limit or a default if <= 0.
func DefaultLimit(limit, defaultVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	return limit
}
