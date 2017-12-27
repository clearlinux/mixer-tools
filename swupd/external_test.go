package swupd

import (
	"bytes"
	"io/ioutil"
	"os/exec"
	"strings"
	"testing"
)

func TestExternalWriter(t *testing.T) {
	tr, err := exec.LookPath("tr")
	if err != nil {
		if err == exec.ErrNotFound {
			t.Skip("couldn't find tr program used for test")
		}
		t.Fatal(err)
	}

	var output bytes.Buffer
	w, err := newExternalWriter(&output, tr, "e", "a")
	if err != nil {
		t.Fatal(err)
	}

	input := "Hello, world!"
	expected := strings.Replace(input, "e", "a", -1)

	_, err = w.Write([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	err = w.Close()
	if err != nil {
		t.Fatal(err)
	}

	if output.String() != expected {
		t.Fatalf("got %q, but want %q", output.String(), expected)
	}
}

func TestExternalReader(t *testing.T) {
	tr, err := exec.LookPath("tr")
	if err != nil {
		if err == exec.ErrNotFound {
			t.Skip("couldn't find tr program used for test")
		}
		t.Fatal(err)
	}

	input := "Hello, world!"
	expected := strings.Replace(input, "e", "a", -1)

	r, err := newExternalReader(strings.NewReader(input), tr, "e", "a")
	if err != nil {
		t.Fatal(err)
	}

	output, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if string(output) != expected {
		t.Fatalf("got %q, but want %q", string(output), expected)
	}

	err = r.Close()
	if err != nil {
		t.Fatal(err)
	}
}
