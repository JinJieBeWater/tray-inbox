//go:build !darwin

package system

import (
	"context"
	"os/exec"
)

func LookPath(name string) (string, error) { return exec.LookPath(name) }

func LookPathContext(_ context.Context, name string) (string, error) { return exec.LookPath(name) }

func OpenForPID(context.Context, int, string) error { return ErrUnsupported }

func (Keychain) Get(account string) (string, error) { return "", ErrUnsupported }

func (Keychain) Set(account, secret string) error { return ErrUnsupported }
