// Copyright 2026 Adrian Hall
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/adrianhall/terraform-provider-cfdev/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers")
	flag.Parse()

	if err := providerserver.Serve(context.Background(), provider.New(version), serveOptions(debug)); err != nil {
		log.Fatal(err.Error())
	}
}

func serveOptions(debug bool) providerserver.ServeOpts {
	return providerserver.ServeOpts{
		Address: "registry.terraform.io/adrianhall/cfdev",
		Debug:   debug,
	}
}
