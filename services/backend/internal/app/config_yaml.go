package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
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
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %q: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return cfg, fmt.Errorf("config document is empty")
		}
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	// yaml.v3 accepts some scalar coercions (for example, a float for an int).
	// Validate the YAML tags before decoding so typed settings cannot silently change.
	if err := validateTypedYAML(data); err != nil {
		return cfg, err
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

func validateTypedYAML(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if err := rejectYAMLMerge(&root); err != nil {
		return err
	}
	return validateYAMLNode(root.Content[0], reflect.TypeOf(backendYAML{}), "config")
}

// Merge keys can hide scalar coercions from the typed-node validator.
func rejectYAMLMerge(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Tag == "!!merge" {
				return fmt.Errorf("YAML merge keys are not supported (line %d)", node.Content[i].Line)
			}
		}
	}
	for _, child := range node.Content {
		if err := rejectYAMLMerge(child); err != nil {
			return err
		}
	}
	return nil
}

func validateYAMLNode(node *yaml.Node, typ reflect.Type, path string) error {
	pointer := false
	for typ.Kind() == reflect.Pointer {
		pointer = true
		typ = typ.Elem()
	}
	if typ == reflect.TypeOf(yamlDuration{}) {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return fmt.Errorf("%s must be a duration string", path)
		}
		if _, err := time.ParseDuration(node.Value); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	}
	if node.Tag == "!!null" {
		if pointer {
			return nil
		}
		return fmt.Errorf("%s cannot be null", path)
	}
	switch typ.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("%s must be a mapping", path)
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			field, ok := fields[key.Value]
			if !ok {
				continue
			}
			if err := validateYAMLNode(node.Content[i+1], field, path+"."+key.Value); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a sequence", path)
		}
		for i, child := range node.Content {
			if err := validateYAMLNode(child, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("%s must be a mapping", path)
		}
		if typ.Elem().Kind() == reflect.Interface {
			return nil
		} // Free-form JSON body values.
		for i := 0; i < len(node.Content); i += 2 {
			if err := validateYAMLNode(node.Content[i+1], typ.Elem(), path+"."+node.Content[i].Value); err != nil {
				return err
			}
		}
	case reflect.String:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return fmt.Errorf("%s must be a string", path)
		}
	case reflect.Bool:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return fmt.Errorf("%s must be an integer", path)
		}
	case reflect.Float32, reflect.Float64:
		if node.Kind != yaml.ScalarNode || (node.Tag != "!!int" && node.Tag != "!!float") {
			return fmt.Errorf("%s must be numeric", path)
		}
	}
	return nil
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
		return Config{}, yamlConfigValidationError(err)
	}
	c.OCRURL = c.CourseGraphURL
	c.OCRTimeout = c.CourseGraphTimeout
	return c, nil
}

// YAML-backed application settings use YAML paths in diagnostics. ConfigFromEnv
// retains the legacy environment-variable diagnostics for existing callers.
func yamlConfigValidationError(err error) error {
	message := err.Error()
	for _, field := range []struct{ env, path string }{
		{"BACKEND_LISTEN_ADDRESS", "server.listen_address"},
		{"BACKEND_STT_URL", "voice.stt.url"},
		{"BACKEND_TTS_URL", "voice.tts.url"},
		{"BACKEND_TTS_CFG_VALUE", "voice.tts.cfg_value"},
		{"BACKEND_TTS_INFERENCE_TIMESTEPS", "voice.tts.inference_timesteps"},
		{"BACKEND_VOICE_TIMEOUT", "voice.timeout"},
		{"BACKEND_S3_BUCKET", "storage.s3.bucket"},
		{"BACKEND_S3_REGION", "storage.s3.region"},
		{"BACKEND_S3_ENDPOINT", "storage.s3.endpoint"},
		{"BACKEND_S3_TIMEOUT", "storage.s3.timeout"},
		{"BACKEND_AUDIO_WORKERS", "workers.audio.count"},
		{"BACKEND_AUDIO_POLL_INTERVAL", "workers.audio.poll_interval"},
		{"learning tutor context limits", "tutor.context limits"},
		{"BACKEND_LLM_GATEWAY_ADDR", "llm_gateway.address"},
		{"BACKEND_LLM_TIMEOUT", "llm_gateway.timeout"},
		{"BACKEND_COURSE_GRAPH_TIMEOUT", "course_graph.timeout"},
		{"BACKEND_GEEK_QUESTION_LIMIT", "geek.question_limit"},
		{"BACKEND_MAX_UPLOAD_BYTES", "uploads.max_bytes"},
	} {
		message = strings.ReplaceAll(message, field.env, field.path)
	}
	return fmt.Errorf("invalid backend config: %s", message)
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
