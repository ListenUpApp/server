package domain

// CollectionShare represents a sharing relationship where a collection owner
// grants access to another user. This enables the social-first model where
// users can share their curated collections with friends and family. While
// Also providing enough controls to restrict books should you wish.

type CollectionShare struct {
	Syncable
	CollectionID     string          `json:"collection_id"`
	SharedWithUserID string          `json:"shared_with_user_id"`
	SharedByUserID   string          `json:"shared_by_user_id"`
	Permission       SharePermission `json:"permission"`
}

// SharePermission defines the level of access granted to a shared collection.
type SharePermission int

const (
	// PermissionRead allows viewing books in the collection.
	PermissionRead SharePermission = iota
	// PermissionWrite allows adding/removing books from the collection.
	PermissionWrite
)

// String returns the string representation of the permission level.
func (sp SharePermission) String() string {
	switch sp {
	case PermissionRead:
		return "read"
	case PermissionWrite:
		return "write"
	default:
		return "unknown"
	}
}

// ParseSharePermission converts a string to SharePermission.
func ParseSharePermission(s string) (SharePermission, bool) {
	switch s {
	case "read":
		return PermissionRead, true
	case "write":
		return PermissionWrite, true
	default:
		return PermissionRead, false
	}
}

// CanRead returns true if the permission allows reading.
func (sp SharePermission) CanRead() bool {
	return sp == PermissionRead || sp == PermissionWrite
}

// CanWrite returns true if the permission allows writing.
func (sp SharePermission) CanWrite() bool {
	return sp == PermissionWrite
}
