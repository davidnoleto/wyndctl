package cmd

import "testing"

func TestAllowedClientTypes(t *testing.T) {
	for _, want := range []string{"hotel", "multifamily"} {
		if !allowedClientTypes[want] {
			t.Errorf("%q should be an allowed client type", want)
		}
	}
	for _, bad := range []string{"", "Hotel", "hotels", "vacation_rental", "foo"} {
		if allowedClientTypes[bad] {
			t.Errorf("%q should NOT be an allowed client type", bad)
		}
	}
}
