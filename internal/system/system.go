// Package system holds the platform code: command lookup and the keychain.
package system

import "errors"

var (
	ErrNotFound    = errors.New("not found")
	ErrUnsupported = errors.New("not supported on this platform")
)

// Keychain stores generic passwords under one service. File is a keychain file's
// absolute path; empty means the user's default keychain.
type Keychain struct {
	Service string
	File    string
}
