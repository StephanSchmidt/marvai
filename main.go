package main

import (
	"fmt"
	"os"

	"github.com/spf13/afero"

	"marvai/internal/marvai"
)

func main() {
	fs := afero.NewOsFs()
	if err := marvai.Run(os.Args, fs, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}