package journal

import (
	"os"
	"strings"
	"testing"
)

func TestPayloadBoundsAndVolumeLock(t *testing.T) {
	dir := t.TempDir()
	b, err := OpenBlobs(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if second, err := OpenBlobs(dir, 10); err == nil {
		second.Close()
		t.Fatal("volume opened by two processes")
	}
	name := strings.Repeat("a", 32) + ".in"
	if err = b.Put(name, []byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if err = b.Put(strings.Repeat("b", 32)+".out", []byte("1")); err == nil {
		t.Fatal("quota ignored")
	}
	if _, err = b.Get(name, 5); err == nil {
		t.Fatal("read bound ignored")
	}
	if _, err = b.Get("../outside", 10); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err = b.Remove(name); err != nil {
		t.Fatal(err)
	}
	if b.Usage() != 0 {
		t.Fatal("usage not decremented")
	}
	if _, err = os.Stat(dir + "/" + name); !os.IsNotExist(err) {
		t.Fatal("blob not removed")
	}
}
