// Package backup provides backup and restore functionality for ListenUp.
package backup

import "errors"

var (
	// ErrInvalidManifest indicates the manifest is missing or malformed.
	ErrInvalidManifest = errors.New("invalid or missing manifest")

	// ErrVersionMismatch indicates the backup version is not supported.
	ErrVersionMismatch = errors.New("backup version not supported")

	// ErrCorruptedBackup indicates the backup failed integrity checks.
	ErrCorruptedBackup = errors.New("backup integrity check failed")

	// ErrBackupNotFound indicates the requested backup does not exist.
	ErrBackupNotFound = errors.New("backup not found")
)
