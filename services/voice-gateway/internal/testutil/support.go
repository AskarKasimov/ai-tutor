// Package testutil supplies isolated queue journals and audio to tests.
package testutil

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
)

type Memory struct {
	Mu      sync.Mutex
	Records map[string]journal.Record
	Fail    bool
}

func (m *Memory) Append(ctx context.Context, r journal.Record) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if m.Fail {
		return errors.New("test journal unavailable")
	}
	if m.Records == nil {
		m.Records = map[string]journal.Record{}
	}
	if len(r.Value) == 0 {
		delete(m.Records, r.Operation+"/"+r.ID)
	} else {
		m.Records[r.Operation+"/"+r.ID] = r
	}
	return nil
}
func (m *Memory) Replay(context.Context) ([]journal.Record, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	r := []journal.Record{}
	for _, v := range m.Records {
		r = append(r, v)
	}
	return r, nil
}
func (m *Memory) Ping(context.Context) error { return nil }
func (m *Memory) Close()                     {}
func WAV() []byte {
	data := make([]byte, 46)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], 38)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 16000)
	binary.LittleEndian.PutUint32(data[28:], 32000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 2)
	return data
}
