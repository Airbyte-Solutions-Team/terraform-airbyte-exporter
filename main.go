package main

import (
	"log"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/cmd"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
