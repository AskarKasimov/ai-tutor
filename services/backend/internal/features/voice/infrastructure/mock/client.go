// Package mock provides local demonstration speech adapters without network calls.
package mock

import (
	"context"
	"encoding/binary"
	"math"
)

type Client struct{}

func (Client) Recognize(ctx context.Context, _ []byte, _ string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "Демонстрационная расшифровка: это пример ответа ученика.", nil
}

// Synthesize returns a short tone, not a reading of the requested text.
func (Client) Synthesize(ctx context.Context, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	const rate, samples = 8000, 3200
	data := make([]byte, 44+samples*2)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], rate)
	binary.LittleEndian.PutUint32(data[28:32], rate*2)
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], samples*2)
	for i := 0; i < samples; i++ {
		envelope := min(float64(i)/160, float64(samples-i)/160, 1)
		value := int16(math.Round(math.Sin(float64(i)/rate*2*math.Pi*440) * envelope * 2000))
		binary.LittleEndian.PutUint16(data[44+i*2:46+i*2], uint16(value))
	}
	return data, nil
}
