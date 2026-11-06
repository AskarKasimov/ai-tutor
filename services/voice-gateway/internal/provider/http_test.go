package provider

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"google.golang.org/grpc/codes"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func wav() []byte {
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
func TestExistingModelProtocols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stt":
			if r.URL.Query().Get("model") != "example" {
				t.Error("missing model")
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			file.Close()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"распознано"}`))
		case "/tts":
			var body struct {
				Text string `json:"text"`
				Seed int    `json:"seed"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text != "Привет!!!" || body.Seed != 7 {
				t.Errorf("unexpected request: %+v %v", body, err)
			}
			w.Write(wav())
		default:
			t.Error("unexpected path")
		}
	}))
	defer server.Close()
	p := HTTP{STT: map[string]STT{"stt": {URL: server.URL + "/stt", Model: "example", Timeout: time.Second}}, TTS: map[string]TTS{"tts": {URL: server.URL + "/tts", Timeout: time.Second, Profiles: map[string]Options{"voice": {Seed: 7}}}}}
	text, err := p.Transcribe(context.Background(), "stt", []byte("audio"), "audio/ogg")
	if err != nil || text != "распознано" {
		t.Fatalf("stt %q %v", text, err)
	}
	data, err := p.Synthesize(context.Background(), "tts", "voice", "Привет")
	if err != nil || len(data) != 46 {
		t.Fatalf("tts %d %v", len(data), err)
	}
}

func TestProxyTimeoutDoesNotProveExecutionStopped(t *testing.T) {
	failure := responseFailure(&http.Response{StatusCode: http.StatusGatewayTimeout}, false).(*Failure)
	if !failure.Uncertain {
		t.Fatal("proxy timeout incorrectly released model slot")
	}
	unavailable := responseFailure(&http.Response{StatusCode: http.StatusServiceUnavailable}, false).(*Failure)
	if unavailable.Uncertain {
		t.Fatal("explicit unavailable response held slot")
	}
}

func TestCancelledBeforeSendDoesNotHoldSlot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := HTTP{STT: map[string]STT{"stt": {URL: "http://localhost/stt", Timeout: time.Second}}, TTS: map[string]TTS{"tts": {URL: "http://localhost/tts", Timeout: time.Second, Profiles: map[string]Options{"voice": {}}}}}
	_, err := p.Transcribe(ctx, "stt", []byte("audio"), "audio/ogg")
	if f, ok := err.(*Failure); !ok || f.Uncertain || f.Code != codes.Canceled {
		t.Fatalf("STT: %v", err)
	}
	_, err = p.Synthesize(ctx, "tts", "voice", "text")
	if f, ok := err.(*Failure); !ok || f.Uncertain || f.Code != codes.Canceled {
		t.Fatalf("TTS: %v", err)
	}
}

func TestCancellationDuringDialDoesNotHoldSlot(t *testing.T) {
	for _, op := range []string{"stt", "tts"} {
		t.Run(op, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
				close(started)
				<-release
				return nil, context.Canceled
			}}
			defer transport.CloseIdleConnections()
			p := testProvider(&http.Client{Transport: transport}, "http://model.invalid")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- invokeProvider(p, op, ctx) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("dial did not start")
			}
			cancel()
			select {
			case err := <-done:
				if f, ok := err.(*Failure); !ok || f.Uncertain || f.Code != codes.Canceled {
					t.Fatalf("cancellation during dial: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled request did not return")
			}
		})
	}
}

func TestCancellationAfterSendKeepsSlot(t *testing.T) {
	for _, op := range []string{"stt", "tts"} {
		t.Run(op, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
			}))
			defer server.Close()
			defer close(release)
			p := testProvider(server.Client(), server.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- invokeProvider(p, op, ctx) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("request did not reach model")
			}
			cancel()
			select {
			case err := <-done:
				if f, ok := err.(*Failure); !ok || !f.Uncertain || f.Code != codes.Canceled {
					t.Fatalf("cancellation after sending request: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled request did not return")
			}
		})
	}
}

func testProvider(client *http.Client, target string) *HTTP {
	return &HTTP{Client: client,
		STT: map[string]STT{"stt": {URL: target, Timeout: time.Second}},
		TTS: map[string]TTS{"tts": {URL: target, Timeout: time.Second, Profiles: map[string]Options{"voice": {}}}},
	}
}

func invokeProvider(p *HTTP, op string, ctx context.Context) error {
	if op == "stt" {
		_, err := p.Transcribe(ctx, "stt", []byte("audio"), "audio/ogg")
		return err
	}
	_, err := p.Synthesize(ctx, "tts", "voice", "текст")
	return err
}
