// Package buildinfo contains public build identity, set by the build pipeline.
package buildinfo

import "fmt"

// These defaults keep source builds useful without pretending to be a release.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	return fmt.Sprintf("nox %s (commit %s, built %s)", Version, Commit, Date)
}
