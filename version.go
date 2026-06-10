package slidex

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed VERSION
var releaseVersion string

var releaseBaseVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

func Version() string {
	version := strings.TrimSpace(releaseVersion)
	if !IsReleaseBaseVersion(version) {
		panic("VERSION must contain one exact release version")
	}
	return version
}

func IsReleaseBaseVersion(version string) bool {
	return releaseBaseVersionPattern.MatchString(strings.TrimSpace(version))
}
