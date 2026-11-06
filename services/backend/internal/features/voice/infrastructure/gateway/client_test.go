package gateway

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/voicegateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeGateway struct {
	stt   *pb.TranscribeRequest
	tts   *pb.SynthesizeRequest
	audio []byte
	err   error
}

func (f *fakeGateway) Transcribe(_ context.Context, req *pb.TranscribeRequest, _ ...grpc.CallOption) (*pb.TranscribeResponse, error) {
	f.stt = req
	return &pb.TranscribeResponse{Text: " ответ "}, f.err
}
func (f *fakeGateway) Synthesize(_ context.Context, req *pb.SynthesizeRequest, _ ...grpc.CallOption) (*pb.SynthesizeResponse, error) {
	f.tts = req
	return &pb.SynthesizeResponse{Audio: f.audio, MediaType: "audio/wav"}, f.err
}

func testWAV() []byte {
	wav := make([]byte, 46)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], 38)
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 16000)
	binary.LittleEndian.PutUint32(wav[28:], 32000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 2)
	return wav
}

func TestAdaptersUseVoiceGatewayContract(t *testing.T) {
	fake := &fakeGateway{audio: testWAV()}
	text, err := NewRecognizer(fake, time.Second).Recognize(context.Background(), []byte("sound"), "audio/ogg")
	if err != nil || text != "ответ" || string(fake.stt.Audio) != "sound" || fake.stt.MediaType != "audio/ogg" {
		t.Fatalf("recognize: %q, %v, %+v", text, err, fake.stt)
	}
	audio, err := NewSynthesizer(fake, time.Second, "geek-v1").Synthesize(context.Background(), "Привет")
	if err != nil || len(audio) != len(fake.audio) || fake.tts.Text != "Привет" || fake.tts.VoiceProfile != "geek-v1" || fake.tts.VoiceRevision != "revision" {
		t.Fatalf("synthesize: %d bytes, %v, %+v", len(audio), err, fake.tts)
	}
	fake.audio = []byte("not a WAV")
	if _, err := NewSynthesizer(fake, time.Second, "geek-v1").Synthesize(context.Background(), "test"); err == nil {
		t.Fatal("invalid audio accepted")
	}
}

func TestAdaptersReturnGatewayFailure(t *testing.T) {
	fake := &fakeGateway{err: status.Error(codes.Unavailable, "secret upstream details")}
	_, err := NewRecognizer(fake, time.Second).Recognize(context.Background(), []byte("sound"), "audio/ogg")
	if err == nil || err.Error() == "secret upstream details" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func (f *fakeGateway) DescribeVoice(context.Context, *pb.DescribeVoiceRequest, ...grpc.CallOption) (*pb.DescribeVoiceResponse, error) {
	return &pb.DescribeVoiceResponse{Revision: "revision"}, f.err
}

func TestLazyVoiceResolutionRetriesAfterGatewayRecovers(t *testing.T) {
	fake := &fakeGateway{err: status.Error(codes.Unavailable, "unavailable"), audio: testWAV()}
	synth := NewSynthesizer(fake, time.Second, "geek-v1")
	if _, err := synth.VoiceName(context.Background()); err == nil {
		t.Fatal("unavailable gateway accepted")
	}
	fake.err = nil
	if name, err := synth.VoiceName(context.Background()); err != nil || name != "geek-v1:revision" {
		t.Fatalf("name %q: %v", name, err)
	}
	if _, err := synth.Synthesize(context.Background(), "test"); err != nil || fake.tts.VoiceRevision != "revision" {
		t.Fatalf("synthesis: %v", err)
	}
}
func TestDialDoesNotContactGateway(t *testing.T) {
	_, conn, err := Dial("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
}
