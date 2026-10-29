package main

import "fmt"

// guideSchemas are components that only Tap's guides describe. The
// marketplace guide shows the destinations object, but no reference page
// declares it.
var guideSchemas = map[string]S{
	"ChargeDestination": {
		"type": "object",
		"properties": S{
			"id":       S{"type": "string", "description": "The destination ID of the business receiving this part of the charge.", "example": "480593"},
			"amount":   S{"type": "number", "description": "The amount sent to this destination.", "example": 2},
			"currency": S{"type": "string", "description": "The currency of the amount, same as the charge.", "example": "KWD"},
		},
		"required": []any{"id", "amount", "currency"},
	},
	"ChargeDestinations": {
		"type":        "object",
		"description": "Splits the charge amount between marketplace businesses. Whatever is not sent to a destination stays with the marketplace.",
		"properties": S{
			"destination": S{"type": "array", "items": S{"$ref": schemaRef + "ChargeDestination"}},
			"amount":      S{"type": "number", "description": "The total sent to destinations. Returned in responses."},
			"currency":    S{"type": "string", "description": "Returned in responses."},
			"count":       S{"type": "integer", "description": "The number of destinations. Returned in responses."},
		},
	},
}

// guideFields adds properties that reference pages leave out, keyed by
// component name. They are applied after the build, so a later docs change
// that declares the field merges over it instead of being lost.
var guideFields = map[string]S{
	"CreateChargeRequest": {
		"destinations": S{"$ref": schemaRef + "ChargeDestinations"},
	},
	"CreateAuthorizeRequest": {
		"destinations": S{"$ref": schemaRef + "ChargeDestinations"},
	},
	"Charge": {
		"destinations": S{"$ref": schemaRef + "ChargeDestinations"},
	},
	"Authorize": {
		"destinations": S{"$ref": schemaRef + "ChargeDestinations"},
	},
	// the refund reference types the destination amount as a string, but
	// it is a decimal like every other amount.
	"CreateRefundDestinationsDestination": {
		"amount": S{"type": "number", "example": 2},
	},
	// the only sample had whole amounts, so these were inferred as
	// integers. a refund of 5.50 would fail to decode the charge.
	"ChargeRefundsSummary": {
		"charge_amount":  S{"type": "number"},
		"refund_amount":  S{"type": "number"},
		"remaing_amount": S{"type": "number"},
	},
}

// addGuideFields applies guideSchemas and guideFields to a built spec.
func addGuideFields(spec S) error {
	comps, _ := spec["components"].(S)
	schemas, _ := comps["schemas"].(S)
	if schemas == nil {
		return fmt.Errorf("spec has no components.schemas")
	}
	for name, s := range guideSchemas {
		if _, ok := schemas[name]; !ok {
			schemas[name] = clone(s)
		}
	}
	for name, fields := range guideFields {
		s, _ := schemas[name].(S)
		if s == nil {
			return fmt.Errorf("guide fields: no component %s", name)
		}
		props := propsOf(s)
		if props == nil {
			props = S{}
			s["properties"] = props
		}
		for k, v := range fields {
			props[k] = clone(v.(S))
		}
	}
	return nil
}
