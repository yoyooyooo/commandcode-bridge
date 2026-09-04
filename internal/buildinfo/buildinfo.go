package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	return fmt.Sprintf("commandcode-bridge %s (%s %s)", Version, Commit, Date)
}
