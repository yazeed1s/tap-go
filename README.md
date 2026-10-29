# tap-go

Go SDK for Tap Payments.

Services and types are generated from an OpenAPI spec that this repo builds
from Tap's API reference, because Tap does not publish one.

## Links

- [Tap Payments](https://www.tap.company/)
- [Tap API reference](https://developers.tap.company/reference)

## Install

```sh
go get github.com/yazeed1s/tap-go
```

## Create a client

```go
client := tap.NewClient("sk_test_xxx")
```

The SDK sends the secret key as a Bearer token like the Tap docs say.

Options:

```go
client := tap.NewClient(
	"sk_test_xxx",
	tap.WithHTTPClient(http.DefaultClient),
	tap.WithLanguage("ar"), // lang_code header for hosted pages
)
```

## Create a charge

```go
charge, err := client.Charges.Create(ctx, tap.CreateChargeRequest{
	Amount:   1,
	Currency: "KWD",
	Customer: tap.CreateChargeCustomer{
		FirstName: "Test",
		Email:     "test@example.com",
	},
	Source:   tap.CreateChargeSource{ID: tap.SourceAll},
	Redirect: tap.Redirect{URL: "https://example.com/return"},
})
```

Amounts are `float64` because Tap amounts are decimals (`1.000` KWD).

Optional booleans are `*bool`, so an explicit `false` is still sent. This
matters for flags Tap defaults to true, like `threeDSecure`.

Source IDs are constants: `tap.SourceAll`, `tap.SourceCard`, `tap.SourceKNET`,
`tap.SourceMada`, `tap.SourceBenefit`, `tap.SourceSTCPay`, `tap.SourceApplePay`,
and so on. A token ID (`tok_...`) or authorize ID (`auth_...`) also works.

## Statuses

Statuses are typed, with constants for every documented value:

```go
switch charge.Status {
case tap.ChargeStatusCaptured:
	// paid
case tap.ChargeStatusInitiated:
	// send the payer to charge.Transaction.URL
default:
	// failed: ABANDONED, CANCELLED, DECLINED, ...
}
```

`ChargeStatus`, `AuthorizeStatus`, `RefundStatus`, `InvoiceStatus`,
`InvoiceTrackStatus`, and `PayoutStatus` are string types, so a value Tap adds
later still decodes.

Tap sends some numbers, mostly timestamps, sometimes as JSON numbers and
sometimes as strings. Those fields are `tap.FlexNumber`, which accepts both:

```go
created, err := token.Created.Time() // Unix milliseconds
```

## Services

- `Charges`
- `Authorize`
- `Refunds`
- `Tokens`
- `Customers`
- `Cards`
- `Invoices`
- `Intents`
- `Businesses`
- `Merchants`
- `Destinations`
- `Files`
- `Payouts`
- `Disputes`
- `Leads`
- `Connect`

Download endpoints (`Charges.Download`, `Refunds.Download`, ...) return the
CSV body as `[]byte`.

## Pagination

Lists that page have a `ListAll` iterator that fetches pages as you go:

```go
for charge, err := range client.Charges.ListAll(ctx, tap.ListChargesRequest{Limit: 50}) {
	if err != nil {
		return err
	}
	fmt.Println(charge.ID, charge.Status)
}
```

Tap uses two paging styles, and the iterator handles both: a cursor
(`starting_after`) for charges and refunds, and page numbers for intents and
destinations. Authorize, customer, and invoice lists page too, but Tap does
not document their response, so they have no iterator yet.

## Marketplace splits

`destinations` sends part of a charge to marketplace businesses. Whatever is
not sent to a destination stays with the marketplace account:

```go
charge, err := client.Charges.Create(ctx, tap.CreateChargeRequest{
	// ...
	Destinations: &tap.ChargeDestinations{Destination: []tap.ChargeDestination{
		{ID: "480593", Amount: 9.5, Currency: "SAR"},
	}},
})
```

Only Tap's marketplace guide shows this object, so it comes from
`guideFields` in `internal/cmd/buildspec`, not from a reference page. To add
guide-only fields to the current spec without fetching the docs:

```sh
go run ./internal/cmd/buildspec -guide-fields
```

## Webhooks

Tap posts the charge, authorize, refund, or invoice to the `post.url` you set
when creating it, signed with a `hashstring` header.

```go
func handleTap(w http.ResponseWriter, r *http.Request) {
	event, err := tap.ParseWebhookRequest(secretKey, r)
	if err != nil {
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	switch event.Object {
	case "charge":
		fmt.Println(event.Charge.ID, event.Charge.Status)
	case "refund":
		fmt.Println(event.Refund.ID, event.Refund.Status)
	}
}
```

`ParseWebhookRequest` rejects a body whose `hashstring` does not match,
returning `tap.ErrInvalidWebhookSignature`. `VerifyWebhook` and `WebhookHash`
check or compute the signature on their own.

Payout webhooks are not signed by Tap, so they return
`tap.ErrUnsupportedWebhook`. Decode those bodies yourself.

## Sandbox test cards

The SDK has Tap's sandbox test cards. For credit and debit cards, the expiry
date picks the outcome and the CVV picks the CVV check result:

```go
tap.TestCardVisa3DS
tap.TestCardMada3DS
tap.TestExpiryApproved  // 01/39
tap.TestExpiryDeclined  // 05/22
tap.TestCVVMatch        // 100
```

Local payment method cards (KNET, Benefit, NAPS/QPay, Jaywan) are in
`tap.TestCards` with their PIN and expected status. These only work with a
`sk_test_` key. See
[Tap test cards](https://developers.tap.company/reference/testing-cards).

## Errors

When Tap returns a non-2xx response, the SDK returns `*tap.APIError`.

```go
var apiErr *tap.APIError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.StatusCode)
	fmt.Println(apiErr.Errors) // [{Code Error Description}]
	fmt.Println(apiErr.Message)
	fmt.Println(string(apiErr.Body))
}
```

## Examples

```sh
TAP_SECRET_KEY=sk_test_xxx go run ./examples/create-charge
```

## How the spec is built

Tap's docs are hosted on ReadMe. Every API reference page has a small
OpenAPI document embedded in its markdown
(`https://developers.tap.company/reference/<page>.md`). The resource path is
in `servers[0]`, the schemas are inline, and about half of the responses only
have a JSON example.

`internal/cmd/buildspec` turns those pages into `ref/api-spec.json`:

1. Finds every reference page and guide page from the docs sidebars and
   `llms.txt`.
2. Pulls the embedded OpenAPI document out of each reference page.
3. Rebases each operation onto `https://api.tap.company`.
4. Merges declared schemas with schemas inferred from response examples.
5. Collects real response bodies from the prose of every page: object
   overviews, payment method guides (Benefit, STC Pay, QPay, ...), and the
   webhook guide. A block counts if it is a list (`has_more` plus a known
   array) or a resource (a known `"object"` value). These are kept as schema
   `examples`.
6. Moves inline objects into named components and merges identical ones
   (every `{country_code, number}` phone becomes `Phone`).
7. Fixes types the docs get wrong: decimal amounts typed as integers, card
   numbers typed as `int32`, phone numbers typed as integers, `limit` and
   `page` typed as strings.
8. Fields whose samples disagree on type are marked with
   `x-tap-mixed-types`. The generator types number-or-string fields as
   `FlexNumber` and anything else as `any`.
9. Adds the status enums from the enum table, and warns when a sample uses
   a value that is not listed.
10. Folds endpoints that share a method and path (six ways to
    `POST /v2/tokens`) into one operation with `x-tap-variants`. The
    generator emits one method for each.

Method and type names come from the operations table at the top of
`internal/cmd/buildspec/main.go`. If Tap adds a page that is not in the
table, it is still included with derived names and a warning is printed.

Some responses are documented as `{}` and have no sample anywhere in the
docs: the authorize, business, customer, invoice, and merchant lists, and
archive merchant. Those come out as `map[string]any`, and the builder warns
about them. Only a real response can type them.

`go test` decodes every documented response example and every collected
sample (88 bodies) into its generated type with unknown fields disallowed,
so a bad type in the spec fails the tests.

The docs site rate-limits. The builder backs off and honors `Retry-After`,
so a run after several back-to-back runs can take a few minutes.

## Regeneration

Rebuild the spec from the docs and regenerate the SDK:

```sh
scripts/update-api-spec.sh
```

If generated files were deleted and the spec itself is unchanged, force
regeneration:

```sh
scripts/update-api-spec.sh --force
```

For polling or CI, use check mode. It exits `0` when the spec is unchanged and
`10` when the docs changed.

```sh
scripts/update-api-spec.sh --check
```

To poll continuously, pass a watch interval in seconds:

```sh
scripts/update-api-spec.sh --watch 3600
```

You can also run the generators directly:

```sh
go run ./internal/cmd/buildspec
go run ./internal/cmd/gensdk
```

`client.go`, `request.go`, `errors.go`, `file_upload.go` (the multipart
upload), `webhook.go`, `flexnumber.go`, `sources.go`, and `testcards.go` are
written by hand. Everything else in the root package is generated.

## Verification

```sh
go test ./...
go vet ./...
go build ./...
```
