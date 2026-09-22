package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/kzheart/klib-cli/internal/cloudcli"
)

func main() {
	dir := os.Getenv("KLIB_CONFIG_DIR")
	var err error
	if dir == "" {
		dir, err = os.UserConfigDir()
		dir = filepath.Join(dir, "klib")
	}
	if err != nil {
		os.Exit(2)
	}
	os.Exit(cloudcli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, filepath.Join(dir, "credentials.json")))
}
