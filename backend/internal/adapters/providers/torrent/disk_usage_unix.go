//go:build unix

package torrent

import (
	"os"
	"syscall"
)

func allocatedFileSize(info os.FileInfo) int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size()
	}
	return stat.Blocks * 512
}
