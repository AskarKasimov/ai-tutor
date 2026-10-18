package audio

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

const goodStream = "{\"type\":\"start\",\"version\":1,\"sample_rate\":24000,\"channels\":1,\"encoding\":\"pcm_s16le\"}\n{\"type\":\"audio\",\"seq\":0,\"data\":\"AQACAA==\"}\n{\"type\":\"end\",\"chunks\":1,\"samples\":2}\n"

func TestStreamRoundTripAndValidation(t *testing.T) {
	var events []StreamEvent
	err := ParseStream(context.Background(), strings.NewReader(goodStream), func(e StreamEvent) error { events = append(events, e); return nil })
	if err != nil || len(events) != 3 {
		t.Fatalf("events=%v err=%v", events, err)
	}
	wav, err := WAVFromPCM(events[0].SampleRate, events[1].PCM)
	if err != nil || !ValidWAV(wav) {
		t.Fatalf("invalid WAV %v", err)
	}
	var out []StreamEvent
	if err = StreamWAV(wav, func(e StreamEvent) error { out = append(out, e); return nil }); err != nil || !bytes.Equal(out[1].PCM, events[1].PCM) {
		t.Fatalf("roundtrip %v", err)
	}
	for _, bad := range []string{strings.Replace(goodStream, `"seq":0`, `"seq":1`, 1), strings.Replace(goodStream, `"samples":2`, `"samples":3`, 1), strings.Replace(goodStream, "AQACAA==", "@@", 1), strings.Replace(goodStream, "AQACAA==", "AQ==", 1), strings.Split(goodStream, `{"type":"end"`)[0], goodStream + "{}\n", strings.Repeat("x", MaxStreamLine+1)} {
		if err := ParseStream(context.Background(), strings.NewReader(bad), func(StreamEvent) error { return nil }); err == nil {
			t.Fatal("invalid stream accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ParseStream(ctx, strings.NewReader(goodStream), func(StreamEvent) error { return nil }) == nil {
		t.Fatal("cancel ignored")
	}
}

func TestPCMBufferBoundsAndFinishesWithoutCopy(t *testing.T) {
	b := NewPCMBuffer()
	if err := b.Append([]byte{1, 0, 2, 0}); err != nil {
		t.Fatal(err)
	}
	wav, err := b.WAV(24000)
	if err != nil || !ValidWAV(wav) {
		t.Fatal(err)
	}
	if &wav[44] != &b.data[44] {
		t.Fatal("finish copied the audio")
	}
	b.data = make([]byte, 44+MaxStreamBytes)
	if b.Append([]byte{1, 0}) == nil {
		t.Fatal("unbounded buffer")
	}
}

func TestStreamRequiresNewlineAndCanonicalBase64(t *testing.T) {
	for _, bad := range []string{strings.TrimSuffix(goodStream, "\n"), strings.Replace(goodStream, "AQACAA==", `AQAC\nAA==`, 1), strings.Replace(goodStream, `"seq":0`, `"seq":null`, 1)} {
		if ParseStream(context.Background(), strings.NewReader(bad), func(StreamEvent) error { return nil }) == nil {
			t.Fatal("incomplete line or noncanonical base64 accepted")
		}
	}
}

func TestStreamReadsFragmentedLinesAndEnforcesDecodedLimit(t *testing.T) {
	if err := ParseStream(context.Background(), iotest.OneByteReader(strings.NewReader(goodStream)), func(StreamEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	defer r.Close()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer w.Close()
		_, _ = io.WriteString(w, strings.Split(goodStream, "\n")[0]+"\n")
		data := base64.StdEncoding.EncodeToString(make([]byte, StreamBlockBytes))
		for i := 0; i <= MaxStreamBytes/StreamBlockBytes; i++ {
			if _, err := fmt.Fprintf(w, "{\"type\":\"audio\",\"seq\":%d,\"data\":\"%s\"}\n", i, data); err != nil {
				return
			}
		}
	}()
	blocks := 0
	err := ParseStream(context.Background(), r, func(e StreamEvent) error {
		if e.Type == "audio" {
			blocks++
		}
		return nil
	})
	r.Close()
	<-writerDone
	if err == nil || blocks != MaxStreamBytes/StreamBlockBytes {
		t.Fatalf("decoded limit blocks=%d err=%v", blocks, err)
	}
}
