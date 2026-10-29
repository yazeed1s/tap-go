// Command create-charge creates a test charge and prints the Tap payment
// page URL.
//
//	TAP_SECRET_KEY=sk_test_xxx go run ./examples/create-charge
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/yazeed1s/tap-go"
)

func main() {
	client := tap.NewClient(os.Getenv("TAP_SECRET_KEY"))

	charge, err := client.Charges.Create(context.Background(), tap.CreateChargeRequest{
		Amount:      1,
		Currency:    "KWD",
		Description: "Order #123",
		Customer: tap.CreateChargeCustomer{
			FirstName: "Test",
			Email:     "test@example.com",
		},
		// src_all lets the payer pick any enabled payment method on Tap's
		// hosted page.
		Source:   tap.CreateChargeSource{ID: tap.SourceAll},
		Redirect: tap.Redirect{URL: "https://example.com/return"},
	})
	if err != nil {
		var apiErr *tap.APIError
		if errors.As(err, &apiErr) {
			log.Fatalf("tap error %d: %v (body: %s)", apiErr.StatusCode, apiErr.Errors, apiErr.Body)
		}
		log.Fatal(err)
	}

	fmt.Println("charge:", charge.ID, charge.Status)
	if charge.Transaction != nil {
		fmt.Println("pay at:", charge.Transaction.URL)
	}
}
