package powershell

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/fireflycons/go-powershell/backend"
	"github.com/stretchr/testify/require"
)

func TestShell(t *testing.T) {

	// choose a backend
	back := &backend.Local{}

	// start a local powershell process
	shell, err := New(back)
	require.NoError(t, err)
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

func TestShellConcurrent(t *testing.T) {

	worker := func(id int, shell Shell, t *testing.T, wg *sync.WaitGroup) {

		defer wg.Done()
		for i := range 5 {
			sleep := rand.Intn(50) + 50
			cmd := fmt.Sprintf("Start-Sleep -Milliseconds %d; 'Worker %d - Iteration %d'", sleep, id, i)
			stdout, stderr, err := shell.Execute(cmd)
			require.NoError(t, err)
			require.Empty(t, stderr)
			expected := fmt.Sprintf("Worker %d - Iteration %d", id, i)
			require.Equal(t, expected, strings.TrimSpace(stdout))
			fmt.Printf("Worker %d completed iteration %d\n", id, i)
		}
	}

	// choose a backend
	back := &backend.Local{}

	// start a local powershell process
	shell, err := New(back)
	require.NoError(t, err)
	defer shell.Exit()

	// start multiple workers that all use the same shell
	const numWorkers = 5

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker(i, shell, t, &wg)
	}
	wg.Wait()
}
