module github.com/AskarKasimov/ai-tutor/services/voice-gateway

go 1.26.4

require (
	github.com/AskarKasimov/ai-tutor/services/shared v0.0.0
	github.com/twmb/franz-go/pkg/kmsg v1.15.0
	google.golang.org/grpc v1.84.0
)

require (
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/pierrec/lz4/v4 v4.1.33 // indirect
)

require (
	github.com/twmb/franz-go v1.22.2
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/AskarKasimov/ai-tutor/services/shared => ../shared

require github.com/AskarKasimov/ai-tutor/services/audio v0.0.0

replace github.com/AskarKasimov/ai-tutor/services/audio => ../audio
