package main

import "testing"

func TestServeOptions(t *testing.T) {
	t.Parallel()

	opts := serveOptions(false)
	if opts.Address != "registry.terraform.io/adrianhall/cfdev" {
		t.Errorf("Address = %q, want registry.terraform.io/adrianhall/cfdev", opts.Address)
	}
	if opts.Debug {
		t.Error("Debug = true, want false")
	}
}
