package naming

import "testing"

func TestExport(t *testing.T) {
	tests := map[string]string{
		"card_threeDSecure":    "CardThreeDSecure",
		"threeDSecure":         "ThreeDSecure",
		"customer_id":          "CustomerID",
		"redirect":             "Redirect",
		"udf1":                 "Udf1",
		"3ds":                  "N3ds",
		"payment_agreement":    "PaymentAgreement",
		"live_mode":            "LiveMode",
		"itemAmount":           "ItemAmount",
		"APIVersion":           "APIVersion",
		"create-a-charge":      "CreateACharge",
		"total_payments_count": "TotalPaymentsCount",
	}
	for in, want := range tests {
		if got := Export(in); got != want {
			t.Errorf("Export(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLower(t *testing.T) {
	tests := map[string]string{
		"CustomerID": "customerID",
		"ID":         "id",
		"URLList":    "urlList",
		"ChargeID":   "chargeID",
	}
	for in, want := range tests {
		if got := Lower(in); got != want {
			t.Errorf("Lower(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSingular(t *testing.T) {
	tests := map[string]string{
		"charges":  "charge",
		"entities": "entity",
		"status":   "status",
		"address":  "address",
		"items":    "item",
		"data":     "data",
	}
	for in, want := range tests {
		if got := Singular(in); got != want {
			t.Errorf("Singular(%q) = %q, want %q", in, got, want)
		}
	}
}
