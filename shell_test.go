// Copyright (c) 2025 Firefly IT Consulting Ltd.

package powershell

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fireflycons/go-powershell/backend"
	"github.com/fireflycons/go-powershell/utils"
	"github.com/stretchr/testify/require"
)

func TestShell(t *testing.T) {

	shell, err := New(&backend.Local{})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	stdout, _, err := shell.Execute("Get-WmiObject -Class Win32_Processor")

	require.NoError(t, err)
	fmt.Println(stdout)
	stdout, _, err = shell.Execute("gci -Path C:\\")
	require.NoError(t, err)
	fmt.Println(stdout)
}

func TestInvalidCommands(t *testing.T) {

	shell, err := New(&backend.Local{})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	tests := []struct {
		name        string
		commands    string
		expectError error
	}{
		{
			name:     "Leading line break",
			commands: "\rWrite-Host 'hello'",
		},
		{
			name:     "Trailing line break",
			commands: "Write-Host 'hello'\n",
		},
		{
			name:        "Embedded LF",
			commands:    "Write-Host 'hello'\nWrite-Host 'goodbye'",
			expectError: ErrInvalidCommandString,
		},
		{
			name:        "Embedded CR",
			commands:    "Write-Host 'hello'\rWrite-Host 'goodbye'",
			expectError: ErrInvalidCommandString,
		},
		{
			name:        "Embedded CRLF",
			commands:    "Write-Host 'hello'\r\nWrite-Host 'goodbye'",
			expectError: ErrInvalidCommandString,
		},
		{
			name:     "Two commands separated by ;",
			commands: "Write-Host 'hello' ; Write-Host 'goodbye'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := shell.Execute(test.commands)

			if test.expectError != nil {
				require.ErrorIs(t, err, test.expectError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestShellWithContext(t *testing.T) {

	shell, err := New(&backend.Local{})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	timeout := time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Sleep past the context deadline
	sleepMSecs := int((timeout + (500 * time.Millisecond)) / time.Millisecond)
	_, _, err = shell.ExecuteWithContext(ctx, fmt.Sprintf("Start-Sleep -Milliseconds %d", sleepMSecs))
	cancel()

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// Now make sure the session isn't broken
	ctx, cancel = context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stdout, _, err := shell.ExecuteWithContext(ctx, "Write-Host 'Hello'\r\n")
	require.NoError(t, err, "Second command died")
	fmt.Println(stdout)
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
	defer func() {
		_ = shell.Exit()
	}()

	// start multiple workers that all use the same shell
	const numWorkers = 5

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker(i, shell, t, &wg)
	}
	wg.Wait()
}

func TestShellExit(t *testing.T) {

	shell, err := New(&backend.Local{})
	require.NoError(t, err)
	_ = shell.Exit()

	// call Exit again - should not panic and should report already closed
	require.NotPanics(t, func() {
		require.ErrorIs(t, shell.Exit(), ErrShellClosed)
	})
}

func TestShellWriteStderr(t *testing.T) {

	shell, err := New(&backend.Local{Version: backend.WindowsPowerShell})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	// Any output on stderr will be seen as a command failure!
	_, serr, err := shell.Execute(`[Console]::Error.WriteLine("error")`)
	require.ErrorIs(t, err, ErrCommandFailed)
	require.Equal(t, "error", strings.TrimSpace(serr))
}

func TestShellExceptionThrown(t *testing.T) {
	shell, err := New(&backend.Local{Version: backend.WindowsPowerShell})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	msg := "This is an error"
	_, stderr, err := shell.Execute("throw '" + msg + "'")
	require.ErrorIs(t, err, ErrCommandFailed)
	require.Equal(t, msg, strings.TrimSpace(stderr))
}

func TestShellWriteClosedShell(t *testing.T) {
	shell, err := New(&backend.Local{})
	require.NoError(t, err)

	// Close it now
	_ = shell.Exit()

	_, _, err = shell.Execute("Write-Host")
	require.ErrorIs(t, err, ErrShellClosed)
}

func TestShellRunScript(t *testing.T) {
	script := `
Write-Host "hello"
Write-Host "goodbye"
`
	scriptFile := filepath.Join(os.TempDir(), utils.CreateRandomString(8)+".ps1")
	require.NoError(t, os.WriteFile(scriptFile, []byte(script), 0644), "Error writing script file")
	defer func() {
		_ = os.Remove(scriptFile)
	}()

	shell, err := New(&backend.Local{})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	sout, _, err := shell.Execute(". " + scriptFile)
	require.NoError(t, err)
	require.Contains(t, sout, "hello")
	require.Contains(t, sout, "goodbye")
}

func TestShellWindowsPowerShell(t *testing.T) {

	shell, err := New(&backend.Local{Version: backend.WindowsPowerShell})
	require.NoError(t, err)
	defer func() {
		_ = shell.Exit()
	}()

	require.Equal(t, int64(5), shell.Version().Major, "Expected PowerShell major version 5")
}

func TestShellPwsh(t *testing.T) {

	shell, err := New(&backend.Local{Version: backend.Pwsh})
	defer func() {
		_ = shell.Exit()
	}()
	require.NoError(t, err)

	v := shell.Version().Major
	require.Greater(t, v, int64(5), "Expected PowerShell major version > 5, but got %s. Maybe pwsh is not installed here", v)
}

func TestWithModules(t *testing.T) {

	// These modules should be present on all systems
	// but not imported by default
	modules := []string{"CimCmdlets", "DnsClient"}

	shell, err := New(
		&backend.Local{},
		WithModules(modules...),
	)
	defer func() {
		_ = shell.Exit()
	}()
	require.NoError(t, err)

	for _, mod := range modules {
		out, _, err := shell.Execute(fmt.Sprintf(`if (Get-Module %s) { "YES" } else {"NO"}`, mod))
		require.NoError(t, err)
		require.Equal(t, "YES", strings.TrimSpace(out))
	}
}

func TestReadWithContext(t *testing.T) {

	const interCommandDelay = time.Millisecond * 100

	pr, pw := io.Pipe()
	defer func() {
		_ = pr.Close()
		_ = pw.Close()
	}()

	ctx := context.Background()
	buf := make([]byte, 64)

	var wg sync.WaitGroup

	// Simulate the child process writing chunks to stdout.
	chunks := []string{
		"first response\r\n",
		"second response with more data\r\n",
		"final short\n",
	}

	wg.Go(
		func() {

			for _, c := range chunks {
				_, _ = pw.Write([]byte(c))
				// Simulate time gap between commands.
				time.Sleep(interCommandDelay)
			}
			// Stream remains open
		},
	)

	for i, c := range chunks {
		n, err := readWithContext(ctx, pr, buf)
		if err != nil {
			require.ErrorIs(t, err, io.EOF, "unexpected error on first read: %v", err)
		}
		got := string(buf[:n])
		require.Equal(t, c, got, "Chunk #%d: got %q", i+1, got)
	}

	// Now the input is exhausted so read should block
	ctx, cancel := context.WithTimeout(context.Background(), interCommandDelay*2)
	defer cancel()
	n, err := readWithContext(ctx, pr, buf)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, n, 0)
	wg.Wait()
}

func TestStreamReader(t *testing.T) {

	const interCommandDelay = time.Millisecond * 100

	pr, pw := io.Pipe()
	defer func() {
		_ = pr.Close()
		_ = pw.Close()
	}()

	testFunc := func(iter int) {

		ctx, cancel := context.WithTimeout(context.Background(), interCommandDelay*5)
		defer cancel()

		var wg sync.WaitGroup

		// Simulate the child process writing chunks to stdout.
		chunks := []struct {
			data     string
			boundary string
		}{
			{
				data:     "first response\n",
				boundary: createBoundary(),
			},
			{
				data:     "second response with more data that will be more than fits in the sixty four byte buffer that we use for intermediate reads\n",
				boundary: createBoundary(),
			},
			{
				data:     "final short\n",
				boundary: createBoundary(),
			},
		}

		wg.Go(func() {

			for _, c := range chunks {
				data := c.data + c.boundary
				_, err := pw.Write([]byte(data))
				require.NoError(t, err, "error writing stdout simulation pipe")
				// Simulate time gap between commands.
				time.Sleep(interCommandDelay)
			}
			// Stream remains open — no pw.Close() here!
		})

		time.Sleep(time.Millisecond)
		for i, c := range chunks {

			sout := ""
			err := streamReader(ctx, pr, c.boundary, &sout)

			if err != nil {
				require.ErrorIs(t, err, io.EOF, "unexpected error on read #%d, iter #%d: %v", i+1, iter+1, err)
			}
			require.Equal(t, c.data, sout, "Unexpected data on read #%d, iter #%d", i+1, iter+1)
		}

		// Now the pipe is empty, simulate a context cancel
		sout := ""
		err := streamReader(ctx, pr, createBoundary(), &sout)
		require.ErrorIs(t, err, context.DeadlineExceeded, "expected deadline exceeded but got %v", err)
		// We seem to need to send something to un-stall the pipe
		_, _ = pw.Write([]byte(" "))
		wg.Wait()
	}

	// Run the whole test twice to check that having stalled the pipe that it recovers.
	for i := range 2 {
		testFunc(i)
	}
}
