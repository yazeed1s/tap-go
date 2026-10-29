package tap

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

type specFile struct {
	Paths map[string]map[string]struct {
		OperationID string `json:"operationId"`
		Responses   map[string]struct {
			Content map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
				Examples map[string]struct {
					Value json.RawMessage `json:"value"`
				} `json:"examples"`
			} `json:"content"`
		} `json:"responses"`
	} `json:"paths"`
	Components struct {
		Schemas map[string]struct {
			Examples []json.RawMessage `json:"examples"`
		} `json:"schemas"`
	} `json:"components"`
}

func loadSpec(t *testing.T) specFile {
	t.Helper()
	data, err := os.ReadFile("ref/api-spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec specFile
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

// decodeStrict decodes data into the generated type for schema, rejecting
// fields the type does not have.
func decodeStrict(t *testing.T, schema string, data []byte) error {
	t.Helper()
	newType, ok := schemaTypes[schema]
	if !ok {
		t.Fatalf("no generated type for schema %s", schema)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(newType())
}

// TestSpecResponseExamples decodes every response example in the spec into
// the generated Go type. It catches fields typed wrongly by the docs (an
// "integer" amount that is really 1.000) and fields the types are missing.
func TestSpecResponseExamples(t *testing.T) {
	spec := loadSpec(t)
	checked := 0
	for path, item := range spec.Paths {
		for method, op := range item {
			for code, resp := range op.Responses {
				mt, ok := resp.Content["application/json"]
				if !strings.HasPrefix(code, "2") || !ok {
					continue
				}
				name := mt.Schema.Ref[strings.LastIndex(mt.Schema.Ref, "/")+1:]
				exNames := make([]string, 0, len(mt.Examples))
				for n := range mt.Examples {
					exNames = append(exNames, n)
				}
				sort.Strings(exNames)
				for _, n := range exNames {
					if err := decodeStrict(t, name, mt.Examples[n].Value); err != nil {
						t.Errorf("%s %s %s (%s) %s: decode into %s: %v",
							method, path, op.OperationID, code, n, name, err)
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no response examples found in spec")
	}
	t.Logf("decoded %d response examples", checked)
}

// TestSpecSchemaSamples decodes the response bodies that buildspec collected
// from Tap's guides (webhook payloads, payment method walkthroughs).
func TestSpecSchemaSamples(t *testing.T) {
	spec := loadSpec(t)
	checked := 0
	for name, schema := range spec.Components.Schemas {
		for i, sample := range schema.Examples {
			if err := decodeStrict(t, name, sample); err != nil {
				t.Errorf("%s sample %d: %v", name, i, err)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no schema samples found in spec")
	}
	t.Logf("decoded %d docs samples", checked)
}
