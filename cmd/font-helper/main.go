// Command font-helper performs system-wide font installs for skrifttype.
//
// The logic lives in internal/fonthelper so its tests do not link this
// package's elevated (requireAdministrator) Windows resource file.
package main

import (
	"os"

	"github.com/amnuts/skrifttype/internal/fonthelper"
)

func main() {
	os.Exit(fonthelper.Main())
}
