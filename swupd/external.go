package swupd

import (
	"io"
	"os/exec"
)

type externalWriter struct {
	cmd   *exec.Cmd
	input io.WriteCloser
}

// newExternalWriter creates a Writer that will filter the contents in the
// external program and then write to w.
func newExternalWriter(w io.Writer, program string, args ...string) (*externalWriter, error) {
	cmd := exec.Command(program, args...)
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = w
	err = cmd.Start()
	if err != nil {
		input.Close()
		return nil, err
	}
	return &externalWriter{cmd, input}, nil
}

func (ew *externalWriter) Write(p []byte) (int, error) {
	return ew.input.Write(p)
}

func (ew *externalWriter) Close() error {
	err := ew.input.Close()
	if err != nil {
		return err
	}
	return ew.cmd.Wait()
}

type externalReader struct {
	cmd    *exec.Cmd
	output io.ReadCloser
}

// newExternalReader creates a Reader that will filter the contents of r in the
// external program before returning it.
func newExternalReader(r io.Reader, program string, args ...string) (*externalReader, error) {
	cmd := exec.Command(program, args...)
	cmd.Stdin = r
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		output.Close()
		return nil, err
	}
	return &externalReader{cmd, output}, nil
}

func (er *externalReader) Read(p []byte) (int, error) {
	return er.output.Read(p)
}

func (er *externalReader) Close() error {
	err := er.output.Close()
	if err != nil {
		return err
	}
	return er.cmd.Wait()
}
