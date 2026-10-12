package audio

import (
	"encoding/binary"
	"testing"
)

// wav16 builds a mono 16-bit PCM WAV; loud(second) gives the amplitude.
func wav16(rate, seconds int, loud func(second float64) int16) []byte {
	p := pcm16{format: make([]byte, 16), rate: rate, align: 2}
	binary.LittleEndian.PutUint16(p.format[0:2], 1)
	binary.LittleEndian.PutUint16(p.format[2:4], 1)
	binary.LittleEndian.PutUint32(p.format[4:8], uint32(rate))
	binary.LittleEndian.PutUint32(p.format[8:12], uint32(rate*2))
	binary.LittleEndian.PutUint16(p.format[12:14], 2)
	binary.LittleEndian.PutUint16(p.format[14:16], 16)
	frames := make([]byte, rate*seconds*2)
	for i := 0; i < rate*seconds; i++ {
		amplitude := loud(float64(i) / float64(rate))
		if i%2 == 1 {
			amplitude = -amplitude
		}
		binary.LittleEndian.PutUint16(frames[i*2:], uint16(amplitude))
	}
	return p.wav(frames)
}

func seconds(part []byte) float64 {
	return float64(len(part)-44) / 2 / 8000
}

func TestSplitWAVCutsLongSpeechAtPauses(t *testing.T) {
	// Speech with a pause at 15 s and at 32 s.
	data := wav16(8000, 45, func(s float64) int16 {
		if (s > 14.8 && s < 15.2) || (s > 31.8 && s < 32.2) {
			return 0
		}
		return 8000
	})
	parts, ok := SplitWAV(data, 20)
	if !ok || len(parts) != 3 {
		t.Fatalf("parts: %d %v", len(parts), ok)
	}
	if got := seconds(parts[0]); got < 14.8 || got > 15.2 {
		t.Fatalf("first cut at %.2f s, want the pause at 15 s", got)
	}
	total := 0.0
	for _, part := range parts {
		if !ValidWAV(part) || seconds(part) > 20 {
			t.Fatalf("invalid part of %.2f s", seconds(part))
		}
		total += seconds(part)
	}
	if total != 45 {
		t.Fatalf("parts hold %.2f s, want 45", total)
	}
}

func TestSplitWAVKeepsShortAudioAndRejectsOtherFormats(t *testing.T) {
	short := wav16(8000, 5, func(float64) int16 { return 100 })
	if parts, ok := SplitWAV(short, 20); !ok || len(parts) != 1 || seconds(parts[0]) != 5 {
		t.Fatal("short audio must stay whole")
	}
	if _, ok := SplitWAV([]byte("OggS not a wav"), 20); ok {
		t.Fatal("non-WAV audio cannot be split")
	}
}
