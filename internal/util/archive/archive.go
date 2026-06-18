package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/lxc/incus/v7/shared/subprocess"
)

type nullWriteCloser struct {
	*bytes.Buffer
}

type Unpacker []string

type Packer []string

// Reader returns a reader from the supplied stream.
// The returned cancelFunc should be called when finished with reader to clean up any resources used.
// This can be done before reading to the end of the tarball if desired.
func Reader(ctx context.Context, r io.Reader, unpacker Unpacker) (io.Reader, context.CancelFunc, error) {
	_, cancelFunc := context.WithCancel(ctx)

	if len(unpacker) == 0 {
		return r, cancelFunc, nil
	}

	// Setup the command.
	var buffer bytes.Buffer
	pipeReader, pipeWriter := io.Pipe()
	cmd := exec.Command(unpacker[0], unpacker[1:]...)
	cmd.Stdin = r
	cmd.Stdout = pipeWriter
	cmd.Stderr = &nullWriteCloser{&buffer}

	// Run the command.
	err := cmd.Start()
	if err != nil {
		return nil, cancelFunc, subprocess.NewRunError(unpacker[0], unpacker[1:], err, nil, &buffer)
	}

	// Close the pipe upon completion.
	chDone := make(chan struct{}, 1)
	go func() {
		err := cmd.Wait()
		_ = pipeWriter.CloseWithError(err)
		close(chDone)
	}()

	ctxCancelFunc := cancelFunc

	// Now that unpacker process has started, wrap context cancel function with one that waits for
	// the unpacker process to complete.
	cancelFunc = func() {
		ctxCancelFunc()
		_ = pipeWriter.Close()
		<-chDone
	}

	return pipeReader, cancelFunc, nil
}

func Writer(ctx context.Context, w io.Writer, packer Packer) (io.WriteCloser, error) {
	if len(packer) == 0 {
		return &nopWriteCloser{w: w}, nil
	}

	cmd := exec.Command(packer[0], packer[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &buffer

	err = cmd.Start()
	if err != nil {
		stdin.Close()
		return nil, subprocess.NewRunError(packer[0], packer[1:], err, nil, &buffer)
	}

	return &cmdWriter{stdin: stdin, cmd: cmd, stderr: &buffer}, nil
}

type cmdWriter struct {
	stdin  io.WriteCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (c *cmdWriter) Write(p []byte) (int, error) {
	return c.stdin.Write(p)
}

func (c *cmdWriter) Close() error {
	closeErr := c.stdin.Close()

	err := c.cmd.Wait()
	if err != nil {
		return fmt.Errorf("Command failed: %w: %s", errors.Join(err, closeErr), c.stderr.String())
	}

	return closeErr
}

type nopWriteCloser struct {
	w io.Writer
}

func (n *nopWriteCloser) Write(p []byte) (int, error) {
	return n.w.Write(p)
}

func (n *nopWriteCloser) Close() error {
	return nil
}
