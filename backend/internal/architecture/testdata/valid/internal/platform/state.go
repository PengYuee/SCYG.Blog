// Package platform proves exact immutable global exemptions.
package platform

import (
	"embed"
	stdlibErrors "errors"
)

// ErrUnavailable is an immutable standard-library sentinel.
var ErrUnavailable = stdlibErrors.New("unavailable")

// Assets contains compile-checked embedded fixture data.
//
//go:embed asset.txt
var Assets embed.FS
