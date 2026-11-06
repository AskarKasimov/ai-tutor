package journal

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockVolume(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

// Windows supports the backend's test fixture, not the production gateway's
// crash durability guarantee: it has no equivalent of POSIX directory fsync.
// Payload files themselves are still flushed by Blobs.Put before rename.
func syncDirectory(string) error { return nil }
