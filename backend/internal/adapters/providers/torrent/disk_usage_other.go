//go:build !unix

package torrent

import "os"

func allocatedFileSize(info os.FileInfo) int64 {
	return info.Size()
}
