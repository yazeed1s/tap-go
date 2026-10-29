package tap

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// WebhookHashHeader is the request header carrying Tap's webhook signature.
const WebhookHashHeader = "hashstring"

// maxWebhookBody caps the body ParseWebhookRequest reads.
const maxWebhookBody = 1 << 20

var (
	// ErrInvalidWebhookSignature is returned when the hashstring header does
	// not match the body.
	ErrInvalidWebhookSignature = errors.New("tap: invalid webhook signature")
	// ErrUnsupportedWebhook is returned for webhook bodies whose object Tap
	// does not sign, such as payout notifications.
	ErrUnsupportedWebhook = errors.New("tap: webhook object cannot be verified")
)

// WebhookEvent is a verified webhook. Tap posts the full resource as the
// body (to the post.url given when it was created); Object says which kind
// it is and the matching field is set.
type WebhookEvent struct {
	// Object is "charge", "authorize", "refund", or "invoice".
	Object    string
	Charge    *Charge
	Authorize *Authorize
	Refund    *Refund
	Invoice   *Invoice
	// Raw is the body as received.
	Raw json.RawMessage
}

// ParseWebhookRequest reads a webhook request, verifies its hashstring
// header with secretKey, and decodes the body.
func ParseWebhookRequest(
	secretKey string,
	r *http.Request,
) (*WebhookEvent, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		return nil, fmt.Errorf("tap: read webhook body: %w", err)
	}
	return ParseWebhook(secretKey, body, r.Header.Get(WebhookHashHeader))
}

// ParseWebhook verifies a webhook body against its hashstring header and
// decodes it.
func ParseWebhook(
	secretKey string,
	body []byte,
	hashstring string,
) (*WebhookEvent, error) {
	if err := VerifyWebhook(secretKey, body, hashstring); err != nil {
		return nil, err
	}
	event := &WebhookEvent{Raw: append(json.RawMessage(nil), body...)}
	var head struct {
		Object string `json:"object"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		return nil, fmt.Errorf("tap: decode webhook: %w", err)
	}
	event.Object = head.Object
	var target any
	switch head.Object {
	case "charge":
		event.Charge = new(Charge)
		target = event.Charge
	case "authorize":
		event.Authorize = new(Authorize)
		target = event.Authorize
	case "refund":
		event.Refund = new(Refund)
		target = event.Refund
	case "invoice":
		event.Invoice = new(Invoice)
		target = event.Invoice
	}
	if err := json.Unmarshal(body, target); err != nil {
		return nil, fmt.Errorf("tap: decode %s webhook: %w", head.Object, err)
	}
	return event, nil
}

// VerifyWebhook checks a webhook body against the hashstring header Tap sent
// with it. It returns ErrInvalidWebhookSignature on a mismatch.
func VerifyWebhook(secretKey string, body []byte, hashstring string) error {
	want, err := WebhookHash(secretKey, body)
	if err != nil {
		return err
	}
	got := strings.ToLower(strings.TrimSpace(hashstring))
	if !hmac.Equal([]byte(want), []byte(got)) {
		return ErrInvalidWebhookSignature
	}
	return nil
}

// WebhookHash computes the hashstring Tap sends for a webhook body: a
// hex HMAC-SHA256, keyed with the secret key, of selected fields of the
// charge, authorize, refund, or invoice.
//
// See https://developers.tap.company/docs/webhook.
func WebhookHash(secretKey string, body []byte) (string, error) {
	message, err := webhookMessage(body)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// webhookFields holds the signed fields as raw JSON, so values Tap sends
// sometimes as numbers and sometimes as strings ("created") are hashed
// exactly as received.
type webhookFields struct {
	Object    string          `json:"object"`
	ID        json.RawMessage `json:"id"`
	Amount    json.RawMessage `json:"amount"`
	Currency  json.RawMessage `json:"currency"`
	Status    json.RawMessage `json:"status"`
	Created   json.RawMessage `json:"created"`
	Updated   json.RawMessage `json:"updated"`
	Reference struct {
		Gateway json.RawMessage `json:"gateway"`
		Payment json.RawMessage `json:"payment"`
	} `json:"reference"`
	Transaction struct {
		Created json.RawMessage `json:"created"`
	} `json:"transaction"`
}

func webhookMessage(body []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var f webhookFields
	if err := dec.Decode(&f); err != nil {
		return "", fmt.Errorf("tap: decode webhook: %w", err)
	}
	currency := rawText(f.Currency)
	amount, err := formatAmount(rawText(f.Amount), currency)
	if err != nil {
		return "", err
	}

	var created string
	switch f.Object {
	case "charge", "authorize":
		created = rawText(f.Transaction.Created)
	case "refund":
		created = rawText(f.Created)
	case "invoice":
		return "x_id" + rawText(f.ID) +
			"x_amount" + amount +
			"x_currency" + currency +
			"x_updated" + rawText(f.Updated) +
			"x_status" + rawText(f.Status) +
			"x_created" + rawText(f.Created), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedWebhook, f.Object)
	}
	return "x_id" + rawText(f.ID) +
		"x_amount" + amount +
		"x_currency" + currency +
		"x_gateway_reference" + rawText(f.Reference.Gateway) +
		"x_payment_reference" + rawText(f.Reference.Payment) +
		"x_status" + rawText(f.Status) +
		"x_created" + created, nil
}

// rawText returns a JSON string without quotes, a number as written, and
// null or a missing value as "".
func rawText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var s string
	if raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// formatAmount rounds an amount to the currency's minor unit, as Tap does
// before hashing: 1.0 SAR is "1.00" and 3 KWD is "3.000".
func formatAmount(amount, currency string) (string, error) {
	v, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return "", fmt.Errorf("tap: webhook amount %q: %w", amount, err)
	}
	return strconv.FormatFloat(v, 'f', currencyDecimals(currency), 64), nil
}

// currencyDecimals is the ISO 4217 minor unit of a currency.
func currencyDecimals(currency string) int {
	switch strings.ToUpper(currency) {
	case "BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND":
		return 3
	case "JPY", "KRW":
		return 0
	}
	return 2
}
