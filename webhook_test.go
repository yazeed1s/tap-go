package tap

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"testing"
)

// testWebhookKey is a made-up key. The expected hashes below were computed
// independently, with a port of the PHP sample in Tap's webhook guide.
const testWebhookKey = "tap-go-webhook-test-secret"

func TestWebhookHash(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{
			// Real charge webhook body from Tap's guide. transaction.created
			// is a string here and amount 1.0 SAR hashes as "1.00".
			name: "charge",
			body: mustRead(t, "testdata/webhook_charge.json"),
			want: "e08ed03261df8375a33529e00c246aacdc687171b44876ae1457e8a1d3834978",
		},
		{
			name: "authorize",
			body: mustRead(t, "testdata/webhook_authorize.json"),
			want: "2d356bda68f88d051347dcefa0bb5d80d3d8ae95b1f6eaaf53fc4b5d8d37dfd3",
		},
		{
			// Three-decimal currency; created as a number, updated as a string.
			name: "invoice",
			body: `{"id":"inv_TS1","object":"invoice","amount":12.5,"currency":"KWD","status":"PAID","created":1698392202943,"updated":"1698392299000"}`,
			want: "b622aae2971428cd96e6460fe50767bda822431b32e976215ff7d226d4506d0e",
		},
		{
			// Refunds sign their top-level created.
			name: "refund",
			body: `{"id":"re_TS1","object":"refund","amount":2,"currency":"SAR","status":"REFUNDED","created":1698392202943,"reference":{"gateway":"gw1","payment":"pay1"}}`,
			want: "cc83408458b42f4f54b5067f3df2760d3c1358096ed35f7228f3d049633141a7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := WebhookHash(testWebhookKey, []byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("WebhookHash = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseWebhookRequest(t *testing.T) {
	body := mustRead(t, "testdata/webhook_charge.json")
	req := httptest.NewRequest(
		"POST",
		"/webhooks/tap",
		bytes.NewBufferString(body),
	)
	req.Header.Set(
		WebhookHashHeader,
		"E08ED03261DF8375A33529E00C246AACDC687171B44876AE1457E8A1D3834978",
	)

	event, err := ParseWebhookRequest(testWebhookKey, req)
	if err != nil {
		t.Fatal(err)
	}
	if event.Object != "charge" || event.Charge == nil {
		t.Fatalf("event = %+v", event)
	}
	if event.Charge.Status != ChargeStatusCaptured || event.Charge.Amount != 1 {
		t.Errorf("charge = %+v", event.Charge)
	}
	if event.Charge.Customer == nil ||
		event.Charge.Customer.ID != "cus_TS07A5420232136o2K52709053" {
		t.Errorf("customer = %+v", event.Charge.Customer)
	}
}

func TestParseWebhookRejectsTampering(t *testing.T) {
	body := mustRead(t, "testdata/webhook_charge.json")
	const hash = "e08ed03261df8375a33529e00c246aacdc687171b44876ae1457e8a1d3834978"

	tampered := bytes.Replace(
		[]byte(body),
		[]byte(`"amount":1.0`),
		[]byte(`"amount":100.0`),
		1,
	)
	if _, err := ParseWebhook(testWebhookKey, tampered, hash); !errors.Is(
		err,
		ErrInvalidWebhookSignature,
	) {
		t.Errorf("tampered body: err = %v", err)
	}
	if _, err := ParseWebhook("wrong-key", []byte(body), hash); !errors.Is(
		err,
		ErrInvalidWebhookSignature,
	) {
		t.Errorf("wrong key: err = %v", err)
	}
	if _, err := ParseWebhook(testWebhookKey, []byte(body), ""); !errors.Is(
		err,
		ErrInvalidWebhookSignature,
	) {
		t.Errorf("missing header: err = %v", err)
	}
}

func TestParseWebhookUnsupportedObject(t *testing.T) {
	body := []byte(
		`{"id":"payout_1","status":"PAID_OUT","amount":8.16,"currency":"SAR"}`,
	)
	if _, err := ParseWebhook(testWebhookKey, body, "x"); !errors.Is(
		err,
		ErrUnsupportedWebhook,
	) {
		t.Errorf("err = %v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimSpace(data))
}
