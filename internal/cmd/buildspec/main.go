// Command buildspec assembles ref/api-spec.json, a single OpenAPI 3.1
// document for the Tap Payments API.
//
// Tap does not publish one spec. Its docs (developers.tap.company, hosted on
// ReadMe) embed a small OpenAPI document in every API reference page, each
// with the resource path in servers[0] and mostly inline schemas. Roughly
// half of the responses have only a JSON example and no schema. This command:
//
//  1. discovers every reference page from the docs sidebar and llms.txt,
//  2. pulls the embedded OpenAPI document out of each page's markdown,
//  3. rebases each operation onto https://api.tap.company,
//  4. merges declared schemas with schemas inferred from examples, plus the
//     response examples on object overview pages ("The Charge object"),
//  5. hoists inline objects into named components and fixes types the docs
//     get wrong (decimal amounts typed as integers, int32 card numbers), and
//  6. folds operations that share a method and path (the many ways to create
//     a token) into one operation listing them under x-tap-variants.
//
// The operations table below names every known operation. Operations that
// appear in the docs but not in the table are still included with derived
// names, and a warning is printed so the table can be updated.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yazeed1s/tap-go/internal/naming"
)

const (
	docsBase  = "https://developers.tap.company"
	apiBase   = "https://api.tap.company"
	schemaRef = "#/components/schemas/"
)

// S is a JSON object: a schema, operation, or any other spec node.
type S = map[string]any

// opConfig names one documented Tap operation.
type opConfig struct {
	Slug     string // docs page slug, used as the operationId
	Tag      string // SDK service, e.g. "Charges"
	Name     string // Go method name
	Request  string // request body schema name
	Response string // 2xx response schema name
}

var tags = []struct{ Name, Description string }{
	{"Charges", "Charges collect a payment from a card or other payment source."},
	{"Authorize", "Authorizations hold an amount on a card for later capture or void."},
	{"Refunds", "Refunds return part or all of a captured charge."},
	{"Tokens", "Tokens represent card, wallet, or saved card details for one use."},
	{"Customers", "Customers store payer details for reuse across payments."},
	{"Cards", "Cards are payment cards saved to a customer."},
	{"Invoices", "Invoices request payment from a customer through a hosted page."},
	{"Intents", "Intents describe a payment session before it becomes a charge."},
	{"Businesses", "Businesses are the legal entities behind marketplace merchants."},
	{"Merchants", "Merchants are accounts that accept payments under a business."},
	{"Destinations", "Destinations split charge funds between merchants."},
	{"Files", "Files hold uploaded documents such as identity documents."},
	{"Payouts", "Payouts transfer settled funds to a merchant bank account."},
	{"Disputes", "Disputes are chargebacks raised against charges."},
	{"Leads", "Leads collect onboarding details for a prospective merchant."},
	{"Connect", "Connect onboards merchants through Tap hosted flows."},
}

var operations = []opConfig{
	{"create-a-charge", "Charges", "Create", "CreateChargeRequest", "Charge"},
	{"retrieve-a-charges", "Charges", "Retrieve", "", "Charge"},
	{"update-a-charge", "Charges", "Update", "UpdateChargeRequest", "Charge"},
	{"list-all-charges", "Charges", "List", "ListChargesRequest", "ChargeList"},
	{"download-charges", "Charges", "Download", "DownloadChargesRequest", ""},

	{"create-an-authorize", "Authorize", "Create", "CreateAuthorizeRequest", "Authorize"},
	{"retrieve-an-authorize", "Authorize", "Retrieve", "", "Authorize"},
	{"update-an-authorize", "Authorize", "Update", "UpdateAuthorizeRequest", "Authorize"},
	{"void-an-authorize", "Authorize", "Void", "", "Authorize"},
	{"list-all-authorize", "Authorize", "List", "ListAuthorizeRequest", "AuthorizeList"},
	{"download-authorize", "Authorize", "Download", "DownloadAuthorizeRequest", ""},

	{"create-a-refund", "Refunds", "Create", "CreateRefundRequest", "Refund"},
	{"retrieve-a-refund", "Refunds", "Retrieve", "", "Refund"},
	{"update-a-refund", "Refunds", "Update", "UpdateRefundRequest", "Refund"},
	{"list-all-refunds", "Refunds", "List", "ListRefundsRequest", "RefundList"},
	{"download-refunds", "Refunds", "Download", "DownloadRefundsRequest", ""},

	{"create-a-token", "Tokens", "CreateFromCard", "CreateCardTokenRequest", "Token"},
	{"create-a-token-from-saved-card", "Tokens", "CreateFromSavedCard", "CreateSavedCardTokenRequest", "Token"},
	{"create-a-token-encrypted-card", "Tokens", "CreateFromEncryptedCard", "CreateEncryptedCardTokenRequest", "Token"},
	{"create-applepay-tap-token", "Tokens", "CreateFromApplePay", "CreateApplePayTokenRequest", "Token"},
	{"create-samsungpay-token", "Tokens", "CreateFromSamsungPay", "CreateSamsungPayTokenRequest", "Token"},
	{"create-a-token-network-token", "Tokens", "CreateNetworkToken", "CreateNetworkTokenRequest", "Token"},
	{"retrieve-a-token", "Tokens", "Retrieve", "", "Token"},

	{"create-a-customer", "Customers", "Create", "CreateCustomerRequest", "Customer"},
	{"retrieve-a-customer", "Customers", "Retrieve", "", "Customer"},
	{"update-a-customer", "Customers", "Update", "UpdateCustomerRequest", "Customer"},
	{"customer-list", "Customers", "List", "ListCustomersRequest", "CustomerList"},

	{"list-all-cards", "Cards", "List", "", "CardList"},
	{"retrieve-a-card", "Cards", "Retrieve", "", "Card"},
	{"delete-a-card", "Cards", "Delete", "", "DeleteCardResponse"},
	{"verify-a-card", "Cards", "Verify", "VerifyCardRequest", "CardVerification"},

	{"create-an-invoice", "Invoices", "Create", "CreateInvoiceRequest", "Invoice"},
	{"retrieve-an-invoice", "Invoices", "Retrieve", "", "Invoice"},
	{"update-an-invoice", "Invoices", "Update", "UpdateInvoiceRequest", "Invoice"},
	{"cancel-an-invoice", "Invoices", "Cancel", "", "Invoice"},
	{"finalize-an-invoice", "Invoices", "Finalize", "", "Invoice"},
	{"remind-an-invoice", "Invoices", "Remind", "", "Invoice"},
	{"list-all-invoices", "Invoices", "List", "ListInvoicesRequest", "InvoiceList"},

	{"create-an-intent", "Intents", "Create", "CreateIntentRequest", "Intent"},
	{"retrieve-an-intent", "Intents", "Retrieve", "", "Intent"},
	{"cancel-an-intent", "Intents", "Cancel", "", "Intent"},
	{"list-all-intents", "Intents", "List", "ListIntentsRequest", "IntentList"},

	{"create-a-business", "Businesses", "Create", "CreateBusinessRequest", "Business"},
	{"retrieve-a-business", "Businesses", "Retrieve", "", "Business"},
	{"list-all-businesses", "Businesses", "List", "ListBusinessesRequest", "BusinessList"},

	{"create-a-merchant", "Merchants", "Create", "CreateMerchantRequest", "Merchant"},
	{"retrieve-a-merchant", "Merchants", "Retrieve", "", "Merchant"},
	{"archive-a-merchant", "Merchants", "Archive", "", "ArchiveMerchantResponse"},
	{"list-all-merchants", "Merchants", "List", "ListMerchantsRequest", "MerchantList"},

	{"retrieve-a-destination", "Destinations", "Retrieve", "", "Destination"},
	{"list-all-destinations", "Destinations", "List", "ListDestinationsRequest", "DestinationList"},

	{"create-a-file", "Files", "Create", "CreateFileRequest", "File"},
	{"retrieve-a-file", "Files", "Retrieve", "", "File"},

	{"list-payouts", "Payouts", "List", "ListPayoutsRequest", "PayoutList"},
	{"retrieve-a-payout", "Payouts", "Retrieve", "RetrievePayoutRequest", "PayoutList"},
	{"download-payouts", "Payouts", "Download", "DownloadPayoutsRequest", ""},

	{"download-disputes", "Disputes", "Download", "DownloadDisputesRequest", ""},

	{"create-a-lead-v3", "Leads", "Create", "CreateLeadRequest", "Lead"},
	{"create-a-lead-for-retailer", "Leads", "CreateForRetailer", "CreateRetailerLeadRequest", "Lead"},
	{"retrieve-a-lead", "Leads", "Retrieve", "", "Lead"},
	{"update-a-lead-for-retailer", "Leads", "UpdateForRetailer", "UpdateRetailerLeadRequest", "Lead"},

	{"create-a-connect-url", "Connect", "CreateURL", "CreateConnectURLRequest", "ConnectURL"},
	{"convert-lead-to-a-retailer", "Connect", "ConvertLead", "ConvertLeadRequest", "ConnectAccount"},
}

// resourceTags maps the first path segment after the API version to a tag.
// It is used to place operations missing from the table above.
var resourceTags = map[string]string{
	"authorize":   "Authorize",
	"business":    "Businesses",
	"card":        "Cards",
	"charges":     "Charges",
	"connect":     "Connect",
	"customers":   "Customers",
	"destination": "Destinations",
	"disputes":    "Disputes",
	"files":       "Files",
	"intent":      "Intents",
	"invoices":    "Invoices",
	"lead":        "Leads",
	"merchant":    "Merchants",
	"payouts":     "Payouts",
	"refunds":     "Refunds",
	"tokens":      "Tokens",
}

// objectSchemas maps the "object" field Tap puts on its resources to the
// schema describing that resource. Any nested object or example carrying
// one of these values is merged into the shared schema rather than given its
// own inline type.
var objectSchemas = map[string]string{
	"authorize":   "Authorize",
	"business":    "Business",
	"card":        "Card",
	"charge":      "Charge",
	"customer":    "Customer",
	"destination": "Destination",
	"file":        "File",
	"intent":      "Intent",
	"invoice":     "Invoice",
	"lead":        "Lead",
	"merchant":    "Merchant",
	"payout":      "Payout",
	"refund":      "Refund",
	"token":       "Token",
}

// skipPathFragments drops placeholder pages the docs publish without content.
var skipPathFragments = []string{"new-endpoint"}

// renames gives generated schema names a better name. Applied before dedupe.
// Merging into an existing schema is allowed: destination list items are
// destinations without the envelope fields.
var renames = map[string]string{
	"PayoutListPayout":           "Payout",
	"DestinationListDestination": "Destination",
}

// enumConfig turns string fields into a named enum. Values are the union of
// the values listed in Tap's docs and those seen in documented responses.
// Enum types are plain strings in Go, so values Tap adds later still decode.
type enumConfig struct {
	Name        string
	Description string
	Values      []string
	Fields      []string // "Schema.property"
}

var (
	chargeStatuses = []string{
		"INITIATED", "IN_PROGRESS", "ABANDONED", "CANCELLED", "FAILED",
		"DECLINED", "RESTRICTED", "CAPTURED", "VOID", "TIMEDOUT", "UNKNOWN",
	}
	enums = []enumConfig{
		{
			Name: "ChargeStatus",
			Description: "ChargeStatus is the status of a charge. CAPTURED means the " +
				"payment succeeded; ABANDONED, CANCELLED, FAILED, DECLINED, " +
				"RESTRICTED, VOID, TIMEDOUT, and UNKNOWN mean it did not.",
			Values: chargeStatuses,
			Fields: []string{"Charge.status", "ListChargesRequest.status"},
		},
		{
			Name: "AuthorizeStatus",
			Description: "AuthorizeStatus is the status of an authorization. " +
				"AUTHORIZED means the amount is held on the card.",
			Values: append(append([]string{}, chargeStatuses...), "AUTHORIZED"),
			Fields: []string{"Authorize.status", "ListAuthorizeRequest.status"},
		},
		{
			Name: "RefundStatus",
			Description: "RefundStatus is the status of a refund. With Tap's refund " +
				"v2 flow a refund is ACCEPTED, then REFUNDED or REJECTED.",
			Values: []string{"PENDING", "ACCEPTED", "REFUNDED", "REJECTED"},
			Fields: []string{"Refund.status"},
		},
		{
			Name:        "InvoiceStatus",
			Description: "InvoiceStatus is the status of an invoice.",
			Values:      []string{"SAVED", "CREATED", "CANCELLED", "EXPIRED", "PAID"},
			Fields:      []string{"Invoice.status"},
		},
		{
			Name: "InvoiceTrackStatus",
			Description: "InvoiceTrackStatus is the latest tracking event of an " +
				"invoice, such as delivery, views, and payment attempts.",
			Values: []string{
				"DRAFT", "SCHEDULED", "DISPATCHED", "DELIVERED", "NOT_DELIVERED",
				"VIEWED", "REMINDED", "CREATED", "UPDATED", "CANCELLED", "EXPIRED",
				"PAYMENT_ATTEMPT", "PAYMENT_FAILED", "PAYMENT_AUTHORIZED",
				"PAYMENT_CAPTURED", "PAYMENT_POSTED", "PAYMENT_REFUNDED",
				"PAYMENT_PARTIALLY_REFUNDED", "PAYMENT_ON_HOLD",
				"PAYMENT_SETTLED", "PAID", "REFUNDED", "INTENT_INITIATED",
				"PROCESSED",
			},
			Fields: []string{"InvoiceTrack.status", "ListInvoicesRequest.track"},
		},
		{
			Name:        "PayoutStatus",
			Description: "PayoutStatus is the status of a payout.",
			Values:      []string{"PENDING", "INITIATED", "FAILED", "PAID_OUT"},
			Fields:      []string{"Payout.status"},
		},
	}
)

// decimalKeys are money-like fields. The docs often type them as integers
// because the example amount happened to be whole, but Tap amounts are
// decimals (1.000 KWD), so decoding into an integer would fail.
var decimalKeys = map[string]bool{
	"amount": true, "itemamount": true, "total": true, "fee": true,
	"fees": true, "value": true, "rate": true, "price": true, "vat": true,
	"tax": true, "balance": true, "commission": true, "percentage": true,
	"total_amount": true, "net_amount": true, "unit_amount": true,
}

var (
	slugRe      = regexp.MustCompile(`"slug":"([a-z0-9][a-z0-9_-]*)"`)
	llmsRe      = regexp.MustCompile(`/(reference|docs)/([a-z0-9][a-z0-9_-]*)\.md`)
	specBlockRe = regexp.MustCompile("(?s)# OpenAPI definition\\s*```json\\n(.*?)\\n```")
	pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)
	apiKeyRe    = regexp.MustCompile(`\b(sk|pk)_(test|live)_[A-Za-z0-9*]{4,}`)
)

var methodOrder = []string{"get", "post", "put", "patch", "delete"}

func main() {
	out := flag.String("out", "ref/api-spec.json", "output spec path")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	refSlugs, guideSlugs, err := discoverSlugs(client)
	if err != nil {
		log.Fatal(err)
	}
	pages, err := fetchPages(client, "reference", refSlugs)
	if err != nil {
		log.Fatal(err)
	}
	guides, err := fetchPages(client, "docs", guideSlugs)
	if err != nil {
		log.Fatal(err)
	}

	b := &builder{schemas: map[string]S{}, samples: map[string][]any{}}
	spec, err := b.build(refSlugs, pages, guides)
	if err != nil {
		log.Fatal(err)
	}
	for _, w := range b.warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	redactKeys(spec)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(
		os.Stderr,
		"wrote %s: %d reference pages, %d guides, %d paths, %d schemas, %d samples\n",
		*out, len(pages), len(guides), len(spec["paths"].(S)), len(b.schemas), b.sampleCount(),
	)
}

// redactKeys replaces API keys in every string of the spec with a
// placeholder. Tap's docs paste real-looking keys into examples and
// descriptions; copying them would trip secret scanners (GitHub push
// protection) and spread whatever keys they are.
func redactKeys(v any) {
	switch x := v.(type) {
	case S:
		for k, e := range x {
			if str, ok := e.(string); ok {
				x[k] = apiKeyRe.ReplaceAllString(str, "${1}_${2}_xxx")
			} else {
				redactKeys(e)
			}
		}
	case []any:
		for i, e := range x {
			if str, ok := e.(string); ok {
				x[i] = apiKeyRe.ReplaceAllString(str, "${1}_${2}_xxx")
			} else {
				redactKeys(e)
			}
		}
	}
}

// discoverSlugs lists every API reference page and guide page. The sidebars
// embedded in the reference and guide HTML are the complete lists; llms.txt
// is a partial index unioned in as a fallback in case the markup changes.
func discoverSlugs(client *http.Client) (reference, guides []string, err error) {
	seen := map[string]map[string]bool{"reference": {}, "docs": {}}
	for section := range seen {
		html, err := get(client, docsBase+"/"+section)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range slugRe.FindAllStringSubmatch(html, -1) {
			seen[section][m[1]] = true
		}
	}
	llms, err := get(client, docsBase+"/llms.txt")
	if err != nil {
		return nil, nil, err
	}
	for _, m := range llmsRe.FindAllStringSubmatch(llms, -1) {
		seen[m[1]][m[2]] = true
	}
	if len(seen["reference"]) == 0 {
		return nil, nil, fmt.Errorf("no reference pages found at %s", docsBase)
	}
	return sortedKeys(seen["reference"]), sortedKeys(seen["docs"]), nil
}

// fetchPages downloads the markdown of each page in a docs section
// ("reference" or "docs"). Missing pages are skipped.
func fetchPages(client *http.Client, section string, slugs []string) (map[string]string, error) {
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		pages    = map[string]string{}
		sem      = make(chan struct{}, 4)
	)
	for _, slug := range slugs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			body, err := get(client, docsBase+"/"+section+"/"+slug+".md")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil && firstErr == nil:
				firstErr = fmt.Errorf("fetch %s: %w", slug, err)
			case err == nil && body != "":
				pages[slug] = body
			}
		}()
	}
	wg.Wait()
	return pages, firstErr
}

// get fetches url, retrying network errors, 429s, and 5xx responses with
// exponential backoff (honoring Retry-After). A 404 returns an empty body
// and no error.
func get(client *http.Client, rawURL string) (string, error) {
	const attempts = 6
	var lastErr error
	delay := 2 * time.Second
	for attempt := range attempts {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}
		resp, err := client.Get(rawURL)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode == http.StatusNotFound:
			return "", nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("%s: status %d", rawURL, resp.StatusCode)
			if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
				delay = time.Duration(secs) * time.Second
			}
		case resp.StatusCode != http.StatusOK:
			return "", fmt.Errorf("%s: status %d", rawURL, resp.StatusCode)
		default:
			return string(body), nil
		}
	}
	return "", lastErr
}

type builder struct {
	schemas  map[string]S
	warnings []string
	merging  map[string]bool
	// samples are real response bodies found in the docs prose, keyed by
	// the schema they were merged into. They are kept in the spec as schema
	// examples so the SDK tests can decode them.
	samples map[string][]any
}

func (b *builder) sampleCount() int {
	n := 0
	for _, s := range b.samples {
		n += len(s)
	}
	return n
}

func (b *builder) warnf(format string, args ...any) {
	b.warnings = append(b.warnings, fmt.Sprintf(format, args...))
}

// docOp is one operation as published on one docs page.
type docOp struct {
	cfg    opConfig
	method string
	path   string
	op     S
	title  string
}

func (b *builder) build(slugs []string, pages, guides map[string]string) (S, error) {
	configs := map[string]opConfig{}
	order := map[string]int{}
	for i, cfg := range operations {
		configs[cfg.Slug] = cfg
		order[cfg.Slug] = i
	}

	var ops []docOp
	seen := map[string]bool{}
	for _, slug := range slugs {
		page, ok := pages[slug]
		if !ok {
			continue
		}
		m := specBlockRe.FindStringSubmatch(page)
		if m == nil {
			continue
		}
		var pageSpec S
		if err := decode(m[1], &pageSpec); err != nil {
			return nil, fmt.Errorf("%s: decode embedded spec: %w", slug, err)
		}
		pageOps, err := b.pageOps(slug, pageSpec, configs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		for _, op := range pageOps {
			op.title = pageTitle(page)
			seen[slug] = true
			ops = append(ops, op)
		}
	}
	for _, cfg := range operations {
		if !seen[cfg.Slug] {
			b.warnf("operation %s is in the table but no longer documented", cfg.Slug)
		}
	}

	sort.SliceStable(ops, func(i, j int) bool {
		oi, iok := order[ops[i].cfg.Slug]
		oj, jok := order[ops[j].cfg.Slug]
		if iok != jok {
			return iok
		}
		if !iok {
			return ops[i].cfg.Slug < ops[j].cfg.Slug
		}
		return oi < oj
	})

	type key struct{ path, method string }
	grouped := map[key][]S{}
	var keys []key
	for _, item := range ops {
		k := key{item.path, item.method}
		if _, ok := grouped[k]; !ok {
			keys = append(keys, k)
		}
		grouped[k] = append(grouped[k], b.operation(item))
	}

	paths := S{}
	for _, k := range keys {
		item, _ := paths[k.path].(S)
		if item == nil {
			item = S{}
			paths[k.path] = item
		}
		item[k.method] = b.foldVariants(k.method+" "+k.path, grouped[k])
	}

	for _, set := range []map[string]string{pages, guides} {
		for _, slug := range sortedKeys(set) {
			b.harvestSamples(set[slug])
		}
	}

	b.finalizeSchemas()
	protected := map[string]bool{"ErrorResponse": true}
	for _, cfg := range operations {
		protected[cfg.Request], protected[cfg.Response] = true, true
	}
	for _, name := range objectSchemas {
		protected[name] = true
	}
	for from, to := range renames {
		protected[to] = true
		b.rename(paths, from, to)
	}
	b.dedupe(paths, protected)
	b.applyEnums()
	for _, name := range sortedKeys(b.samples) {
		if s, ok := b.schemas[name]; ok {
			s["examples"] = b.samples[name]
		}
	}
	sort.Strings(b.warnings)

	tagList := make([]any, 0, len(tags))
	for _, t := range tags {
		tagList = append(tagList, S{"name": t.Name, "description": t.Description})
	}

	return S{
		"openapi": "3.1.0",
		"info": S{
			"title":   "Tap Payments API",
			"version": "2.0",
			"description": "Generated by internal/cmd/buildspec from the API reference at " +
				docsBase + "/reference. Tap does not publish a combined OpenAPI " +
				"document; this file merges the per-page definitions, infers " +
				"schemas from documented examples, and names shared objects.",
		},
		"servers":  []any{S{"url": apiBase}},
		"security": []any{S{"bearerAuth": []any{}}},
		"tags":     tagList,
		"paths":    paths,
		"components": S{
			"securitySchemes": S{
				"bearerAuth": S{
					"type":        "http",
					"scheme":      "bearer",
					"description": "Tap secret key, e.g. sk_test_...",
				},
			},
			"schemas": b.schemas,
		},
	}, nil
}

func (b *builder) pageOps(slug string, pageSpec S, configs map[string]opConfig) ([]docOp, error) {
	base := ""
	if servers, _ := pageSpec["servers"].([]any); len(servers) > 0 {
		raw, _ := servers[0].(S)["url"].(string)
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse server url %q: %w", raw, err)
		}
		base = u.Path
	}

	var out []docOp
	paths, _ := pageSpec["paths"].(S)
	for _, rawPath := range sortedKeys(paths) {
		path := normalizePath(base + "/" + strings.TrimSpace(rawPath))
		if skipPath(path) {
			continue
		}
		item, _ := paths[rawPath].(S)
		for _, method := range methodOrder {
			op, ok := item[method].(S)
			if !ok {
				continue
			}
			cfg, ok := configs[slug]
			if !ok {
				cfg = deriveConfig(slug, path)
				b.warnf(
					"operation %s (%s %s) is not in the table; using %s.%s",
					slug, strings.ToUpper(method), path, cfg.Tag, cfg.Name,
				)
			}
			out = append(out, docOp{cfg: cfg, method: method, path: path, op: op})
		}
	}
	return out, nil
}

func deriveConfig(slug, path string) opConfig {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	resource := ""
	if len(parts) > 1 {
		resource = parts[1]
	}
	tag, ok := resourceTags[resource]
	if !ok {
		tag = naming.Export(resource)
	}
	name := naming.Export(slug)
	return opConfig{
		Slug:     slug,
		Tag:      tag,
		Name:     name,
		Request:  name + "Request",
		Response: name + "Response",
	}
}

// operation converts one docs operation into a spec operation, defining its
// request and response schemas as components along the way.
func (b *builder) operation(item docOp) S {
	cfg, src := item.cfg, item.op
	op := S{
		"operationId":  cfg.Slug,
		"x-go-name":    cfg.Name,
		"tags":         []any{cfg.Tag},
		"externalDocs": S{"url": docsBase + "/reference/" + cfg.Slug},
	}
	summary, _ := src["summary"].(string)
	if summary == "" {
		summary = item.title
	}
	if summary != "" {
		op["summary"] = strings.TrimSpace(summary)
	}
	if desc, _ := src["description"].(string); strings.TrimSpace(desc) != "" {
		op["description"] = strings.TrimSpace(desc)
	}
	if dep, _ := src["deprecated"].(bool); dep {
		op["deprecated"] = true
	}
	if params := b.parameters(item.path, src); len(params) > 0 {
		op["parameters"] = params
	}
	if body := b.requestBody(cfg, src); body != nil {
		op["requestBody"] = body
	}
	op["responses"] = b.responses(cfg, src)
	return op
}

func (b *builder) parameters(path string, src S) []any {
	var out []any
	have := map[string]bool{}
	raw, _ := src["parameters"].([]any)
	for _, p := range raw {
		param, _ := p.(S)
		name, _ := param["name"].(string)
		in, _ := param["in"].(string)
		if name == "" || in == "" {
			continue
		}
		clean := S{"name": name, "in": in}
		if desc, _ := param["description"].(string); desc != "" {
			clean["description"] = desc
		}
		schema, _ := param["schema"].(S)
		if schema == nil {
			schema = S{"type": "string"}
		}
		clean["schema"] = fixSchema(clone(schema), name, "")
		if in == "path" {
			clean["required"] = true
			clean["schema"] = S{"type": "string"}
			have[name] = true
		} else if req, _ := param["required"].(bool); req {
			clean["required"] = true
		}
		out = append(out, clean)
	}
	for _, m := range pathParamRe.FindAllStringSubmatch(path, -1) {
		if !have[m[1]] {
			out = append(out, S{
				"name": m[1], "in": "path", "required": true,
				"schema": S{"type": "string"},
			})
		}
	}
	return out
}

func (b *builder) requestBody(cfg opConfig, src S) S {
	body, _ := src["requestBody"].(S)
	content, _ := body["content"].(S)
	if len(content) == 0 {
		if cfg.Request != "" {
			b.warnf("%s: table names request %s but the docs have no body", cfg.Slug, cfg.Request)
		}
		return nil
	}
	name := cfg.Request
	if name == "" {
		name = cfg.Name + naming.Singular(cfg.Tag) + "Request"
		b.warnf("%s: has a request body but no request name; using %s", cfg.Slug, name)
	}
	outContent := S{}
	for _, ct := range sortedKeys(content) {
		media, _ := content[ct].(S)
		tree := S{}
		if declared, ok := media["schema"].(S); ok {
			tree = clone(declared)
		}
		// Request examples are only a fallback: some pages show a captured
		// request ({method, url, headers, body}) rather than the body itself.
		if len(propsOf(tree)) == 0 {
			for _, ex := range mediaExamples(media) {
				tree = merge(tree, infer(ex, ""))
			}
		}
		b.define(name, fixSchema(tree, "", ""))
		mt := S{"schema": S{"$ref": schemaRef + name}}
		if ct == "application/json" || strings.HasPrefix(ct, "multipart/") {
			outContent[ct] = mt
		} else {
			outContent["application/json"] = mt
		}
	}
	return S{"required": true, "content": outContent}
}

func (b *builder) responses(cfg opConfig, src S) S {
	out := S{}
	raw, _ := src["responses"].(S)
	hasSuccess := false
	for _, code := range sortedKeys(raw) {
		resp, _ := raw[code].(S)
		content, _ := resp["content"].(S)
		success := strings.HasPrefix(code, "2")
		hasSuccess = hasSuccess || success

		tree := S{}
		var examples []any
		isJSON := false
		textExample := ""
		for _, ct := range sortedKeys(content) {
			media, _ := content[ct].(S)
			if declared, ok := media["schema"].(S); ok && len(propsOf(declared)) > 0 {
				tree = merge(tree, clone(declared))
				isJSON = true
			}
			if strings.Contains(ct, "json") {
				isJSON = true
			}
			for _, ex := range mediaExamples(media) {
				switch ex.(type) {
				case S, []any:
					examples = append(examples, ex)
					isJSON = true
				case string:
					textExample = ex.(string)
				}
			}
		}
		for _, ex := range examples {
			tree = merge(tree, infer(ex, ""))
		}

		out[code] = b.response(cfg, code, success, isJSON, tree, examples, textExample)
	}
	if !hasSuccess {
		out["200"] = b.response(cfg, "200", true, true, S{}, nil, "")
	}
	return out
}

func (b *builder) response(cfg opConfig, code string, success, isJSON bool, tree S, examples []any, text string) S {
	resp := S{"description": responseDescription(code)}
	if success && cfg.Response == "" {
		if !isJSON || len(propsOf(tree)) == 0 {
			media := S{"schema": S{"type": "string"}}
			if text != "" {
				media["example"] = text
			}
			resp["content"] = S{"text/plain": media}
			return resp
		}
		b.warnf("%s: returns JSON but the table names no response schema", cfg.Slug)
		cfg.Response = cfg.Name + "Response"
	}
	if !isJSON && !success {
		return resp
	}
	name := cfg.Response
	if !success {
		name = "ErrorResponse"
	} else if known := objectSchemas[objectKind(tree)]; known != "" && known != name &&
		propsOf(tree)["has_more"] == nil { // list bodies sometimes carry the item kind
		b.warnf("%s: example is a %s but the table names %s", cfg.Slug, known, name)
	}
	b.define(name, fixSchema(tree, "", ""))
	media := S{"schema": S{"$ref": schemaRef + name}}
	if len(examples) > 0 {
		exs := S{}
		for i, ex := range examples {
			exs[fmt.Sprintf("example%d", i+1)] = S{"value": ex}
		}
		media["examples"] = exs
	}
	resp["content"] = S{"application/json": media}
	return resp
}

func responseDescription(code string) string {
	var n int
	fmt.Sscanf(code, "%d", &n)
	if text := http.StatusText(n); text != "" {
		return text
	}
	return code
}

// foldVariants merges operations that share a method and path. OpenAPI allows
// only one operation per method and path, but Tap documents several distinct
// calls on some endpoints (six ways to POST /v2/tokens). The first becomes
// the operation; all of them are listed under x-tap-variants with their own
// request bodies so the SDK generator can emit one method for each.
func (b *builder) foldVariants(where string, ops []S) S {
	if len(ops) == 1 {
		return ops[0]
	}
	primary := clone(ops[0])
	var variants, refs []any
	resp := ops[0]["responses"]
	for _, op := range ops {
		v := S{}
		for _, k := range []string{"operationId", "x-go-name", "summary", "description", "externalDocs", "requestBody", "deprecated"} {
			if val, ok := op[k]; ok {
				v[k] = val
			}
		}
		variants = append(variants, v)
		if ref := requestRef(op); ref != "" {
			refs = append(refs, S{"$ref": ref})
		}
		if r1, r2 := successRef(resp), successRef(op["responses"]); r1 != r2 {
			b.warnf("%s: variants return different schemas (%s, %s)", where, r1, r2)
		}
	}
	primary["x-tap-variants"] = variants
	if len(refs) > 0 {
		primary["requestBody"] = S{
			"required": true,
			"content":  S{"application/json": S{"schema": S{"oneOf": refs}}},
		}
	}
	return primary
}

func requestRef(op S) string {
	body, _ := op["requestBody"].(S)
	content, _ := body["content"].(S)
	for _, ct := range sortedKeys(content) {
		media, _ := content[ct].(S)
		schema, _ := media["schema"].(S)
		if ref, _ := schema["$ref"].(string); ref != "" {
			return ref
		}
	}
	return ""
}

func successRef(responses any) string {
	rs, _ := responses.(S)
	for _, code := range sortedKeys(rs) {
		if !strings.HasPrefix(code, "2") {
			continue
		}
		resp, _ := rs[code].(S)
		content, _ := resp["content"].(S)
		for _, ct := range sortedKeys(content) {
			media, _ := content[ct].(S)
			schema, _ := media["schema"].(S)
			ref, _ := schema["$ref"].(string)
			return ref
		}
	}
	return ""
}

// harvestSamples merges real response bodies shown in docs prose into the
// spec. Guides and object overview pages ("The Customer object", the webhook
// guide) contain fenced JSON blocks; a block is used when it is
//
//   - a list: it has has_more and an array named in listSchemas, or
//   - a resource: its "object" field names a known resource.
//
// Captured webhook requests ({"postData": {"text": "..."}}) are unwrapped
// first. Anything else (request bodies, curl commands) is ignored.
func (b *builder) harvestSamples(page string) {
	for _, block := range fencedBlocks(page) {
		var v any
		if decode(block, &v) != nil {
			continue
		}
		obj, _ := v.(S)
		if _, ok := obj["openapi"]; ok {
			continue
		}
		if post, ok := obj["postData"].(S); ok {
			text, _ := post["text"].(string)
			var inner any
			if decode(text, &inner) != nil {
				continue
			}
			obj, _ = inner.(S)
		}
		if name := sampleSchema(obj); name != "" {
			b.define(name, fixSchema(infer(obj, ""), "", ""))
			b.samples[name] = append(b.samples[name], obj)
		}
	}
}

// listSchemas maps the array field of a Tap list response to its schema.
var listSchemas = map[string]string{
	"businesses":   "BusinessList",
	"charges":      "ChargeList",
	"customers":    "CustomerList",
	"destinations": "DestinationList",
	"intents":      "IntentList",
	"invoices":     "InvoiceList",
	"merchants":    "MerchantList",
	"payouts":      "PayoutList",
	"refunds":      "RefundList",
}

// sampleSchema returns the schema a docs sample belongs to, or "".
func sampleSchema(obj S) string {
	if obj == nil {
		return ""
	}
	// Lists are checked first: some carry the item kind in "object"
	// ({"object": "destination", "destinations": [...]}).
	if _, ok := obj["has_more"]; ok {
		for _, key := range sortedKeys(listSchemas) {
			if _, ok := obj[key].([]any); ok {
				return listSchemas[key]
			}
		}
		return ""
	}
	kind, _ := obj["object"].(string)
	return objectSchemas[kind]
}

// fencedBlocks returns the contents of every ``` fenced block in page.
func fencedBlocks(page string) []string {
	var blocks []string
	var cur []string
	in := false
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if in {
				blocks = append(blocks, strings.Join(cur, "\n"))
			}
			in, cur = !in, cur[:0]
			continue
		}
		if in {
			cur = append(cur, line)
		}
	}
	return blocks
}

// define hoists nested objects out of s and merges the result into the named
// component.
func (b *builder) define(name string, s S) {
	s = b.hoist(name, s)
	if old, ok := b.schemas[name]; ok {
		s = mergeWith(b, old, s)
	}
	b.schemas[name] = s
}

// mergeRefs folds the schema behind ref b into the schema behind ref a, for
// when two examples put differently named objects in the same field.
func (b *builder) mergeRefs(ra, rb string) {
	na, nb := strings.TrimPrefix(ra, schemaRef), strings.TrimPrefix(rb, schemaRef)
	key := na + "|" + nb
	sa, okA := b.schemas[na]
	sb, okB := b.schemas[nb]
	if !okA || !okB || b.merging[key] {
		return
	}
	if b.merging == nil {
		b.merging = map[string]bool{}
	}
	b.merging[key] = true
	defer delete(b.merging, key)
	b.schemas[na] = mergeWith(b, sa, sb)
}

func (b *builder) hoist(name string, s S) S {
	out := clone(s)
	delete(out, "x-tap-object")
	if props := propsOf(s); props != nil {
		np := S{}
		base := strings.TrimSuffix(name, "Request")
		for _, key := range sortedKeys(props) {
			prop, _ := props[key].(S)
			np[key] = b.hoistValue(base+naming.Export(key), key, prop)
		}
		out["properties"] = np
	}
	return out
}

func (b *builder) hoistValue(child, key string, s S) S {
	switch {
	case s["$ref"] != nil:
		return s
	case typeOf(s) == "object" && len(propsOf(s)) > 0:
		// Use the shared resource schema only when the field is named for
		// it (customer, charges[], or a list's data[]). Polymorphic fields such as a charge's
		// source hold a token or a source object and keep their own type.
		kind := objectKind(s)
		if known := objectSchemas[kind]; known != "" && (naming.Singular(key) == kind || key == "data") {
			child = known
		}
		b.define(child, s)
		ref := S{"$ref": schemaRef + child}
		if desc, ok := s["description"]; ok {
			ref["description"] = desc
		}
		return ref
	case typeOf(s) == "array":
		items, _ := s["items"].(S)
		if items == nil {
			return s
		}
		out := clone(s)
		singular := strings.TrimSuffix(child, naming.Export(key)) +
			naming.Export(naming.Singular(key))
		out["items"] = b.hoistValue(singular, key, items)
		return out
	}
	return s
}

// objectKind returns the Tap "object" discriminator of a schema, either seen
// in an example or declared on its "object" property.
func objectKind(s S) string {
	if kind, _ := s["x-tap-object"].(string); kind != "" {
		return kind
	}
	prop, _ := propsOf(s)["object"].(S)
	for _, k := range []string{"example", "default", "const"} {
		if v, _ := prop[k].(string); v != "" {
			return v
		}
	}
	return ""
}

func (b *builder) finalizeSchemas() {
	for name, s := range b.schemas {
		if typeOf(s) == "" && s["$ref"] == nil {
			s["type"] = "object"
		}
		if typeOf(s) == "object" && len(propsOf(s)) == 0 && s["additionalProperties"] == nil {
			s["additionalProperties"] = true
			b.warnf("schema %s has no documented fields", name)
		}
	}
}

// rename moves schema from to the name to, merging if to already exists.
func (b *builder) rename(paths S, from, to string) {
	s, ok := b.schemas[from]
	if !ok {
		b.warnf("rename: schema %s not found", from)
		return
	}
	delete(b.schemas, from)
	if old, ok := b.schemas[to]; ok {
		s = mergeWith(b, old, s)
	}
	b.schemas[to] = s
	for _, other := range b.schemas {
		rewriteRefs(other, map[string]string{from: to})
	}
	rewriteRefs(paths, map[string]string{from: to})
}

// applyEnums defines each enum as a component and points its fields at it.
// A field that holds an array of strings gets the enum as its item type.
func (b *builder) applyEnums() {
	for _, e := range enums {
		values := make([]any, len(e.Values))
		for i, v := range e.Values {
			values[i] = v
		}
		b.schemas[e.Name] = S{"type": "string", "description": e.Description, "enum": values}
		ref := S{"$ref": schemaRef + e.Name}
		for _, field := range e.Fields {
			schemaName, prop, _ := strings.Cut(field, ".")
			props := propsOf(b.schemas[schemaName])
			p, ok := props[prop].(S)
			if !ok {
				b.warnf("enum %s: field %s not found", e.Name, field)
				continue
			}
			target := p
			if typeOf(p) == "array" {
				target, _ = p["items"].(S)
			}
			if typeOf(target) != "string" {
				b.warnf("enum %s: field %s is %q, not a string", e.Name, field, typeOf(target))
				continue
			}
			out := clone(ref)
			if desc, ok := p["description"]; ok {
				out["description"] = desc
			}
			if typeOf(p) == "array" {
				arr := clone(p)
				arr["items"] = ref
				out = arr
			}
			props[prop] = out
			b.checkEnumSamples(e, schemaName, prop)
		}
	}
}

// checkEnumSamples warns when a documented sample uses a value missing from
// the enum, so the enum table can be updated.
func (b *builder) checkEnumSamples(e enumConfig, schemaName, prop string) {
	known := map[string]bool{}
	for _, v := range e.Values {
		known[v] = true
	}
	for _, sample := range b.samples[schemaName] {
		obj, _ := sample.(S)
		if v, ok := obj[prop].(string); ok && !known[v] {
			b.warnf("enum %s: sample value %q of %s.%s is not listed", e.Name, v, schemaName, prop)
		}
	}
}

// genericNames renames dedupe targets that would be too vague as Go types.
var genericNames = map[string]string{
	"Response": "ResponseStatus",
}

// dedupe collapses hoisted schemas that have the same shape and the same
// trailing name word, e.g. CreateChargeCustomerPhone, InvoiceCustomerPhone,
// and every other {country_code, number} "...Phone" become one Phone. The
// group is named by the longest word suffix its members share. Schemas named
// in the operations table or by objectSchemas are never renamed. Merging can
// make parents identical, so it repeats until nothing changes.
func (b *builder) dedupe(paths S, protected map[string]bool) {
	for {
		groups := map[string][]string{}
		for _, name := range sortedKeys(b.schemas) {
			if protected[name] {
				continue
			}
			words := naming.Words(name)
			key := shapeKey(b.schemas[name]) + "|" + words[len(words)-1]
			groups[key] = append(groups[key], name)
		}

		// Larger groups pick names first, so the most common shape gets the
		// short name (Phone) and rarer variants keep their long names.
		keys := sortedKeys(groups)
		sort.SliceStable(keys, func(i, j int) bool {
			return len(groups[keys[i]]) > len(groups[keys[j]])
		})
		renames := map[string]string{}
		claimed := map[string]bool{}
		for _, key := range keys {
			names := groups[key]
			if len(names) < 2 {
				continue
			}
			target := commonSuffix(names)
			if repl, ok := genericNames[target]; ok {
				target = repl
			}
			member := false
			for _, n := range names {
				member = member || n == target
			}
			if claimed[target] || (!member && b.schemas[target] != nil) {
				continue
			}
			claimed[target] = true
			keep := b.schemas[names[0]]
			for _, n := range names {
				renames[n] = target
				delete(b.schemas, n)
			}
			b.schemas[target] = keep
		}
		if len(renames) == 0 {
			return
		}
		for _, s := range b.schemas {
			rewriteRefs(s, renames)
		}
		rewriteRefs(paths, renames)
	}
}

func commonSuffix(names []string) string {
	suffix := naming.Words(names[0])
	for _, n := range names[1:] {
		words := naming.Words(n)
		i := 0
		for i < len(suffix) && i < len(words) &&
			suffix[len(suffix)-1-i] == words[len(words)-1-i] {
			i++
		}
		suffix = suffix[len(suffix)-i:]
	}
	return naming.Export(strings.Join(suffix, "_"))
}

// shapeKey is a canonical encoding of s without descriptions and examples.
func shapeKey(s S) string {
	var strip func(v any) any
	strip = func(v any) any {
		switch x := v.(type) {
		case S:
			out := S{}
			for k, e := range x {
				if k != "description" && k != "example" {
					out[k] = strip(e)
				}
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, e := range x {
				out[i] = strip(e)
			}
			return out
		}
		return v
	}
	data, _ := json.Marshal(strip(s))
	return string(data)
}

func rewriteRefs(v any, renames map[string]string) {
	switch x := v.(type) {
	case S:
		if ref, ok := x["$ref"].(string); ok {
			if to, ok := renames[strings.TrimPrefix(ref, schemaRef)]; ok {
				x["$ref"] = schemaRef + to
			}
		}
		for _, e := range x {
			rewriteRefs(e, renames)
		}
	case []any:
		for _, e := range x {
			rewriteRefs(e, renames)
		}
	}
}

// infer derives a schema from an example value.
func infer(v any, key string) S {
	switch x := v.(type) {
	case nil:
		return S{}
	case bool:
		return S{"type": "boolean"}
	case string:
		return S{"type": "string"}
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") || decimalKeys[strings.ToLower(key)] {
			return S{"type": "number"}
		}
		return S{"type": "integer"}
	case []any:
		items := S{}
		for _, e := range x {
			items = merge(items, infer(e, key))
		}
		return S{"type": "array", "items": items}
	case S:
		props := S{}
		for k, e := range x {
			props[k] = infer(e, k)
		}
		out := S{"type": "object", "properties": props}
		if kind, _ := x["object"].(string); kind != "" {
			out["x-tap-object"] = kind
		}
		return out
	}
	return S{}
}

// fixSchema corrects the types the docs get wrong and drops ReadMe noise.
// key is the property name of s and parent the property name holding it.
func fixSchema(s S, key, parent string) S {
	if s == nil {
		return S{}
	}
	lower := strings.ToLower(key)
	if lower == "metadata" {
		out := S{"type": "object", "additionalProperties": S{"type": "string"}}
		if desc, ok := s["description"]; ok {
			out["description"] = desc
		}
		return out
	}
	if def, ok := s["default"]; ok {
		if _, has := s["example"]; !has {
			s["example"] = def
		}
		delete(s, "default")
	}
	if (lower == "limit" || lower == "page") && typeOf(s) == "string" {
		// Paging fields are numbers; the docs type some of them as strings.
		s["type"] = "integer"
		delete(s, "format")
		if ex, ok := s["example"].(string); ok {
			s["example"] = json.Number(ex)
			if _, err := s["example"].(json.Number).Int64(); err != nil {
				delete(s, "example")
			}
		}
	}
	switch typeOf(s) {
	case "integer", "number":
		delete(s, "format")
		switch {
		case lower == "cvc" || lower == "cvv",
			strings.EqualFold(parent, "phone") && (lower == "number" || lower == "country_code"):
			s["type"] = "string"
			if ex, ok := s["example"]; ok {
				s["example"] = fmt.Sprint(ex)
			}
		case decimalKeys[lower]:
			s["type"] = "number"
		}
	case "array":
		if _, ok := s["items"].(S); !ok {
			s["items"] = S{}
		}
	}
	if props := propsOf(s); props != nil {
		for k, v := range props {
			p, _ := v.(S)
			props[k] = fixSchema(p, k, key)
		}
	}
	if items, ok := s["items"].(S); ok {
		s["items"] = fixSchema(items, key, parent)
	}
	for _, k := range []string{"oneOf", "anyOf", "allOf"} {
		list, _ := s[k].([]any)
		for i, v := range list {
			p, _ := v.(S)
			list[i] = fixSchema(p, key, parent)
		}
	}
	return s
}

// merge unions two schemas that contain no component refs.
func merge(a, b S) S { return mergeWith(nil, a, b) }

// mergeWith unions two schemas. Properties are merged recursively, numeric
// types widen to number, and conflicting types become untyped (any). Refs
// only appear after hoisting, so bl is needed to reconcile two different
// refs; refs always point at object schemas.
func mergeWith(bl *builder, a, b S) S {
	if len(a) == 0 {
		return clone(b)
	}
	if len(b) == 0 {
		return clone(a)
	}
	if a[mixedTypes] != nil || b[mixedTypes] != nil {
		return untyped(a, b)
	}
	ra, _ := a["$ref"].(string)
	rb, _ := b["$ref"].(string)
	if ra != "" || rb != "" {
		ref, other := a, b
		if ra == "" {
			ref, other = b, a
		}
		switch t := typeOf(other); {
		case t != "" && t != "object":
			return untyped(a, b)
		case ra != "" && rb != "" && ra != rb && bl != nil:
			bl.mergeRefs(ra, rb)
		}
		out := clone(ref)
		if desc, ok := a["description"]; ok {
			out["description"] = desc
		}
		return out
	}

	out := clone(a)
	ta, tb := typeOf(a), typeOf(b)
	switch {
	case ta == "" && tb != "":
		out["type"] = b["type"]
	case tb == "" || ta == tb:
	case isNumeric(ta) && isNumeric(tb):
		out["type"] = "number"
	default:
		return untyped(a, b)
	}
	for k, v := range b {
		if _, ok := out[k]; !ok && k != "properties" && k != "items" {
			out[k] = v
		}
	}
	if pa, pb := propsOf(a), propsOf(b); pa != nil || pb != nil {
		props := S{}
		for k, v := range pa {
			props[k] = v
		}
		for k, v := range pb {
			vb, _ := v.(S)
			if old, ok := props[k].(S); ok {
				props[k] = mergeWith(bl, old, vb)
			} else {
				props[k] = vb
			}
		}
		out["properties"] = props
	}
	ia, _ := a["items"].(S)
	ib, _ := b["items"].(S)
	if ia != nil || ib != nil {
		out["items"] = mergeWith(bl, ia, ib)
	}
	return out
}

// mixedTypes lists the JSON types seen for a field whose examples disagree,
// e.g. ["integer", "string"] for a timestamp sent both ways. It is sticky:
// an empty schema means "nothing known yet" (a null example) and is filled
// by the next example, but a marked field stays untyped. The SDK generator
// uses the list to pick a type (FlexNumber or any).
const mixedTypes = "x-tap-mixed-types"

// untyped merges two disagreeing schemas into an untyped one that keeps the
// description of a and records the types both sides were seen with.
func untyped(a, b S) S {
	seen := map[string]bool{}
	for _, s := range []S{a, b} {
		for _, t := range seenTypes(s) {
			seen[t] = true
		}
	}
	types := make([]any, 0, len(seen))
	for _, t := range sortedKeys(seen) {
		types = append(types, t)
	}
	out := S{mixedTypes: types}
	if desc, ok := a["description"]; ok {
		out["description"] = desc
	}
	return out
}

// seenTypes returns the JSON types a schema stands for. Refs always point at
// object schemas.
func seenTypes(s S) []string {
	if list, ok := s[mixedTypes].([]any); ok {
		out := make([]string, 0, len(list))
		for _, t := range list {
			out = append(out, fmt.Sprint(t))
		}
		return out
	}
	if s["$ref"] != nil {
		return []string{"object"}
	}
	if t := typeOf(s); t != "" {
		return []string{t}
	}
	return nil
}

func mediaExamples(media S) []any {
	var out []any
	add := func(v any) {
		if s, ok := v.(string); ok {
			var parsed any
			if decode(s, &parsed) == nil {
				v = parsed
			}
		}
		if v != nil {
			out = append(out, v)
		}
	}
	if ex, ok := media["example"]; ok {
		add(ex)
	}
	examples, _ := media["examples"].(S)
	for _, name := range sortedKeys(examples) {
		ex, _ := examples[name].(S)
		if v, ok := ex["value"]; ok {
			add(v)
		}
	}
	return out
}

func pageTitle(page string) string {
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

func normalizePath(path string) string {
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func skipPath(path string) bool {
	for _, frag := range skipPathFragments {
		if strings.Contains(path, frag) {
			return true
		}
	}
	return false
}

// decode parses JSON keeping numbers as json.Number so card numbers and
// amounts like 1.000 survive unchanged.
func decode(data string, v any) error {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing data after JSON value")
	}
	return nil
}

func typeOf(s S) string {
	switch t := s["type"].(type) {
	case string:
		return t
	case []any:
		for _, v := range t {
			if str, ok := v.(string); ok && str != "null" {
				return str
			}
		}
	}
	if _, ok := s["properties"]; ok {
		return "object"
	}
	return ""
}

func propsOf(s S) S {
	props, _ := s["properties"].(S)
	return props
}

func isNumeric(t string) bool { return t == "integer" || t == "number" }

func clone(s S) S {
	out := make(S, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
