// Package billing creates Stripe customers used by the create-account flow.
package billing

import (
	"fmt"
	"io"
	"time"

	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/client"
)

// pollInterval controls how long FindOrCreateCustomer waits between Stripe
// search retries after creating a customer. Overridable from tests.
var pollInterval = 2 * time.Second

// pollMaxAttempts is the number of search retries after a create. With the
// default 2s interval this yields a ~60s timeout, matching the Python script.
var pollMaxAttempts = 30

// FindOrCreateCustomer searches Stripe for a customer with the given email
// and creates one if none exists. After creating, it polls for the customer
// to appear in search results, matching the Python
// mass_deployment.find_or_create_stripe_customer behavior.
//
// Returns the customer ID. progressOut, if non-nil, receives human-readable
// progress lines.
func FindOrCreateCustomer(apiKey, email, name, description string, progressOut io.Writer) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("stripe api_key is not configured")
	}

	sc := &client.API{}
	sc.Init(apiKey, nil)
	return findOrCreateCustomer(sc, email, name, description, progressOut)
}

func findOrCreateCustomer(sc *client.API, email, name, description string, progressOut io.Writer) (string, error) {
	if id, ok, err := findCustomer(sc, email); err != nil {
		return "", err
	} else if ok {
		writef(progressOut, "Customer already exists for email: %s, skipping\n", email)
		return id, nil
	}

	writef(progressOut, "Creating stripe customer for email: %s\n", email)
	if _, err := sc.Customers.New(&stripe.CustomerParams{
		Email:       stripe.String(email),
		Name:        stripe.String(name),
		Description: stripe.String(description),
	}); err != nil {
		return "", fmt.Errorf("creating stripe customer: %w", err)
	}
	writef(progressOut, "Stripe customer creation initiated for email: %s\n", email)

	for i := 0; i < pollMaxAttempts; i++ {
		time.Sleep(pollInterval)
		if id, ok, err := findCustomer(sc, email); err != nil {
			return "", err
		} else if ok {
			writef(progressOut, "Stripe customer created for email: %s\n", email)
			return id, nil
		}
	}
	return "", fmt.Errorf("timeout: stripe customer for %q did not appear within poll window", email)
}

func findCustomer(sc *client.API, email string) (string, bool, error) {
	iter := sc.Customers.Search(&stripe.CustomerSearchParams{
		SearchParams: stripe.SearchParams{
			Query: fmt.Sprintf("email:'%s'", email),
		},
	})
	for iter.Next() {
		c := iter.Customer()
		if c != nil {
			return c.ID, true, nil
		}
	}
	if err := iter.Err(); err != nil {
		return "", false, fmt.Errorf("searching stripe customers: %w", err)
	}
	return "", false, nil
}

func writef(w io.Writer, format string, args ...interface{}) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}
