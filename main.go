// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/wncservices/terraform-provider-omada/internal/provider"
)

// version is set by the linker at release time (see .goreleaser.yml).
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// Must match the source address in required_providers, e.g. wncservices/omada.
		Address: "registry.terraform.io/wncservices/omada",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
