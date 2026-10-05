package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"git.ddupan.top/panxiao81/terraform-provider-homelab/internal/provider"
)

// version is set by the release build (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "terraform.lab.ddupan.top/ddupan/homelab",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
