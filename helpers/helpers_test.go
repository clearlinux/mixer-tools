package helpers

import (
	"fmt"
	"strings"
	"testing"
)

func TestRunCommandOutputSuccess(t *testing.T) {
	const msg = "Hello, world!"
	const fail = "This is not working!"
	// Prints both in stdout and stderr.
	out, err := RunCommandOutput("bash", "-c", fmt.Sprintf("echo -n %q; echo -n %q >&2", msg, fail))
	if err != nil {
		t.Fatal(err)
	}
	// Output contains only stdout.
	if out.String() != msg {
		t.Fatalf("unexpected output %q instead of %q", out.String(), msg)
	}
}

func TestRunCommandOutputFailure(t *testing.T) {
	// Prints both in stdout and stderr. Calling false forces failure.
	_, err := RunCommandOutput("bash", "-c", "export OK=OK; export FAIL=FAIL; echo -n $OK$OK; echo -n $FAIL$FAIL >&2; false")
	if err == nil {
		t.Fatal("unexpected success when running command")
	}
	// Error should contain both stdout and stderr. Note that the strings we are
	// looking at are not part of the command, to avoid matching those.
	if !strings.Contains(err.Error(), "OKOK") {
		t.Errorf("error doesn't contain the stdout of the program")
	}
	if !strings.Contains(err.Error(), "FAILFAIL") {
		t.Errorf("error doesn't contain the stderr of the program")
	}
	if t.Failed() {
		fmt.Println(err)
	}
}
