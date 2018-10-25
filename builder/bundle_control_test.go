package builder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/clearlinux/mixer-tools/config"
)

func TestGetUpstreamBundlesVerDir(t *testing.T) {
	testCases := []struct {
		ver string
		exp string
	}{
		{"0", "clr-bundles-0"},
		{"000001", "clr-bundles-000001"},
		{"non-numeric", "clr-bundles-non-numeric"},
		{"", "clr-bundles-"},
		{"25660", "clr-bundles-25660"},
	}

	for _, tc := range testCases {
		t.Run(tc.ver, func(t *testing.T) {
			actual := getUpstreamBundlesVerDir(tc.ver)
			if actual != tc.exp {
				t.Errorf("expected %s on input %s but got %s", tc.exp, tc.ver, actual)
			}
		})
	}
}

func TestGetUpstreamBundlesPath(t *testing.T) {
	testCases := []struct {
		ver string
		exp string
	}{
		{"0", "test/upstream-bundles/clr-bundles-0/bundles"},
		{"000001", "test/upstream-bundles/clr-bundles-000001/bundles"},
		{"non-numeric", "test/upstream-bundles/clr-bundles-non-numeric/bundles"},
		{"", "test/upstream-bundles/clr-bundles-/bundles"},
		{"25660", "test/upstream-bundles/clr-bundles-25660/bundles"},
	}
	b := New()
	b.Config = config.MixConfig{}
	b.Config.LoadDefaultsForPath("test")
	b.Config.Builder.VersionPath = "test"
	for _, tc := range testCases {
		t.Run(tc.ver, func(t *testing.T) {
			actual := b.getUpstreamBundlesPath(tc.ver)
			if actual != tc.exp {
				t.Errorf("expected %s on input %s but got %s", tc.exp, tc.ver, actual)
			}
		})
	}
}

func TestGetLocalPackagesPath(t *testing.T) {
	b := New()
	// internally this function requires some sub-objects of builder to
	// be set (non-nil)
	b.Config = config.MixConfig{}
	b.Config.LoadDefaultsForPath("test")
	b.Config.Builder.VersionPath = "test"
	b.LocalPackagesFile = "local-packages"
	actual := b.getLocalPackagesPath()
	if actual != "test/local-packages" {
		t.Errorf("expected test/local-packages but got %s", actual)
	}
}

func TestGetUpstreamPackagesPath(t *testing.T) {
	testCases := []struct {
		ver string
		exp string
	}{
		{"0", "upstream-bundles/clr-bundles-0/packages"},
		{"000001", "upstream-bundles/clr-bundles-000001/packages"},
		{"non-numeric", "upstream-bundles/clr-bundles-non-numeric/packages"},
		{"", "upstream-bundles/clr-bundles-/packages"},
		{"25660", "upstream-bundles/clr-bundles-25660/packages"},
	}

	b := New()
	for _, tc := range testCases {
		t.Run(tc.ver, func(t *testing.T) {
			b.UpstreamVer = tc.ver
			actual := b.getUpstreamPackagesPath()
			if actual != tc.exp {
				t.Errorf("expected %s but got %s", tc.exp, actual)
			}
		})
	}
}

func TestGetUpstreamBundles(t *testing.T) {
	b := New()
	Offline = true
	if b.getUpstreamBundles("", false) != nil {
		t.Error("returned error when in offline mode")
	}
	// TODO: mock network
}

func writeToTmpFile(t *testing.T, dir, data string) string {
	t.Helper()
	var f *os.File
	var err error
	if f, err = ioutil.TempFile(dir, "packages"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()
	fn := f.Name()

	if _, err = f.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	return fn
}

func TestSetPackagesList(t *testing.T) {
	d, err := ioutil.TempDir("", "setpackageslist")
	if err != nil {
		t.Fatalf("unable to create test directory")
	}
	defer func() {
		_ = os.RemoveAll(d)
	}()

	testCases := []struct {
		tn  string
		fn  string
		exp map[string]bool
	}{
		{
			"multiple",
			writeToTmpFile(t, d, "packageA\npackageB"),
			map[string]bool{"packageA": true, "packageB": true},
		},
		{
			"empty",
			writeToTmpFile(t, d, ""),
			nil,
		},
		{
			"one",
			writeToTmpFile(t, d, "packageA"),
			map[string]bool{"packageA": true},
		},
		{
			"file does not exist",
			filepath.Join(d, "foo"),
			nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.tn, func(t *testing.T) {
			s := make(map[string]bool)
			if err := setPackagesList(&s, tc.fn); err != nil {
				t.Fatal(err)
			}
			if len(s) == 0 && len(tc.exp) == 0 {
				// reflect can't compare empty maps
				return
			}
			if !reflect.DeepEqual(s, tc.exp) {
				t.Errorf("expected %v but got %v", tc.exp, s)
			}
		})
	}

	// test that populated map is left unchanged
	s := map[string]bool{"packageA": true}
	e := map[string]bool{"packageA": true}
	if err := setPackagesList(&s, writeToTmpFile(t, d, "packageD\npackageE")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, e) {
		t.Errorf("expected %v but got %v", e, s)
	}
}

func mustCreateTempBundleDirs(t *testing.T, b *Builder, d string) {
	t.Helper()

	var err error
	if err = os.MkdirAll(b.Config.Mixer.LocalBundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err = os.MkdirAll(filepath.Dir(b.getLocalPackagesPath()), 0755); err != nil {
		t.Fatal(err)
	}

	if err = os.MkdirAll(b.getUpstreamBundlesPath(b.UpstreamVer), 0755); err != nil {
		t.Fatal(err)
	}

	if err = os.MkdirAll(filepath.Dir(b.getUpstreamPackagesPath()), 0755); err != nil {
		t.Fatal(err)
	}
}

func mustAddBundleToPath(t *testing.T, path, name string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(path, name), os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func mustAddBundleToFile(t *testing.T, path, name string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()
	if _, err = f.WriteString(name); err != nil {
		t.Fatal(err)
	}
}

func mustAddBundleToLocal(t *testing.T, b *Builder, name string) {
	t.Helper()
	mustAddBundleToPath(t, b.Config.Mixer.LocalBundleDir, name)
}

func mustAddBundleToLocalPackages(t *testing.T, b *Builder, name string) {
	t.Helper()
	mustAddBundleToFile(t, b.getLocalPackagesPath(), name)
}

func mustAddBundleToUpstream(t *testing.T, b *Builder, name string) {
	t.Helper()
	mustAddBundleToPath(t, b.getUpstreamBundlesPath(b.UpstreamVer), name)
}

func mustAddBundleToUpstreamPackages(t *testing.T, b *Builder, name string) {
	t.Helper()
	mustAddBundleToFile(t, b.getUpstreamPackagesPath(), name)
}

func TestGetBundlePath(t *testing.T) {
	var d string
	var err error
	if d, err = ioutil.TempDir("", "getbundlepath"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		//_ = os.RemoveAll(d)
	}()
	b := New()
	b.UpstreamVer = "10"
	b.Config = config.MixConfig{}
	b.Config.LoadDefaultsForPath(d)
	mustCreateTempBundleDirs(t, b, d)

	testCases := []struct {
		name   string
		in     string
		exp    string
		helper func(*testing.T, *Builder, string)
	}{
		{
			"local_bundle",
			"testlocal",
			filepath.Join(d, "local-bundles/testlocal"),
			mustAddBundleToLocal,
		},
		{
			"local_packages",
			"testlocalpackages",
			filepath.Join(d, "local-packages"),
			mustAddBundleToLocalPackages,
		},
		{
			"upstream_bundles",
			"testupstreambundles",
			filepath.Join(d, "upstream-bundles/clr-bundles-10/bundles/testupstreambundles"),
			mustAddBundleToUpstream,
		},
		{
			"upstream_packages",
			"testupstreampackages",
			filepath.Join(d, "upstream-bundles/clr-bundles-10/packages"),
			mustAddBundleToUpstreamPackages,
		},
		{
			"does_not_exist",
			"testdoesnotexist",
			"",
			func(*testing.T, *Builder, string) { /*do not create */ },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.helper(t, b, tc.in)
			actual, err := b.getBundlePath(tc.in)
			if tc.exp != "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Errorf("expected error on %s input but did not get one", tc.in)
			}

			if actual != tc.exp {
				t.Errorf("expected %s but got %s", tc.exp, actual)
			}
		})
	}
}

func TestIsLocalBundle(t *testing.T) {
	validPrefix := "tlbprefix"
	testCases := []struct {
		in  string
		exp bool
	}{
		{filepath.Join(validPrefix, "test"), true},
		{filepath.Join(validPrefix), false}, // it is a valid prefix but there is no bundle
		{"invalid/bundle", false},
		{"", false},
		{"test/local-packages", true},
		{"testlocal-packages", false},
	}

	b := New()
	b.Config = config.MixConfig{}
	b.Config.LoadDefaultsForPath("test")
	b.Config.Mixer.LocalBundleDir = validPrefix
	b.LocalPackagesFile = "local-packages"
	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			actual := b.isLocalBundle(tc.in)
			if actual != tc.exp {
				t.Errorf("expected %v on %s input but got %v", tc.exp, tc.in, actual)
			}
		})
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func hasSameElements(a, b []string) bool {
	for _, e := range a {
		if !contains(b, e) {
			return false
		}
	}

	for _, e := range b {
		if !contains(a, e) {
			return false
		}
	}

	return true
}

func TestGetBundleSetKeys(t *testing.T) {
	testCases := []struct {
		name string
		set  bundleSet
		exp  []string
	}{
		{
			"base",
			bundleSet{"one": nil, "two": nil, "three": nil},
			[]string{"one", "two", "three"},
		},
		{
			"empty",
			bundleSet{},
			[]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getBundleSetKeys(tc.set)
			if !hasSameElements(actual, tc.exp) {
				t.Errorf("expected %v but got %v", tc.exp, actual)
			}
		})
	}
}

func TestGetBundleKeysSorted(t *testing.T) {
	testCases := []struct {
		name string
		set  bundleSet
		exp  []string
	}{
		{
			"base",
			bundleSet{"b": nil, "c": nil, "a": nil},
			[]string{"a", "b", "c"},
		},
		{
			"empty",
			bundleSet{},
			[]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getBundleSetKeysSorted(tc.set)
			if !reflect.DeepEqual(actual, tc.exp) {
				t.Errorf("expected %v but got %v", tc.exp, actual)
			}
		})
	}
}
