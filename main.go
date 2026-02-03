package main

import (
	"flag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/devindice/terraform-provider-sambadns/internal/provider"
)

// version is set at build time via ldflags
var version = "dev"

func main() {
	var debugMode bool

	flag.BoolVar(&debugMode, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := &plugin.ServeOpts{
		ProviderFunc: provider.New(version),
	}

	if debugMode {
		opts.Debug = true
		opts.ProviderAddr = "registry.terraform.io/devindice/sambadns"
	}

	plugin.Serve(opts)
}
