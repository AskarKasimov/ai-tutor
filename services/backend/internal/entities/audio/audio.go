package audio

import (
	"bytes"
	"encoding/binary"
	"mime"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

// Validate the RIFF chunk structure instead of trusting a filename/MIME alone.
func ValidWAV(data []byte) bool {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return false
	}
	end := int64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if end != int64(len(data)) {
		return false
	}
	haveFmt, haveData := false, false
	var blockAlign uint16
	var dataSize int64
	for pos := int64(12); pos < end; {
		if pos+8 > end {
			return false
		}
		size := int64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		if start+size > end {
			return false
		}
		switch string(data[pos : pos+4]) {
		case "fmt ":
			if size < 16 {
				return false
			}
			format := binary.LittleEndian.Uint16(data[start : start+2])
			channels := binary.LittleEndian.Uint16(data[start+2 : start+4])
			rate := binary.LittleEndian.Uint32(data[start+4 : start+8])
			byteRate := binary.LittleEndian.Uint32(data[start+8 : start+12])
			blockAlign = binary.LittleEndian.Uint16(data[start+12 : start+14])
			bits := binary.LittleEndian.Uint16(data[start+14 : start+16])
			if format == 0xfffe {
				if size < 40 || binary.LittleEndian.Uint16(data[start+16:start+18]) < 22 {
					return false
				}
				validBits := binary.LittleEndian.Uint16(data[start+18 : start+20])
				if validBits == 0 || validBits > bits {
					return false
				}
				guid := data[start+24 : start+40]
				if !bytes.Equal(guid[2:], []byte{0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}) {
					return false
				}
				format = binary.LittleEndian.Uint16(guid[:2])
			}
			validBits := (format == 1 && (bits == 8 || bits == 16 || bits == 24 || bits == 32)) || (format == 3 && (bits == 32 || bits == 64))
			if !validBits || channels == 0 || rate == 0 || uint64(blockAlign) != uint64(channels)*uint64(bits)/8 || uint64(byteRate) != uint64(rate)*uint64(blockAlign) {
				return false
			}
			haveFmt = true
		case "data":
			haveData = size > 0
			dataSize = size
		}
		pos = start + size + (size % 2)
		if pos > end {
			return false
		}
	}
	return haveFmt && haveData && blockAlign > 0 && dataSize%int64(blockAlign) == 0
}

func Check(data []byte, media string) error {
	if len(data) == 0 {
		return fault.New(fault.Invalid, "INVALID_AUDIO", "Аудиозапись пуста.")
	}
	base, params, _ := mime.ParseMediaType(media)
	switch base {
	case "audio/wav", "audio/x-wav", "audio/wave":
		if !ValidWAV(data) {
			return fault.New(fault.Invalid, "INVALID_AUDIO", "Повреждённая WAV-запись.")
		}
	case "audio/ogg", "application/ogg":
		if len(data) < 27 || string(data[:4]) != "OggS" {
			return fault.New(fault.Invalid, "INVALID_AUDIO", "Повреждённый Ogg-контейнер.")
		}
		if !bytes.Contains(data, []byte("OpusHead")) {
			return fault.New(fault.Unsupported, "UNSUPPORTED_AUDIO_FORMAT", "Для Ogg поддерживается только Opus.")
		}
	case "audio/webm", "video/webm":
		if len(data) < 4 || !bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
			return fault.New(fault.Invalid, "INVALID_AUDIO", "Повреждённый WebM-контейнер.")
		}
		if !bytes.Contains(data, []byte("A_OPUS")) {
			return fault.New(fault.Unsupported, "UNSUPPORTED_AUDIO_FORMAT", "Для WebM поддерживается только Opus.")
		}
	default:
		return fault.New(fault.Unsupported, "UNSUPPORTED_AUDIO_FORMAT", "Поддерживаются WAV, Ogg/Opus и WebM/Opus.")
	}
	if codec := strings.ToLower(params["codecs"]); codec != "" && codec != "opus" {
		return fault.New(fault.Unsupported, "UNSUPPORTED_AUDIO_FORMAT", "Неподдерживаемый кодек.")
	}
	return nil
}
