// Package version holds the build version, stamped at release time with
//
//	-ldflags "-X github.com/StrStark/advance-download-manager/internal/version.Version=1.2.3"
package version

// Version is the running build's version (semver without the "v").
var Version = "0.0.0-dev"

// Repo is the GitHub repository releases are published to.
const Repo = "StrStark/advance-download-manager"

// IsDev reports whether this is an unreleased development build.
func IsDev() bool { return Version == "0.0.0-dev" }
