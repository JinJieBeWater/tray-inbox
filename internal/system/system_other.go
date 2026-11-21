//go:build !darwin

package system

import "os/exec"

func LookPath(name string) (string, error) { return exec.LookPath(name) }

func (Keychain) Get(account string) (string, error) { return "", ErrUnsupported }

func (Keychain) Set(account, secret string) error { return ErrUnsupported }
