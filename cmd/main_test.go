package main

import "testing"

func TestListenAddressFormatsIPv6Host(t *testing.T) {
	got := listenAddress("::", "8079")
	want := "[::]:8079"

	if got != want {
		t.Fatalf("listenAddress() = %q, want %q", got, want)
	}
}

func TestListenAddressUsesDefaults(t *testing.T) {
	got := listenAddress("", "")
	want := "127.0.0.1:8079"

	if got != want {
		t.Fatalf("listenAddress() = %q, want %q", got, want)
	}
}
