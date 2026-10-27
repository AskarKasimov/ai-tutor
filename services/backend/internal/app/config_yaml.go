package app

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type yamlDuration struct{ time.Duration }

func (d *yamlDuration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("duration must be a string with units")
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

type backendYAML struct {
	Server *struct {
		ListenAddress *string `yaml:"listen_address"`
	} `yaml:"server"`
	Voice *struct {
		STT *struct {
			URL *string `yaml:"url"`
		} `yaml:"stt"`
		TTS *struct {
			URL                *string  `yaml:"url"`
			Seed               *int64   `yaml:"seed"`
			CFGValue           *float64 `yaml:"cfg_value"`
			InferenceTimesteps *int     `yaml:"inference_timesteps"`
		} `yaml:"tts"`
		Timeout *yamlDuration `yaml:"timeout"`
	} `yaml:"voice"`
	Uploads *struct {
		MaxBytes *int64 `yaml:"max_bytes"`
	} `yaml:"uploads"`
	Storage *struct {
		S3 *struct {
			Endpoint     *string       `yaml:"endpoint"`
			Region       *string       `yaml:"region"`
			Bucket       *string       `yaml:"bucket"`
			PathStyle    *bool         `yaml:"path_style"`
			Timeout      *yamlDuration `yaml:"timeout"`
			CreateBucket *bool         `yaml:"create_bucket"`
		} `yaml:"s3"`
	} `yaml:"storage"`
	Workers *struct {
		Audio *struct {
			Count        *int          `yaml:"count"`
			PollInterval *yamlDuration `yaml:"poll_interval"`
		} `yaml:"audio"`
	} `yaml:"workers"`
	Tutor *struct {
		Enabled *bool `yaml:"enabled"`
		Context *struct {
			TaskMaxChars    *int `yaml:"task_max_chars"`
			HistoryMaxChars *int `yaml:"history_max_chars"`
			SummaryMaxChars *int `yaml:"summary_max_chars"`
		} `yaml:"context"`
	} `yaml:"tutor"`
	CourseGraph *struct {
		URL     *string       `yaml:"url"`
		Timeout *yamlDuration `yaml:"timeout"`
	} `yaml:"course_graph"`
	LLMGateway *struct {
		Address *string       `yaml:"address"`
		Timeout *yamlDuration `yaml:"timeout"`
	} `yaml:"llm_gateway"`
	Geek *struct {
		QuestionLimit *int `yaml:"question_limit"`
	} `yaml:"geek"`
}

func parseBackendYAML(path string) (backendYAML, error) {
	var cfg backendYAML
	file, err := os.Open(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %q: %w", path, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return cfg, fmt.Errorf("config document is empty")
		}
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return cfg, fmt.Errorf("config must contain exactly one YAML document")
		}
		return cfg, fmt.Errorf("decode trailing config document: %w", err)
	}
	return cfg, nil
}

func requireBackendYAML(c backendYAML) error {
	missing := []string{}
	if c.Server == nil || c.Server.ListenAddress == nil {
		missing = append(missing, "server.listen_address")
	}
	if c.Voice == nil || c.Voice.STT == nil || c.Voice.STT.URL == nil {
		missing = append(missing, "voice.stt.url")
	}
	if c.Voice == nil || c.Voice.TTS == nil {
		missing = append(missing, "voice.tts")
	} else {
		if c.Voice.TTS.URL == nil {
			missing = append(missing, "voice.tts.url")
		}
		if c.Voice.TTS.Seed == nil {
			missing = append(missing, "voice.tts.seed")
		}
		if c.Voice.TTS.CFGValue == nil {
			missing = append(missing, "voice.tts.cfg_value")
		}
		if c.Voice.TTS.InferenceTimesteps == nil {
			missing = append(missing, "voice.tts.inference_timesteps")
		}
	}
	if c.Voice == nil || c.Voice.Timeout == nil {
		missing = append(missing, "voice.timeout")
	}
	if c.Uploads == nil || c.Uploads.MaxBytes == nil {
		missing = append(missing, "uploads.max_bytes")
	}
	if c.Storage == nil || c.Storage.S3 == nil {
		missing = append(missing, "storage.s3")
	} else {
		s := c.Storage.S3
		if s.Endpoint == nil {
			missing = append(missing, "storage.s3.endpoint")
		}
		if s.Region == nil {
			missing = append(missing, "storage.s3.region")
		}
		if s.Bucket == nil {
			missing = append(missing, "storage.s3.bucket")
		}
		if s.PathStyle == nil {
			missing = append(missing, "storage.s3.path_style")
		}
		if s.Timeout == nil {
			missing = append(missing, "storage.s3.timeout")
		}
		if s.CreateBucket == nil {
			missing = append(missing, "storage.s3.create_bucket")
		}
	}
	if c.Workers == nil || c.Workers.Audio == nil || c.Workers.Audio.Count == nil || c.Workers.Audio.PollInterval == nil {
		missing = append(missing, "workers.audio.count/poll_interval")
	}
	if c.Tutor == nil || c.Tutor.Enabled == nil || c.Tutor.Context == nil {
		missing = append(missing, "tutor.enabled/context")
	} else {
		x := c.Tutor.Context
		if x.TaskMaxChars == nil || x.HistoryMaxChars == nil || x.SummaryMaxChars == nil {
			missing = append(missing, "tutor.context limits")
		}
	}
	if c.CourseGraph == nil || c.CourseGraph.URL == nil || c.CourseGraph.Timeout == nil {
		missing = append(missing, "course_graph.url/timeout")
	}
	if c.LLMGateway == nil || c.LLMGateway.Address == nil || c.LLMGateway.Timeout == nil {
		missing = append(missing, "llm_gateway.address/timeout")
	}
	if c.Geek == nil || c.Geek.QuestionLimit == nil {
		missing = append(missing, "geek.question_limit")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// LoadConfig reads application settings from a strict, single-document YAML file;
// only database and object-store credentials are read from the environment.
func LoadConfig(path string) (Config, error) {
	y, err := parseBackendYAML(path)
	if err != nil {
		return Config{}, err
	}
	if err := requireBackendYAML(y); err != nil {
		return Config{}, err
	}
	c := Config{ListenAddress: *y.Server.ListenAddress, STTURL: *y.Voice.STT.URL, TTSURL: *y.Voice.TTS.URL, TTSSeed: *y.Voice.TTS.Seed, TTSCFGValue: *y.Voice.TTS.CFGValue, TTSInferenceTimesteps: *y.Voice.TTS.InferenceTimesteps, VoiceTimeout: y.Voice.Timeout.Duration, MaxUploadBytes: *y.Uploads.MaxBytes,
		S3Endpoint: *y.Storage.S3.Endpoint, S3Region: *y.Storage.S3.Region, S3Bucket: *y.Storage.S3.Bucket, S3PathStyle: *y.Storage.S3.PathStyle, S3Timeout: y.Storage.S3.Timeout.Duration, S3CreateBucket: *y.Storage.S3.CreateBucket, S3AccessKey: strings.TrimSpace(os.Getenv("BACKEND_S3_ACCESS_KEY")), S3SecretKey: strings.TrimSpace(os.Getenv("BACKEND_S3_SECRET_KEY")),
		AudioWorkers: *y.Workers.Audio.Count, AudioPollInterval: y.Workers.Audio.PollInterval.Duration, LearningTutorEnabled: *y.Tutor.Enabled, TutorTaskMaxChars: *y.Tutor.Context.TaskMaxChars, TutorHistoryMaxChars: *y.Tutor.Context.HistoryMaxChars, TutorSummaryMaxChars: *y.Tutor.Context.SummaryMaxChars,
		CourseGraphURL: *y.CourseGraph.URL, CourseGraphTimeout: y.CourseGraph.Timeout.Duration, LLMGatewayAddress: *y.LLMGateway.Address, LLMTimeout: y.LLMGateway.Timeout.Duration, GeekMaxQuestions: *y.Geek.QuestionLimit}
	var missing []string
	c.DatabaseURL = strings.TrimSpace(os.Getenv("BACKEND_DATABASE_URL"))
	if c.DatabaseURL == "" {
		missing = append(missing, "BACKEND_DATABASE_URL")
	}
	if c.S3AccessKey == "" {
		missing = append(missing, "BACKEND_S3_ACCESS_KEY")
	}
	if c.S3SecretKey == "" {
		missing = append(missing, "BACKEND_S3_SECRET_KEY")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("required secrets missing: %s", strings.Join(missing, ", "))
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	c.OCRURL = c.CourseGraphURL
	c.OCRTimeout = c.CourseGraphTimeout
	return c, nil
}

// LoadLLMGatewayConfig reads and validates the CLI-required gateway section
// without requiring database or object-store credentials.
func LoadLLMGatewayConfig(path string) (string, time.Duration, error) {
	y, err := parseBackendYAML(path)
	if err != nil {
		return "", 0, err
	}
	if y.LLMGateway == nil || y.LLMGateway.Address == nil || y.LLMGateway.Timeout == nil {
		return "", 0, fmt.Errorf("config requires llm_gateway.address and llm_gateway.timeout")
	}
	if strings.TrimSpace(*y.LLMGateway.Address) == "" {
		return "", 0, fmt.Errorf("llm_gateway.address is required")
	}
	host, port, err := net.SplitHostPort(*y.LLMGateway.Address)
	if err != nil || host == "" || !validPort(port) {
		return "", 0, fmt.Errorf("llm_gateway.address must be a host:port gRPC address")
	}
	if y.LLMGateway.Timeout.Duration <= 0 {
		return "", 0, fmt.Errorf("llm_gateway.timeout must be positive")
	}
	return *y.LLMGateway.Address, y.LLMGateway.Timeout.Duration, nil
}

// LoadListenAddress reads the HTTP address without requiring database or S3 secrets.
func LoadListenAddress(path string) (string, error) {
	y, err := parseBackendYAML(path)
	if err != nil {
		return "", err
	}
	if y.Server == nil || y.Server.ListenAddress == nil {
		return "", fmt.Errorf("config requires server.listen_address")
	}
	if _, port, err := net.SplitHostPort(*y.Server.ListenAddress); err != nil || !validPort(port) {
		return "", fmt.Errorf("server.listen_address must be a host:port address")
	}
	return *y.Server.ListenAddress, nil
}
