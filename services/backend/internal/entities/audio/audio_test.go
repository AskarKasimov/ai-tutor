package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func wavFixture(extensible bool) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(0))
	b.WriteString("WAVEfmt ")
	size, format := uint32(16), uint16(1)
	if extensible {
		size, format = 40, 0xfffe
	}
	for _, v := range []any{size, format, uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	if extensible {
		for _, v := range []any{uint16(22), uint16(16), uint32(0)} {
			_ = binary.Write(&b, binary.LittleEndian, v)
		}
		b.Write([]byte{1, 0, 0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71})
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(2))
	b.Write([]byte{0, 0})
	data := b.Bytes()
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	return data
}

func TestValidWAVPreservesFormatAndFrameInvariants(t *testing.T) {
	for _, extensible := range []bool{false, true} {
		data := wavFixture(extensible)
		if !ValidWAV(data) {
			t.Fatal("valid WAV rejected")
		}
		for _, tc := range []struct {
			name   string
			mutate func([]byte)
		}{
			{"riff size", func(b []byte) { b[4]++ }},
			{"channels", func(b []byte) { b[22] = 0 }},
			{"rate", func(b []byte) { clear(b[24:28]) }},
			{"byte rate", func(b []byte) { clear(b[28:32]) }},
			{"frame alignment", func(b []byte) { b[32] = 1 }},
			{"sample bits", func(b []byte) { b[34] = 12 }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				bad := bytes.Clone(data)
				tc.mutate(bad)
				if ValidWAV(bad) {
					t.Fatal("invalid WAV accepted")
				}
			})
		}
	}
	ext := wavFixture(true)
	for _, offset := range []int{38, 46, 50} {
		bad := bytes.Clone(ext)
		bad[offset] = 0xff
		if ValidWAV(bad) {
			t.Fatalf("invalid extensible field at %d accepted", offset)
		}
	}
	badExtension := bytes.Clone(ext)
	binary.LittleEndian.PutUint16(badExtension[36:38], 21)
	if ValidWAV(badExtension) {
		t.Fatal("short extensible format extension accepted")
	}
	// One byte of data cannot form a 16-bit frame, even with valid chunk padding.
	bad := wavFixture(false)
	binary.LittleEndian.PutUint32(bad[40:44], 1)
	if ValidWAV(bad) {
		t.Fatal("partial audio frame accepted")
	}
}

func TestCheckPreservesCodecMetadata(t *testing.T) {
	ogg := append(append([]byte("OggS"), make([]byte, 23)...), []byte("OpusHead")...)
	if err := Check(ogg, "audio/ogg; codecs=opus"); err != nil {
		t.Fatal(err)
	}
	if err := Check(ogg, "audio/ogg; codecs=vorbis"); err == nil {
		t.Fatal("unsupported codec accepted")
	}
}
