package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInode(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Get file info
	info, err := os.Stat(testFile)
	require.NoError(t, err)

	// Extract inode
	inode := getInode(info.Sys())

	// Inode should be non-zero
	assert.NotZero(t, inode, "Inode should not be zero")
}

func TestGetInode_DifferentFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two different files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	err := os.WriteFile(file1, []byte("content 1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(file2, []byte("content 2"), 0644)
	require.NoError(t, err)

	// Get inodes
	info1, err := os.Stat(file1)
	require.NoError(t, err)
	inode1 := getInode(info1.Sys())

	info2, err := os.Stat(file2)
	require.NoError(t, err)
	inode2 := getInode(info2.Sys())

	// Different files should have different inodes
	assert.NotEqual(t, inode1, inode2, "Different files should have different inodes")
}

func TestGetInode_InvalidSysInterface(t *testing.T) {
	// Passing nil or invalid interface should return 0
	inode := getInode(nil)
	assert.Zero(t, inode, "Invalid sys interface should return 0")

	inode = getInode("not a valid type")
	assert.Zero(t, inode, "Invalid sys interface should return 0")
}
