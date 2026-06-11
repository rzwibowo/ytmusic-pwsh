//go:build !windows

package main

import (
	"os"
	"os/exec"
)

func attachProcessToJob(_ *os.Process) error { return nil }
func hideProcessWindow(_ *exec.Cmd)          {}
