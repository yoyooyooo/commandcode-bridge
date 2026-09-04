package protocol

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var embeddedVersion string

func CommandCodeVersion() string {
	return strings.TrimSpace(embeddedVersion)
}
