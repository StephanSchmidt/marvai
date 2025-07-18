package marvai

import (
	"fmt"

	"github.com/spf13/afero"
)

// ShowVersion displays the version information
func ShowVersion(fs afero.Fs, version string) error {
	fmt.Printf("marvai version %s\n", version)
	return nil
}
