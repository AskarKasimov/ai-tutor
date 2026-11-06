package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedConfigAndVoiceRevision(t *testing.T) {
	c, err := Load("../../config/dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	v := c.Voices["geek-default"]
	first := v.Revision()
	v.Seed++
	if first == v.Revision() {
		t.Fatal("seed omitted from revision")
	}
	v = c.Voices["geek-default"]
	v.Identity = "another reference"
	if first == v.Revision() {
		t.Fatal("voice identity omitted")
	}
}
func TestInvalidConfiguration(t *testing.T) {
	body, err := os.ReadFile("../../config/dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"unknown":       string(body) + "unknown: true\n",
		"duplicate":     string(body) + "log_level: warn\n",
		"multi":         string(body) + "---\nlog_level: info\n",
		"bad bool":      strings.Replace(string(body), "longform: false", "longform: yes", 1),
		"string number": strings.Replace(string(body), "slots: 1", "slots: '1'", 1),
		"float slots":   strings.Replace(string(body), "slots: 1", "slots: 1.5", 1),
		"empty pool":    strings.Replace(string(body), "max_waiting: 32", "max_waiting: 0", 1),
		"bad time":      strings.Replace(string(body), "request_timeout: 120s", "request_timeout: 120", 1),
		"same topics":   strings.Replace(string(body), "voice.tts.v1", "voice.stt.v1", 1),
		"bad profile":   strings.Replace(string(body), "profiles: [geek-default]", "profiles: [absent]", 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = Load(path); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
