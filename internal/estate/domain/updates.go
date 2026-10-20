package domain

import (
	"regexp"
	"time"
)

// Update is one open Renovate pull request that moves an image to a newer version.
type Update struct {
	Dependency string
	Version    string
	Repository string
	Number     int
	URL        string
	Title      string
	Opened     time.Time
}

// renovateTitle is how Renovate titles a pull request that moves one image's tag: `update
// n8nio/n8n docker tag to v2.44.0`, or `update dependency x to 1.2.3`, after any commit prefix.
var renovateTitle = regexp.MustCompile(`(?i)\bupdate (?:dependency )?(\S+?)(?: docker tag)? to (v?\d+(?:\.\d+)*[0-9A-Za-z._+-]*)\s*$`)

// UpdateOf reads the dependency and version out of a Renovate pull request's title. A title that
// moves no one image to a version, a digest pin or a group of updates, reads as none.
func UpdateOf(title string) (dependency, version string, ok bool) {
	m := renovateTitle.FindStringSubmatch(title)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}
