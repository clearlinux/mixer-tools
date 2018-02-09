package swupd

import (
	"os"
	"syscall"
	"testing"
)

// Need to set up a type to hold the FileInfo
// as we are not holding them on disk
type mockinfo struct {
	os.FileInfo // Embedded but only used to get things like Name() method
	size        int64
}

// Here is the method to override the FileInfo.Size() for mockinfo
func (m mockinfo) Size() int64 {
	return m.size
}

// Constructor for os.FileInfo using mockinfo
func sizer(s int64) os.FileInfo {
	return mockinfo{size: s}
}

// generateTestArray creates some File structures with shortnames and
// returns a map of them.
func generateTestArray(t *testing.T) map[string]File {
	// Set up an array of File with short names
	const (
		_rwx = syscall.S_IFREG + 0644
	)
	files := []struct {
		n string // short name for the tests
		f File
		s int64  // size
		m uint32 // mode
		k string // stuff to make content unique
	}{
		// Two files, same contents
		{n: "I1", f: File{Name: "/lib/python3.6/libpython.3.6.0.so", Type: TypeFile}, s: 400, m: _rwx, k: "file1"},
		{n: "I2", f: File{Name: "/lib/python3.7/libpython.3.7.0.so", Type: TypeFile}, s: 400, m: _rwx, k: "file1"},
		// Two short files, different contents
		{n: "S1", f: File{Name: "/lib/python3.6/tiny", Type: TypeFile}, s: 180, m: _rwx, k: "123"},
		{n: "S2", f: File{Name: "/lib/python3.7/tiny", Type: TypeFile}, s: 180, m: _rwx, k: "456"},
		// long files, different contents, same length
		{n: "L1", f: File{Name: "/lib/python3.6/big", Type: TypeFile}, s: 580, m: _rwx, k: "123"},
		{n: "L2", f: File{Name: "/lib/python3.7/big", Type: TypeFile}, s: 580, m: _rwx, k: "456"},
		{n: "L3", f: File{Name: "/lib/python2.6/big", Type: TypeFile}, s: 580, m: _rwx, k: "123"},
		{n: "L4", f: File{Name: "/lib/python2.7/big", Type: TypeFile}, s: 580, m: _rwx, k: "456"},
	}

	m := &Manifest{}
	m.Header.Version = 20

	sn := make(map[string]File)
	for i := range files {
		f := &files[i]
		hi := HashFileInfo{Mode: f.m, Size: f.s}
		contents := make([]byte, f.s)           // Load of NUL of correct length
		copy(contents[0:len(f.k)], []byte(f.k)) // set the first n bytes
		hv, err := GetHashForBytes(&hi, contents)
		if err != nil {
			t.Fatalf("GetHashForBytes for %v returned %v", f, err)
		}
		f.f.Hash = internHash(hv)
		f.f.Info = sizer(f.s) // A FileInfo that returns our desired size
		sn[f.n] = f.f
	}
	return sn
}

// filelist takes a list of wanted shortnames, looks them up in sn, and returns
// a list of pointers to copies of the Files. Because they are copies they can
// be altered without side effects on other tests
func filelist(t *testing.T, sn map[string]File, wanted []string) []*File {
	t.Helper()
	r := make([]*File, len(wanted))
	for i := range wanted {
		element, ok := sn[wanted[i]]
		if !ok {
			t.Fatalf("Shortname %v not found in array", wanted[i])
		}
		r[i] = &element
	}
	return r
}

// markdelete sets all the files in the slice to deleted
func markdelete(a []*File) []*File {
	for i := range a {
		a[i].Status = StatusDeleted
	}
	return a
}

// checkLinked tests forward and backward pairing of DeltaPeers
func checkLinked(t *testing.T, a, b *File, description string) bool {
	t.Helper()
	// a is never nil, it is the entry in the "from" array
	if a.DeltaPeer != b {
		t.Errorf("%s Incorrect Linkage forward %v -> %v, got %v", description, a, b, a.DeltaPeer)
		return false
	}
	// b is nil in the case that it is not supposed to have a DeltaPeer
	if b != nil && b.DeltaPeer != a {
		t.Errorf("%s Incorrect Linkage reverse %v -> %v, got %v", description, b, a, b.DeltaPeer)
		return false
	}
	return true
}

func TestRename(t *testing.T) {
	sn := generateTestArray(t)
	tests := []struct {
		description string
		remove      []string // shortnames of File to remove
		add         []string // shortnames of File to add
		partner     []int    // Indexes of add that should match remove
	}{
		{description: "Rename of file, same contents",
			remove: []string{"I1", "I2"}, add: []string{"I2"}, partner: []int{0, -1}},
		{description: "Rename of short file, different contents, shouldn't link",
			remove: []string{"S1"}, add: []string{"S2"}, partner: []int{-1}},
		{description: "Rename of long file, different contents, should link",
			remove: []string{"S1", "L1"}, add: []string{"L2"}, partner: []int{-1, 0}},
		{description: "Rename of long files pairwise, should link",
			remove: []string{"S1", "L1", "L3"}, add: []string{"L2", "L4"},
			partner: []int{-1, 0, 1}},
		{description: "Rename of long files pairwise reversed should link",
			remove: []string{"S1", "L3", "L1"}, add: []string{"L2", "L4"},
			partner: []int{-1, 1, 0}},
		{description: "Rename files pairwise longer reversed should link",
			remove: []string{"S1", "L3", "S2", "L1"}, add: []string{"L4", "L2"},
			partner: []int{-1, 0, -1, 1}},
	}
	for _, tc := range tests {
		add := filelist(t, sn, tc.add)
		remove := markdelete(filelist(t, sn, tc.remove))
		renameDetection(&Manifest{}, add, remove, config{})
		if len(tc.partner) != len(tc.remove) {
			t.Fatalf("Invalid testcase %v, wrong partner length", tc)
		}
		for r, tr := range tc.partner {
			if tr == -1 { // No partner expected
				checkLinked(t, remove[r], nil, tc.description)
				continue
			}
			// Partner expected, check both ways
			checkLinked(t, add[tr], remove[r], tc.description)
		}
	}
}
