package journal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
)

var blobName = regexp.MustCompile(`^[a-f0-9]{32}\.(in|out)$`)

type Blobs struct {
	dir   string
	lock  *os.File
	mu    sync.Mutex
	bytes int64
	limit int64
}

// OpenBlobs also locks the volume. A gateway deployment has one active process;
// additional replicas must never share these queue topics with another volume.
func OpenBlobs(dir string, limit int64) (*Blobs, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("voice data volume already locked")
	}
	b := &Blobs{dir: dir, lock: file, limit: limit}
	entries, err := os.ReadDir(dir)
	if err != nil {
		b.Close()
		return nil, err
	}
	for _, e := range entries {
		if blobName.MatchString(e.Name()) {
			info, err := e.Info()
			if err != nil {
				b.Close()
				return nil, err
			}
			b.bytes += info.Size()
		} else if len(e.Name()) > 5 && e.Name()[:5] == ".tmp-" {
			if err = os.Remove(filepath.Join(dir, e.Name())); err != nil {
				b.Close()
				return nil, err
			}
		}
	}
	return b, nil
}
func (b *Blobs) Close() { _ = syscall.Flock(int(b.lock.Fd()), syscall.LOCK_UN); _ = b.lock.Close() }
func (b *Blobs) Put(name string, data []byte) error {
	if !blobName.MatchString(name) {
		return errors.New("invalid blob name")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := os.Stat(filepath.Join(b.dir, name)); err == nil {
		return errors.New("blob already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if b.bytes+int64(len(data)) > b.limit {
		return errors.New("voice payload storage limit exceeded")
	}
	file, err := os.CreateTemp(b.dir, ".tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(file.Name(), filepath.Join(b.dir, name)); err != nil {
		return err
	}
	b.bytes += int64(len(data))
	if err = b.sync(); err != nil {
		return err
	}
	return nil
}
func (b *Blobs) Get(name string, max int64) ([]byte, error) {
	if !blobName.MatchString(name) {
		return nil, errors.New("invalid blob name")
	}
	file, err := os.Open(filepath.Join(b.dir, name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("voice payload too large")
	}
	return data, nil
}
func (b *Blobs) Remove(name string) error {
	if !blobName.MatchString(name) {
		return errors.New("invalid blob name")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	info, err := os.Stat(filepath.Join(b.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(b.dir, name)); err != nil {
		return err
	}
	b.bytes -= info.Size()
	return b.sync()
}
func (b *Blobs) Usage() int64 { b.mu.Lock(); defer b.mu.Unlock(); return b.bytes }
func (b *Blobs) sync() error {
	dir, err := os.Open(b.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (b *Blobs) Names() ([]string, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if blobName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
