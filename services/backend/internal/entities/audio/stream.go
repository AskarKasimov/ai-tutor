package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const (
	MaxStreamBytes   = 64 * 1024 * 1024
	MaxStreamLine    = 256 * 1024
	StreamBlockBytes = 64 * 1024
)

var ErrStream = errors.New("invalid or incomplete PCM stream")

// StreamEvent is a validated wire frame. PCM is decoded internally; Data is wire-only.
type StreamEvent struct {
	Type       string `json:"type"`
	Version    int    `json:"version,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	Encoding   string `json:"encoding,omitempty"`
	Seq        int    `json:"seq,omitempty"`
	Data       string `json:"data,omitempty"`
	Chunks     int    `json:"chunks,omitempty"`
	Samples    int    `json:"samples,omitempty"`
	Code       string `json:"code,omitempty"`
	PCM        []byte `json:"-"`
}

// MarshalJSON preserves mandatory zero counters in audio/end frames.
func (e StreamEvent) MarshalJSON() ([]byte, error) {
	switch e.Type {
	case "audio":
		return json.Marshal(struct {
			Type string `json:"type"`
			Seq  int    `json:"seq"`
			Data string `json:"data"`
		}{e.Type, e.Seq, base64.StdEncoding.EncodeToString(e.PCM)})
	case "end":
		return json.Marshal(struct {
			Type    string `json:"type"`
			Chunks  int    `json:"chunks"`
			Samples int    `json:"samples"`
		}{e.Type, e.Chunks, e.Samples})
	default:
		type plain StreamEvent
		return json.Marshal(plain(e))
	}
}
func StartEvent(rate int) StreamEvent {
	return StreamEvent{Type: "start", Version: 1, SampleRate: rate, Channels: 1, Encoding: "pcm_s16le"}
}
func ParseStream(ctx context.Context, r io.Reader, emit func(StreamEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), MaxStreamLine+1)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return 0, nil, ErrStream
		}
		return 0, nil, nil
	})
	started, ended := false, false
	chunks, total := 0, 0
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if ended || len(scanner.Bytes()) > MaxStreamLine || !utf8.Valid(scanner.Bytes()) {
			return ErrStream
		}
		var e StreamEvent
		// Counters are required even when zero.
		var fields map[string]json.RawMessage
		if json.Unmarshal(scanner.Bytes(), &e) != nil || json.Unmarshal(scanner.Bytes(), &fields) != nil {
			return ErrStream
		}
		switch e.Type {
		case "start":
			if started || e.Version != 1 || e.SampleRate < 8000 || e.SampleRate > 192000 || e.Channels != 1 || e.Encoding != "pcm_s16le" {
				return ErrStream
			}
			started = true
		case "audio":
			if !started || (fields["seq"] == nil || string(fields["seq"]) == "null") || fields["data"] == nil || e.Seq != chunks {
				return ErrStream
			}
			var err error
			e.PCM, err = base64.StdEncoding.Strict().DecodeString(e.Data)
			if err != nil || base64.StdEncoding.EncodeToString(e.PCM) != e.Data || len(e.PCM) == 0 || len(e.PCM)%2 != 0 || total+len(e.PCM) > MaxStreamBytes {
				return ErrStream
			}
			total += len(e.PCM)
			chunks++
			e.Data = ""
		case "end":
			if !started || total == 0 || fields["chunks"] == nil || fields["samples"] == nil || e.Chunks != chunks || e.Samples != total/2 {
				return ErrStream
			}
			ended = true
		case "error":
			return ErrStream
		default:
			return ErrStream
		}
		// Hold end until EOF, so trailing garbage cannot publish a cache.
		if e.Type != "end" {
			if err := emit(e); err != nil {
				return err
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scanner.Err() != nil {
		return scanner.Err()
	}
	if !ended {
		return ErrStream
	}
	return emit(StreamEvent{Type: "end", Chunks: chunks, Samples: total / 2})
}
func WAVFromPCM(rate int, pcm []byte) ([]byte, error) {
	if rate < 8000 || rate > 192000 || len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > MaxStreamBytes {
		return nil, ErrStream
	}
	format := make([]byte, 16)
	binary.LittleEndian.PutUint16(format, 1)
	binary.LittleEndian.PutUint16(format[2:], 1)
	binary.LittleEndian.PutUint32(format[4:], uint32(rate))
	binary.LittleEndian.PutUint32(format[8:], uint32(rate*2))
	binary.LittleEndian.PutUint16(format[12:], 2)
	binary.LittleEndian.PutUint16(format[14:], 16)
	return (pcm16{format: format}).wav(pcm), nil
}
func PCMFromWAV(wav []byte) (int, []byte, error) {
	p, ok := parsePCM16(wav)
	if !ok || p.align != 2 || p.rate < 8000 || p.rate > 192000 || len(p.frames) > MaxStreamBytes {
		return 0, nil, ErrStream
	}
	return p.rate, p.frames, nil
}
func StreamWAV(wav []byte, emit func(StreamEvent) error) error {
	rate, pcm, err := PCMFromWAV(wav)
	if err != nil {
		return err
	}
	if err = emit(StartEvent(rate)); err != nil {
		return err
	}
	seq := 0
	for len(pcm) > 0 {
		n := min(len(pcm), StreamBlockBytes)
		if err = emit(StreamEvent{Type: "audio", Seq: seq, PCM: pcm[:n]}); err != nil {
			return err
		}
		pcm = pcm[n:]
		seq++
	}
	_, frames, _ := PCMFromWAV(wav)
	return emit(StreamEvent{Type: "end", Chunks: seq, Samples: len(frames) / 2})
}

// PCMBuffer reserves the WAV header from the start. Its backing array never
// exceeds 64 MiB PCM + 44 header bytes; finalization does not copy the samples.
type PCMBuffer struct{ data []byte }

func NewPCMBuffer() *PCMBuffer { return &PCMBuffer{data: make([]byte, 44)} }
func (b *PCMBuffer) Append(pcm []byte) error {
	if len(pcm) == 0 || len(pcm)%2 != 0 || len(b.data)-44+len(pcm) > MaxStreamBytes {
		return ErrStream
	}
	wanted := len(b.data) + len(pcm)
	if wanted > cap(b.data) {
		capacity := min(MaxStreamBytes+44, max(wanted, cap(b.data)*2))
		next := make([]byte, len(b.data), capacity)
		copy(next, b.data)
		b.data = next
	}
	b.data = append(b.data, pcm...)
	return nil
}
func (b *PCMBuffer) Samples() int { return (len(b.data) - 44) / 2 }
func (b *PCMBuffer) WAV(rate int) ([]byte, error) {
	if rate < 8000 || rate > 192000 || len(b.data) <= 44 {
		return nil, ErrStream
	}
	header, err := WAVFromPCM(rate, []byte{0, 0})
	if err != nil {
		return nil, err
	}
	copy(b.data[:44], header[:44])
	binary.LittleEndian.PutUint32(b.data[4:], uint32(len(b.data)-8))
	binary.LittleEndian.PutUint32(b.data[40:], uint32(len(b.data)-44))
	return b.data, nil
}
