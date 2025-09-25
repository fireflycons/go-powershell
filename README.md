# go-powershell

This is a fork of the original Gorilla implementation with some enhancements
* Wrap all submitted commands in a `try` block to properly capture errors and ensure the output boundary markers are properly written.
* Make the session thread safe.
* Optimize the `streamReader` function to perform fewer allocations.

This package is inspired by [jPowerShell](https://github.com/profesorfalken/jPowerShell)
and allows one to run and remote-control a PowerShell session. Use this if you
don't have a static script that you want to execute, bur rather run dynamic
commands.

The session is kept hot in a single instance of `powershell.exe` such that you do not have the overhead of creating a new PowerShell process every time you want to run some commands.

## Installation

    go get github.com/fireflycons/go-powershell

## Usage

To start a PowerShell shell, you need a backend. Backends take care of starting
and controlling the actual `powershell.exe` process. In most cases, you will want
to use the Local backend, which just uses `os/exec` to start the process.

```go
package main

import (
	"fmt"

	ps "github.com/fireflycons/go-powershell"
	"github.com/fireflycons/go-powershell/backend"
)

func main() {
	// choose a backend
	back := &backend.Local{}

	// start a local powershell process
	shell, err := ps.New(back)
	if err != nil {
		panic(err)
	}
	defer shell.Exit()

	// ... and interact with it
	stdout, stderr, err := shell.Execute("Get-WmiObject -Class Win32_Processor")
	if err != nil {
		panic(err)
	}

	fmt.Println(stdout)
}
```

## Remote Sessions

You can use an existing PS shell to use PSSession cmdlets to connect to remote
computers. Instead of manually handling that, you can use the Session middleware,
which takes care of authentication. Note that you can still use the "raw" shell
to execute commands on the computer where the powershell host process is running.

```go
package main

import (
	"fmt"

	ps "github.com/fireflycons/go-powershell"
	"github.com/fireflycons/go-powershell/backend"
	"github.com/fireflycons/go-powershell/middleware"
)

func main() {
	// choose a backend
	back := &backend.Local{}

	// start a local powershell process
	shell, err := ps.New(back)
	if err != nil {
		panic(err)
	}

	// prepare remote session configuration
	config := middleware.NewSessionConfig()
	config.ComputerName = "remote-pc-1"

	// create a new shell by wrapping the existing one in the session middleware
	session, err := middleware.NewSession(shell, config)
	if err != nil {
		panic(err)
	}
	defer session.Exit() // will also close the underlying ps shell!

	// everything run via the session is run on the remote machine
	stdout, stderr, err = session.Execute("Get-WmiObject -Class Win32_Processor")
	if err != nil {
		panic(err)
	}

	fmt.Println(stdout)
}
```

Note that all commands that you execute are wrapped in special echo
statements to delimit the stdout/stderr streams. After ``.Execute()``ing a command,
you can therefore not access ``$LastExitCode`` anymore and expect meaningful
results.

## License

MIT, see LICENSE file.
