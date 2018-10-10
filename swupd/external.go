// Copyright 2018 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package swupd

import (
	"io"
	"os/exec"
)

// ExternalWriter filters a Writer with an external program. Every
// time Write is called, it will write to the program, and then the
// result written to the underlying Writer.
type ExternalWriter struct {
	cmd   *exec.Cmd
	input io.WriteCloser
}

// NewExternalWriter creates an ExternalWriter with the passed
// underlying Writer and the program to execute as filter.
func NewExternalWriter(w io.Writer, program string, args ...string) (*ExternalWriter, error) {
	cmd := exec.Command(program, args...)
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = w
	err = cmd.Start()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	return &ExternalWriter{cmd, input}, nil
}

func (ew *ExternalWriter) Write(p []byte) (int, error) {
	return ew.input.Write(p)
}

// Close properly finish the execution of an ExternalWriter.
func (ew *ExternalWriter) Close() error {
	err := ew.input.Close()
	if err != nil {
		return err
	}
	return ew.cmd.Wait()
}

// ExternalReader filters a Reader with an external program. Every
// time a Read is called, it will read from the output of the program,
// that reads from the underlying reader.
type ExternalReader struct {
	cmd    *exec.Cmd
	output io.ReadCloser
}

// NewExternalReader creates an ExternalReader with the passed underlying
// Reader and the program to execute as filter.
func NewExternalReader(r io.Reader, program string, args ...string) (*ExternalReader, error) {
	cmd := exec.Command(program, args...)
	cmd.Stdin = r
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		_ = output.Close()
		return nil, err
	}
	return &ExternalReader{cmd, output}, nil
}

func (er *ExternalReader) Read(p []byte) (int, error) {
	return er.output.Read(p)
}

// Close properly finish the execution of an ExternalReader.
func (er *ExternalReader) Close() error {
	return er.cmd.Wait()
}
