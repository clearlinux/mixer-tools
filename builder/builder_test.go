package builder

import (
	"io/ioutil"
	"os"
	"testing"
)

func mustExist(t *testing.T, name string) {
	t.Helper()
	_, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
}

func cleanup(path string) {
	_ = os.RemoveAll(path)
}

func TestNewDNFConfOnInit(t *testing.T) {
	testDir, err := ioutil.TempDir("", "dnf-testdir-")
	if err != nil {
		t.Fatalf("couldn't create temporary directory to write test cases: %s", err)
	}
	defer cleanup(testDir)
	conf := testDir + "/builder.conf"

	b := New()
	b.Config.LoadDefaultsForPath(testDir)

	if err = b.Config.InitConfigPath(conf); err != nil {
		t.Errorf("Failed to init default config: %s", err)
	}

	if err = b.Config.Save(); err != nil {
		t.Errorf("Failed to create default config: %s", err)
	}

	if err = b.Config.Load(conf); err != nil {
		t.Errorf("Failed to load default builder.conf: %s", err)
	}
	Offline = true

	err = b.InitMix("10", "10", false, false, true, "https://example.com", false)
	if err != nil {
		t.Errorf("Failed to initialize mix: %s", err)
	}

	mustExist(t, testDir+"/.yum-mix.conf")
}
