package main

import (
	"reflect"
	"testing"
)

func mustDecode(t *testing.T, data string) any {
	t.Helper()
	var v any
	if err := decode(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestInferKeepsDecimalAmounts(t *testing.T) {
	s := infer(mustDecode(t, `{"amount": 1, "fee": 0.250, "created": 1697000000000}`), "")
	props := propsOf(s)
	for key, want := range map[string]string{"amount": "number", "fee": "number", "created": "integer"} {
		if got := typeOf(props[key].(S)); got != want {
			t.Errorf("%s: type %q, want %q", key, got, want)
		}
	}
}

func TestFixSchemaCorrectsDocTypes(t *testing.T) {
	s := fixSchema(S{"type": "object", "properties": S{
		"amount":   S{"type": "integer", "format": "int32", "default": 1},
		"cvc":      S{"type": "integer", "example": json123()},
		"metadata": S{"type": "object", "properties": S{"udf1": S{"type": "string"}}},
		"phone": S{"type": "object", "properties": S{
			"number": S{"type": "integer"},
		}},
	}}, "", "")
	props := propsOf(s)
	if amount := props["amount"].(S); amount["type"] != "number" || amount["format"] != nil || amount["example"] != 1 {
		t.Errorf("amount = %v", amount)
	}
	if cvc := props["cvc"].(S); cvc["type"] != "string" || cvc["example"] != "123" {
		t.Errorf("cvc = %v", cvc)
	}
	if md := props["metadata"].(S); md["properties"] != nil || !reflect.DeepEqual(md["additionalProperties"], S{"type": "string"}) {
		t.Errorf("metadata = %v", md)
	}
	if n := propsOf(props["phone"].(S))["number"].(S); n["type"] != "string" {
		t.Errorf("phone.number = %v", n)
	}
}

func json123() any { return 123 }

func TestMergeConflictingTypesBecomeUntyped(t *testing.T) {
	got := merge(S{"type": "string", "description": "d"}, S{"type": "array", "items": S{}})
	if !reflect.DeepEqual(got, S{"description": "d", mixedTypes: []any{"array", "string"}}) {
		t.Errorf("got %v", got)
	}
	if got := merge(S{"type": "integer"}, S{"type": "number"}); got["type"] != "number" {
		t.Errorf("integer+number = %v", got)
	}
}

func TestRefMeetsArrayBecomesUntyped(t *testing.T) {
	b := &builder{schemas: map[string]S{}}
	b.define("Lead", infer(mustDecode(t, `{"name": {"en": "x"}}`), ""))
	b.define("Lead", infer(mustDecode(t, `{"name": ["x"]}`), ""))
	if name := propsOf(b.schemas["Lead"])["name"].(S); typeOf(name) != "" || name["$ref"] != nil {
		t.Errorf("name = %v", name)
	}
}

func TestPolymorphicFieldKeepsOwnSchema(t *testing.T) {
	b := &builder{schemas: map[string]S{}}
	b.define("Charge", infer(mustDecode(t, `{"source": {"object": "token", "id": "tok_1"}}`), ""))
	b.define("Charge", infer(mustDecode(t, `{"source": {"object": "source", "id": "src_all", "on_file": false}}`), ""))
	src := propsOf(b.schemas["Charge"])["source"].(S)
	if src["$ref"] != schemaRef+"ChargeSource" {
		t.Fatalf("source = %v", src)
	}
	if _, ok := propsOf(b.schemas["ChargeSource"])["on_file"]; !ok {
		t.Errorf("ChargeSource lost on_file: %v", b.schemas["ChargeSource"])
	}
	if _, ok := b.schemas["Token"]; ok {
		t.Errorf("source was mapped to Token")
	}
}

func TestNamedFieldUsesSharedObjectSchema(t *testing.T) {
	b := &builder{schemas: map[string]S{}}
	b.define("ChargeList", infer(mustDecode(t, `{"charges": [{"object": "charge", "id": "chg_1"}]}`), ""))
	items := propsOf(b.schemas["ChargeList"])["charges"].(S)["items"].(S)
	if items["$ref"] != schemaRef+"Charge" {
		t.Errorf("items = %v", items)
	}
}

func TestDedupeNamesBySharedSuffix(t *testing.T) {
	phone := S{"type": "object", "properties": S{"number": S{"type": "string"}}}
	b := &builder{schemas: map[string]S{
		"CreateChargeCustomerPhone": clone(phone),
		"InvoiceCustomerPhone":      clone(phone),
		"Charge": {"type": "object", "properties": S{
			"phone": S{"$ref": schemaRef + "CreateChargeCustomerPhone"},
		}},
	}}
	paths := S{}
	b.dedupe(paths, map[string]bool{"Charge": true})
	if _, ok := b.schemas["CustomerPhone"]; !ok || len(b.schemas) != 2 {
		t.Fatalf("schemas = %v", b.schemas)
	}
	if ref := propsOf(b.schemas["Charge"])["phone"].(S)["$ref"]; ref != schemaRef+"CustomerPhone" {
		t.Errorf("ref not rewritten: %v", ref)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := map[string]string{
		"/v2/disputes//download": "/v2/disputes/download",
		"/v2/customers/list/":    "/v2/customers/list",
		"/v2/charges/":           "/v2/charges",
	}
	for in, want := range tests {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMixedTypesAreSticky(t *testing.T) {
	// string, then integer: conflict. A later integer must not re-type it,
	// whatever order the examples arrive in.
	s := merge(merge(S{"type": "string"}, S{"type": "integer"}), S{"type": "integer"})
	if typeOf(s) != "" || !reflect.DeepEqual(s[mixedTypes], []any{"integer", "string"}) {
		t.Errorf("got %v", s)
	}
	// null examples carry no information and must not mark a conflict.
	if s := merge(infer(nil, ""), S{"type": "integer"}); typeOf(s) != "integer" {
		t.Errorf("null then integer = %v", s)
	}
}

func TestRedactKeys(t *testing.T) {
	// Keys are assembled at runtime so no key-shaped literal sits in the
	// source for secret scanners to flag.
	key := func(kind, mode, rest string) string { return kind + "_" + mode + "_" + rest }
	spec := S{
		"description": "Use " + key("sk", "test", "abcd1234EFGH") + " or " + key("pk", "live", "ab12***cd34") + ".",
		"examples":    []any{"Bearer " + key("sk", "live", "abcd1234"), S{"key": key("sk", "test", "ZZZZ9999")}},
	}
	redactKeys(spec)
	want := S{
		"description": "Use " + key("sk", "test", "xxx") + " or " + key("pk", "live", "xxx") + ".",
		"examples":    []any{"Bearer " + key("sk", "live", "xxx"), S{"key": key("sk", "test", "xxx")}},
	}
	if !reflect.DeepEqual(spec, want) {
		t.Errorf("got %v", spec)
	}
}
