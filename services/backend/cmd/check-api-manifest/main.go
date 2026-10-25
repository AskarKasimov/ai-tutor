package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type route struct {
	Mode       string `json:"mode"`
	Method     string `json:"method"`
	TargetPath string `json:"target_path"`
}

type manifest struct {
	Routes []route `json:"routes"`
}

func main() {
	manifestBytes, err := os.ReadFile("../../documents/integration-parity/api-manifest.json")
	if err != nil {
		panic(err)
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		panic(err)
	}
	apiBytes, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		panic(err)
	}
	var api struct {
		Paths map[string]map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(apiBytes, &api); err != nil {
		panic(err)
	}
	for _, r := range m.Routes {
		path := strings.TrimPrefix(r.TargetPath, "/api/v1")
		operation, ok := api.Paths[path][strings.ToLower(r.Method)]
		if !ok {
			panic(fmt.Sprintf("manifest route missing from OpenAPI: %s %s", r.Method, r.TargetPath))
		}
		if mode, ok := operation["x-mode"].(string); ok && mode != r.Mode {
			panic(fmt.Sprintf("manifest/OpenAPI mode mismatch for %s %s: %s != %s", r.Method, r.TargetPath, r.Mode, mode))
		}
		if status, ok := operation["x-implementation"].(string); !ok || status != "implemented" {
			panic(fmt.Sprintf("manifest route is not marked implemented: %s %s", r.Method, r.TargetPath))
		}
	}
	fmt.Printf("API manifest routes verified: %d\n", len(m.Routes))
}
