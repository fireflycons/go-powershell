// Copyright (c) 2017 Gorillalabs. All rights reserved.
// Portions copyright (c) 2025 Firefly IT Consulting Ltd.

// Package powershell provides an interface to run PowerShell code in a powershell
// session that remains "hot", such that you do not have to create a new instance
// of the powershell process with each invocation.
package powershell

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fireflycons/go-powershell/backend"
	"github.com/fireflycons/go-powershell/utils"
	"github.com/juju/errors"
	"golang.org/x/sync/errgroup"
)

const newline = "\r\n"

// ShellOptions represents options passed to the shell when it is started.
type ShellOptions struct {
	modulesToLoad []string
}

type ShellOptionFunc func(*ShellOptions)

// Shell is the interface to a PowerShell session
type Shell interface {
	Execute(cmd string) (string, string, error)
	ExecuteWithContext(ctx context.Context, cmd string) (string, string, error)
	Exit()
}

// concrete implementation of shell
type shell struct {
	handle  backend.Waiter
	backend backend.Starter
	stdin   io.Writer
	stdout  io.Reader
	stderr  io.Reader
	lock    *sync.Mutex
}

// New creates a new PowerShell session
func New(backend backend.Starter, opts ...ShellOptionFunc) (Shell, error) {

	s := &shell{
		backend: backend,
		lock:    &sync.Mutex{},
	}

	if err := s.start(); err != nil {
		return nil, err
	}

	return s, nil
}

// WithModules specifies a list of PowerShell modules to
// import into the shell when it starts
func WithModules(modules ...string) ShellOptionFunc {
	return func(so *ShellOptions) {
		so.modulesToLoad = modules
	}
}

// Execute runs PowerShell script in the session instance, capturing stdout and stderr streams
//
// This call may block indefinitely if the command is not well formed.
func (s *shell) Execute(cmd string) (string, string, error) {

	return s.ExecuteWithContext(context.TODO(), cmd)
}

// ExecuteWithContext runs PowerShell script in the session instance, capturing stdout and stderr streams.
//
// The context allows cancellation of the command. Streams to/from the PowerShell session may
// block indefinitely if the command is not well formed as the input may still be waiting.
//
// Note that if the error is "context deadline exceeded", the underlying session will be
// unstable and it will be restarted. A restarted shell _may_ be unstable!
func (s *shell) ExecuteWithContext(ctx context.Context, cmd string) (string, string, error) {

	// Lock the shell so that only one thread can execute a command at a time
	s.lock.Lock()
	sout, serr, err := s.executeWithContext(ctx, cmd)
	s.lock.Unlock()
	return sout, serr, err
}

// Exit releases the PowerShell session, terminating the underlying powershell.exe process
func (s *shell) Exit() {

	// Prevent panics if Exit is called multiple times
	if s == nil || s.handle == nil {
		fmt.Fprintf(os.Stderr, "Warning: go-powershell: Attempted to exit a shell that is already closed.\n")
		return
	}

	// Discard error here and everywhere else. We are binning the session
	_, _ = s.stdin.Write([]byte("exit" + newline))

	// if it's possible to close stdin, do so (some backends, like the local one,
	// do support it)
	closer, ok := s.stdin.(io.Closer)
	if ok {
		_ = closer.Close()
	}

	_ = s.handle.Wait()

	s.handle = nil
	s.stdin = nil
	s.stdout = nil
	s.stderr = nil
}

func (s *shell) executeWithContext(ctx context.Context, cmd string) (string, string, error) {
	if s.handle == nil {
		return "", "", errors.Annotate(errors.New(cmd), "Cannot execute commands on closed shells.")
	}

	outBoundary := createBoundary()
	errBoundary := createBoundary()

	// wrap the command in special markers so we know when to stop reading from the pipes
	// and also a try block to correctly capture errors.
	// The finally block is needed to ensure that the boundaries are always written
	// even if the command itself contains an exit statement.
	full := fmt.Sprintf("try { %s } catch { [Console]::Error.WriteLine($_.Exception.Message) } finally { [Console]::WriteLine('%s'); [Console]::Error.WriteLine('%s') }%s", cmd, outBoundary, errBoundary, newline)

	_, err := s.stdin.Write([]byte(full))
	if err != nil {
		return "", "", errors.Annotate(errors.Annotate(err, cmd), "Could not send PowerShell command")
	}

	// read stdout and stderr
	sout := ""
	serr := ""

	eg := &errgroup.Group{}

	eg.Go(func() error {
		return streamReader(ctx, s.stdout, outBoundary, &sout)
	})

	eg.Go(func() error {
		return streamReader(ctx, s.stderr, errBoundary, &serr)
	})

	err = eg.Wait()

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {

			if err1 := s.restart(); err1 != nil {
				err = errors.Wrap(err, err1)
			}
		}

		return "", err.Error(), err
	}

	if len(serr) > 0 {
		return sout, serr, errors.Annotate(errors.New(cmd), serr)
	}

	return sout, serr, nil
}

func (s *shell) start() error {

	handle, stdin, stdout, stderr, err := s.backend.StartProcess("powershell.exe", "-NoExit", "-Command", "-")
	if err != nil {
		return err
	}

	s.handle = handle
	s.stdin = stdin
	s.stdout = stdout
	s.stderr = stderr

	return nil
}

func (s *shell) restart() error {

	s.Exit()
	err := s.start()
	// Wait a fraction for new shell to stabilize
	time.Sleep(time.Millisecond * 500)
	return err
}

func streamReader(ctx context.Context, stream io.Reader, boundary string, buffer *string) error {

	// read all output until we have found our boundary token
	output := strings.Builder{}
	bufsize := 64
	marker := boundary + newline
	buf := make([]byte, bufsize)

	for {
		var (
			read int
			err  error
		)

		if ctx != context.TODO() {
			read, err = readWithContext(ctx, stream, buf)
		} else {
			read, err = stream.Read(buf)
		}

		if err != nil {
			return err
		}

		output.Write(buf[:read])

		if strings.HasSuffix(output.String(), marker) {
			break
		}
	}

	*buffer = strings.TrimSuffix(output.String(), marker)

	return nil
}

// ReadWithContext reads from r into buf, returning when either
// the read completes, the context is canceled, or an error occurs.
func readWithContext(ctx context.Context, r io.Reader, buf []byte) (int, error) {
	pr, pw := io.Pipe()

	// Start a background copier that forwards from r -> pw.
	go func() {
		_, err := io.Copy(pw, r)
		// When the copy ends or fails, close the pipe.
		_ = pw.CloseWithError(err)
	}()

	// Ensure the pipe is closed when context expires.
	go func() {
		<-ctx.Done()
		_ = pw.CloseWithError(ctx.Err())
	}()

	// Now do a normal blocking read — this is cancellable via ctx.
	n, err := pr.Read(buf)
	if errors.Is(err, io.ErrClosedPipe) {
		return n, ctx.Err()
	}
	return n, err
}

func createBoundary() string {
	return "$boundary" + utils.CreateRandomString(12) + "$"
}
