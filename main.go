package main

import (
	"os"

	"github.com/k8nstantin/OpenPraxis/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
