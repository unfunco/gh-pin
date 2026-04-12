package main

import "strings"

// version defaults local builds to dev and is overridden for tagged releases.
var version = "dev"

func formatUserAgent(extensionVersion string) string {
	extensionVersion = strings.TrimSpace(extensionVersion)
	if extensionVersion == "" {
		extensionVersion = "dev"
	}

	return "gh-pin/" + extensionVersion
}
