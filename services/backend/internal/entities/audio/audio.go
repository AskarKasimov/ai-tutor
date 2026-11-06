package audio

import (
	"bytes"
	wav "github.com/AskarKasimov/ai-tutor/services/audio"
	"mime"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func Check(data []byte, media string) error {
	if len(data) == 0 {
		return fault.New(fault.Invalid, "INVALID_AUDIO", "Аудиозапись пуста.")
	}
	base, params, _ := mime.ParseMediaType(media)
	switch base {
	case "audio/wav", "audio/x-wav", "audio/wave":
		info, ok := wav.InspectWAV(data)
		if !ok || info.SampleRate < 8000 || info.SampleRate > 192000 {
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
