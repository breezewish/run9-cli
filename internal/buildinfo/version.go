package buildinfo

import "strings"

// Version is the build-time CLI version injected by release builds.
var Version = "dev"

// DisplayVersion returns one stable version string for user-visible output.
func DisplayVersion() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		return "dev"
	}
	return version
}
