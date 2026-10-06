// merge-openapi refreshes the implemented part of the single API contract.
package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type object = map[string]any

func read(path string) (object, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc object
	err = yaml.Unmarshal(data, &doc)
	return doc, err
}

func merge(contract, generated object) error {
	paths := contract["paths"].(object)
	currentPaths := generated["paths"].(object)
	for path, value := range paths {
		for method, operation := range value.(object) {
			if !strings.Contains(" get post put patch delete head options ", " "+method+" ") {
				continue
			}
			design := operation.(object)
			switch design["x-implementation"] {
			case "planned":
				continue
			case "implemented":
			default:
				return fmt.Errorf("%s %s: require x-implementation implemented or planned", method, path)
			}
			generatedPath, ok := currentPaths[path].(object)
			if !ok {
				return fmt.Errorf("implemented route absent in generated API: %s %s", method, path)
			}
			actual, ok := generatedPath[method].(object)
			if !ok {
				return fmt.Errorf("implemented method absent in generated API: %s %s", method, path)
			}
			for key, value := range design {
				if strings.HasPrefix(key, "x-") || key == "deprecated" {
					actual[key] = value
				}
			}
			value.(object)[method] = actual
		}
	}
	components := contract["components"].(object)
	schemas := components["schemas"].(object)
	for name, schema := range generated["components"].(object)["schemas"].(object) {
		schemas[name] = schema
	}
	used := map[string]bool{}
	var visit func(any) error
	visit = func(node any) error {
		switch value := node.(type) {
		case object:
			if ref, ok := value["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
				name := strings.TrimPrefix(ref, "#/components/schemas/")
				if !used[name] {
					schema, ok := schemas[name]
					if !ok {
						return fmt.Errorf("missing schema: %s", ref)
					}
					used[name] = true
					if err := visit(schema); err != nil {
						return err
					}
				}
			}
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(paths); err != nil {
		return err
	}
	for name := range schemas {
		if !used[name] {
			delete(schemas, name)
		}
	}
	return nil
}

func run() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("usage: merge-openapi contract.yaml implemented.yaml output.yaml")
	}
	contract, err := read(os.Args[1])
	if err != nil {
		return err
	}
	generated, err := read(os.Args[2])
	if err != nil {
		return err
	}
	if err := merge(contract, generated); err != nil {
		return err
	}
	file, err := os.Create(os.Args[3])
	if err != nil {
		return err
	}
	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	err = encoder.Encode(contract)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
