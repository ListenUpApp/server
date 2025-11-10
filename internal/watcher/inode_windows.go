//go:build windows

package watcher

// getInode extracts a file identifier from os.FileInfo.Sys()
// Windows doesn't have inodes, so we use the file index from FileAttributeData
// Note: This is a best-effort implementation. For true file identity on Windows,
// TODO use GetFileInformationByHandle if we decide we actually want to support windows as anything but a development platform.
func getInode(sys interface{}) uint64 {
	// Windows doesn't expose file index through Stat_t easily
	// For now, return 0. Move detection won't work on Windows,
	// but that's acceptable for a development platform.
	// TODO: Implement proper file ID tracking using GetFileInformationByHandle
	return 0
}
