package audio

import (
	"encoding/binary"
	"math"
)

// pcm16 is a parsed 16-bit PCM WAV: its format chunk and sample frames.
type pcm16 struct {
	format []byte
	frames []byte
	rate   int
	align  int
}

func parsePCM16(data []byte) (pcm16, bool) {
	if !ValidWAV(data) {
		return pcm16{}, false
	}
	var result pcm16
	for pos := 12; pos+8 <= len(data); {
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		chunk := data[pos+8 : pos+8+size]
		switch string(data[pos : pos+4]) {
		case "fmt ":
			if binary.LittleEndian.Uint16(chunk[0:2]) != 1 || binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return pcm16{}, false
			}
			result.format = chunk[:16]
			result.rate = int(binary.LittleEndian.Uint32(chunk[4:8]))
			result.align = int(binary.LittleEndian.Uint16(chunk[12:14]))
		case "data":
			result.frames = chunk
		}
		pos += 8 + size + size%2
	}
	return result, result.format != nil && result.frames != nil
}

func (p pcm16) wav(frames []byte) []byte {
	out := make([]byte, 44+len(frames))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+len(frames)))
	copy(out[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	copy(out[20:36], p.format)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(frames)))
	copy(out[44:], frames)
	return out
}

// energy is the mean square of the samples in frames [from, to).
func (p pcm16) energy(from, to int) float64 {
	var sum float64
	for i := from * p.align; i+1 < to*p.align; i += 2 {
		sample := float64(int16(binary.LittleEndian.Uint16(p.frames[i : i+2])))
		sum += sample * sample
	}
	return sum / math.Max(1, float64(to-from))
}

// SplitWAV cuts a 16-bit PCM WAV longer than maxSeconds into WAV parts of at
// most maxSeconds, each cut at the quietest 100 ms in the last 40% of the part
// (the latest of equally quiet ones) so that words are not split. ok is false
// for audio it cannot split; shorter audio is returned whole.
func SplitWAV(data []byte, maxSeconds int) (parts [][]byte, ok bool) {
	p, ok := parsePCM16(data)
	if !ok || p.rate <= 0 || p.align <= 0 || maxSeconds <= 0 {
		return nil, false
	}
	total := len(p.frames) / p.align
	limit := p.rate * maxSeconds
	window := max(1, p.rate/10)
	for start := 0; start < total; {
		end := total
		if total-start > limit {
			end = start + limit
			best := math.Inf(1)
			for from := start + limit*6/10; from+window <= start+limit; from += window / 2 {
				if e := p.energy(from, from+window); e <= best {
					best, end = e, from+window/2
				}
			}
		}
		parts = append(parts, p.wav(p.frames[start*p.align:end*p.align]))
		start = end
	}
	return parts, true
}

// JoinWAV concatenates 16-bit PCM WAV parts of the same format into one WAV.
// ok is false for no parts, an unparsable part or mismatched formats.
func JoinWAV(parts [][]byte) (joined []byte, ok bool) {
	if len(parts) == 0 {
		return nil, false
	}
	first, ok := parsePCM16(parts[0])
	if !ok {
		return nil, false
	}
	frames := []byte{}
	for _, part := range parts {
		p, ok := parsePCM16(part)
		if !ok || string(p.format) != string(first.format) {
			return nil, false
		}
		frames = append(frames, p.frames...)
	}
	return first.wav(frames), true
}
