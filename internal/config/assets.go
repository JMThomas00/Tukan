package config

import _ "embed"

// LogoContent is the ASCII art splash logo, embedded at compile time.
//
//go:embed logo.txt
var LogoContent string
