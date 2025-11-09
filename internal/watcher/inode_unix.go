//go:build unix

package watcher

import "syscall"

// getInode extracts the inode number from os.FileInfo.Sys()
// On Unix systems (Linux, macOS, BSD), this returns the actual inode number
func getInode(sys interface{}) uint64 {
	if stat, ok := sys.(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
