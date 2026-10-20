// terraform-provider-rustinion manages Rustinion monitoring config
// (alert rules, notification channels, dashboards, synthetic checks) as
// Terraform resources, via the customer-facing config-as-code API
// documented in the main repo's web/src/routes/customer_api.rs. See
// README.md for setup/auth and docs/roadmap.md (main repo) for what
// this provider deliberately does not cover yet.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/provider"
)

// version is overridden at release-build time via
// `-ldflags "-X main.version=x.y.z"`; "dev" for a local build.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// The public Terraform Registry address (`source =
		// "CanuteTheGreat/rustinion"`); see RELEASING.md.
		Address: "registry.terraform.io/CanuteTheGreat/rustinion",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
