package tap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(
	t *testing.T,
	handler http.HandlerFunc,
	opts ...Option,
) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(
		"sk_test_123",
		append([]Option{WithBaseURL(srv.URL)}, opts...)...)
}

func TestCreateCharge(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/charges" {
			t.Errorf("got %s %s, want POST /v2/charges", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk_test_123" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["amount"] != 1.5 || body["currency"] != "KWD" {
			t.Errorf("body = %v", body)
		}
		if body["threeDSecure"] != false {
			t.Errorf("explicit threeDSecure=false was not sent: %v", body)
		}
		if _, ok := body["save_card"]; ok {
			t.Errorf("unset save_card was sent: %v", body)
		}
		w.Write(
			[]byte(
				`{"id":"chg_123","object":"charge","amount":1.500,"currency":"KWD","status":"INITIATED"}`,
			),
		)
	})

	threeDS := false
	charge, err := client.Charges.Create(
		context.Background(),
		CreateChargeRequest{
			Amount:       1.5,
			Currency:     "KWD",
			ThreeDSecure: &threeDS,
			Customer: CreateChargeCustomer{
				FirstName: "Test",
				Email:     "test@example.com",
			},
			Source:   CreateChargeSource{ID: "src_all"},
			Redirect: Redirect{URL: "https://example.com/return"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if charge.ID != "chg_123" || charge.Amount != 1.5 ||
		charge.Status != "INITIATED" {
		t.Errorf("charge = %+v", charge)
	}
}

func TestRetrieveEscapesPathParams(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet ||
			r.URL.EscapedPath() != "/v2/card/cus%2F1/card_2" {
			t.Errorf("got %s %s", r.Method, r.URL.EscapedPath())
		}
		w.Write([]byte(`{"id":"card_2","object":"card"}`))
	})
	card, err := client.Cards.Retrieve(context.Background(), "cus/1", "card_2")
	if err != nil {
		t.Fatal(err)
	}
	if card.ID != "card_2" {
		t.Errorf("card = %+v", card)
	}
}

func TestAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_1")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(
			[]byte(
				`{"errors":[{"code":"1108","description":"Invalid amount"}]}`,
			),
		)
	})
	_, err := client.Charges.Retrieve(context.Background(), "chg_1")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != 400 || apiErr.Message != "Invalid amount" ||
		len(apiErr.Errors) != 1 || apiErr.Errors[0].Code != "1108" {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if apiErr.Header.Get("X-Request-Id") != "req_1" {
		t.Errorf("header not kept")
	}
	if got := err.Error(); got != "tap: status 400: 1108: Invalid amount" {
		t.Errorf("Error() = %q", got)
	}
}

func TestAPIErrorSingleObjectAndRawBody(t *testing.T) {
	tests := []struct {
		body, message string
	}{
		{
			`{"error":{"code":"9999","error":"INTERNAL_SERVER_ERROR"}}`,
			"INTERNAL_SERVER_ERROR",
		},
		{`upstream timeout`, "upstream timeout"},
		{``, "Bad Gateway"},
	}
	for _, tt := range tests {
		client := newTestClient(
			t,
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte(tt.body))
			},
		)
		_, err := client.Tokens.Retrieve(context.Background(), "tok_1")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Message != tt.message {
			t.Errorf("body %q: err = %v", tt.body, err)
		}
	}
}

func TestDownloadReturnsRawBody(t *testing.T) {
	const csv = "charge_id,amount\nchg_1,1.000\n"
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/charges/download" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(csv))
	})
	got, err := client.Charges.Download(
		context.Background(),
		DownloadChargesRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != csv {
		t.Errorf("got %q", got)
	}
}

func TestTokenVariantsShareEndpoint(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v2/tokens" ||
			!strings.Contains(string(body), `"saved_card"`) {
			t.Errorf("got %s %s", r.URL.Path, body)
		}
		w.Write([]byte(`{"id":"tok_1","object":"token"}`))
	})
	token, err := client.Tokens.CreateFromSavedCard(
		context.Background(),
		CreateSavedCardTokenRequest{
			SavedCard: &CreateSavedCardTokenSavedCard{
				CardID:     "card_1",
				CustomerID: "cus_1",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if token.ID != "tok_1" {
		t.Errorf("token = %+v", token)
	}
}

func TestWithLanguage(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("lang_code"); got != "ar" {
			t.Errorf("lang_code = %q", got)
		}
		w.Write([]byte(`{}`))
	}, WithLanguage("ar"))
	if _, err := client.Customers.Retrieve(context.Background(), "cus_1"); err != nil {
		t.Fatal(err)
	}
}

func TestFileUpload(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("purpose") != "identity_document" ||
			r.FormValue("file_link_create") != "true" {
			t.Errorf("form = %v", r.MultipartForm.Value)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(f)
		if hdr.Filename != "id.pdf" || string(data) != "pdf-bytes" {
			t.Errorf("file = %s %q", hdr.Filename, data)
		}
		w.Write([]byte(`{"id":"file_1","object":"file"}`))
	})
	file, err := client.Files.Create(context.Background(), CreateFileRequest{
		Purpose:        "identity_document",
		Title:          "Civil ID",
		FileName:       "id.pdf",
		File:           strings.NewReader("pdf-bytes"),
		FileLinkCreate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != "file_1" {
		t.Errorf("file = %+v", file)
	}
}

func TestListAllCursorPaging(t *testing.T) {
	var cursors []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		cursor, _ := req["starting_after"].(string)
		cursors = append(cursors, cursor)
		switch cursor {
		case "":
			w.Write(
				[]byte(
					`{"has_more":true,"charges":[{"id":"chg_1"},{"id":"chg_2"}]}`,
				),
			)
		case "chg_2":
			w.Write([]byte(`{"has_more":false,"charges":[{"id":"chg_3"}]}`))
		default:
			t.Errorf("unexpected cursor %q", cursor)
		}
	})
	var ids []string
	for charge, err := range client.Charges.ListAll(context.Background(), ListChargesRequest{Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, charge.ID)
	}
	if strings.Join(ids, ",") != "chg_1,chg_2,chg_3" ||
		strings.Join(cursors, ",") != ",chg_2" {
		t.Errorf("ids = %v, cursors = %q", ids, cursors)
	}
}

func TestListAllPageNumbersAndEarlyStop(t *testing.T) {
	var pages []float64
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		page, _ := req["page"].(float64)
		pages = append(pages, page)
		w.Write([]byte(`{"has_more":true,"intents":[{"id":"a"},{"id":"b"}]}`))
	})
	n := 0
	for _, err := range client.Intents.ListAll(context.Background(), ListIntentsRequest{}) {
		if err != nil {
			t.Fatal(err)
		}
		if n++; n == 3 {
			break
		}
	}
	// Two pages fetched (1, 2); breaking stops before a third.
	if len(pages) != 2 || pages[0] != 1 || pages[1] != 2 {
		t.Errorf("pages = %v", pages)
	}
}

func TestListAllYieldsError(t *testing.T) {
	calls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"has_more":true,"refunds":[{"id":"re_1"}]}`))
	})
	var got []string
	var lastErr error
	for refund, err := range client.Refunds.ListAll(context.Background(), ListRefundsRequest{}) {
		if err != nil {
			lastErr = err
			continue
		}
		got = append(got, refund.ID)
	}
	var apiErr *APIError
	if len(got) != 1 || !errors.As(lastErr, &apiErr) || calls != 2 {
		t.Errorf("got %v, err %v, calls %d", got, lastErr, calls)
	}
}

func TestCreateChargeWithDestinations(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Destinations struct {
				Destination []map[string]any `json:"destination"`
			} `json:"destinations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		d := body.Destinations.Destination
		if len(d) != 1 || d[0]["id"] != "480593" || d[0]["amount"] != 9.5 ||
			d[0]["currency"] != "SAR" {
			t.Errorf("destinations = %v", d)
		}
		w.Write(
			[]byte(
				`{"id":"chg_1","object":"charge","status":"CAPTURED","amount":10,"currency":"SAR",
			"destinations":{"amount":9.5,"currency":"SAR","count":1,
			"destination":[{"id":"480593","amount":9.5,"currency":"SAR"}]}}`,
			),
		)
	})
	chg, err := client.Charges.Create(context.Background(), CreateChargeRequest{
		Amount:   10,
		Currency: "SAR",
		Customer: CreateChargeCustomer{
			FirstName: "Test",
			Email:     "test@example.com",
		},
		Source:   CreateChargeSource{ID: SourceAll},
		Redirect: Redirect{URL: "https://example.com/return"},
		Destinations: &ChargeDestinations{Destination: []ChargeDestination{
			{ID: "480593", Amount: 9.5, Currency: "SAR"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if chg.Destinations == nil || len(chg.Destinations.Destination) != 1 ||
		chg.Destinations.Destination[0].Amount != 9.5 {
		t.Errorf("response destinations = %+v", chg.Destinations)
	}
}
