package main

import (
	"os"

	"github.com/konghang/ember/backend/internal/entrypoint"
)

func main() {
	os.Exit(entrypoint.Run(os.Args[1:]))
}
