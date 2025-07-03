package main

import (
	"fmt"
	"os"

	"github.com/spf13/afero"

	"github.com/marvai-dev/marvai/internal/marvai"
)

var Version string

func GetVersion() string {
	if Version == "" {
		Version = "dev"
	}
	return Version
}

func main() {
	fs := afero.NewOsFs()
	if err := marvai.Run(os.Args, fs, os.Stderr, GetVersion()); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
