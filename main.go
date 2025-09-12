package main

import (
	"context"
	"flag"
	"log"

	"terraform-provider-temporal/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// these will be set by the goreleaser configuration
// to appropriate values for the compiled binary.
var version string = "dev"

// goreleaser can pass other information to the main package, such as the specific commit
// https://goreleaser.com/cookbooks/using-main.version/

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "hashicorp/platacard/terraform-provider-temporal",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
