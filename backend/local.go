// Copyright (c) 2017 Gorillalabs. All rights reserved.

package backend

import (
	"io"
	"os/exec"

	"github.com/juju/errors"
)

type PowerShellVersion int

const (
	// Use Windows Powershell (5.1)
	WindowsPowerShell PowerShellVersion = iota

	// Use pwsh if found on system (version > 5)
	Pwsh
)

// Local represents a PowerShell session on the local machine, that is, the machine executing this code.
type Local struct {
	// Requested version of PowerShell to start.
	Version PowerShellVersion // Default 5.1
}

func (b *Local) StartProcess(cmd string, args ...string) (Waiter, io.Writer, io.Reader, io.Reader, error) {
	command := exec.Command(cmd, args...)

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, errors.Annotate(err, "Could not get hold of the PowerShell's stdin stream")
	}

	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, errors.Annotate(err, "Could not get hold of the PowerShell's stdout stream")
	}

	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, errors.Annotate(err, "Could not get hold of the PowerShell's stderr stream")
	}

	err = command.Start()
	if err != nil {
		return nil, nil, nil, nil, errors.Annotate(err, "Could not spawn PowerShell process")
	}

	return command, stdin, stdout, stderr, nil
}
