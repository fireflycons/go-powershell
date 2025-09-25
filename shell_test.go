package powershell

import (
	"fmt"
	"testing"

	"github.com/fireflycons/go-powershell/backend"
	"github.com/stretchr/testify/require"
)

func TestShell(t *testing.T) {

	// choose a backend
	back := &backend.Local{}

	// start a local powershell process
	shell, err := New(back)
	if err != nil {
		panic(err)
	}
	defer shell.Exit()

	// ... and interact with it
	stdout, _, err := shell.Execute("Get-WmiObject -Class Win32_Processor")

	require.NoError(t, err)
	fmt.Println(stdout)
	stdout, _, err = shell.Execute("gci -Path C:\\")
	require.NoError(t, err)
	fmt.Println(stdout)

	_, stderr, err := shell.Execute("throw 'This is an error'")
	require.Error(t, err)
	fmt.Println(stderr)
}
