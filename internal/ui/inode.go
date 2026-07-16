package ui

import (
	"os"
	"syscall"
)

// fileInode returns the inode of a path. The inode survives renames, so
// it works as the file identity when pairing rename events.
func fileInode(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Ino, true
}
