package swupd

import (
	"fmt"
	"os"
	"syscall"
	"testing"
)

func resetHash() {
	Hashes = []*string{&AllZeroHash}
	invHash = map[string]hashval{AllZeroHash: 0}
}

func TestInternHash(t *testing.T) {
	// reset Hashes so we get the expected indices
	resetHash()
	testCases := []struct {
		hash     string
		expected hashval
	}{
		{"9bcc1718757db298fb656ae6e2ee143dde746f49fbf6805db7683cb574c36728", 1},
		{"33ccead640727d66c62be03e089a3ca3f4ef7c374a3eeab79764f9509075b0d8", 2},
		{"33ccead640727d66c62be03e089a3ca3f4ef7c374a3eeab79764f9509075b0d8", 2},
		{"b26f85ffaf3595ecd9a8b1e0c894f1b9e6e3ed0e8c3f28bcde3d66e63bfedd4d", 3},
		{"a49e68b3e2230855586e9ffd1b2962a2282411a488b80e3bd65851f068394c0a", 4},
		{"a49e68b3e2230855586e9ffd1b2962a2282411a488b80e3bd65851f068394c0a", 4},
		{"a49e68b3e2230855586e9ffd1b2962a2282411a488b80e3bd65851f068394c0a", 4},
		{"0000000000000000000000000000000000000000000000000000000000000000", 0},
		{"864f78102661c05b61cafcb59785349fd2fb7a956ec00a77198fe5bc2432de76", 5},
	}

	for _, tc := range testCases {
		t.Run("validHash", func(t *testing.T) {
			if idx := internHash(tc.hash); idx != tc.expected {
				t.Errorf("interned hash index %v did not match expected %v",
					idx, tc.expected)
			}
		})
	}
}

func TestHashPrinting(t *testing.T) {
	s := "0000000000000000000000000000000000000000000000000000000000000001"
	v := internHash(s)
	sout := fmt.Sprintf("%v", v)
	if sout != s {
		t.Errorf("in and out of hashtable do not match\n\t%v\n\t%v", sout, s)
	}
}

func TestHashPrinting2(t *testing.T) {
	s := []byte("0000000000000000000000000000000000000000000000000000000000000001")
	v := internHash(string(s))
	s[0] = '1'
	sout := fmt.Sprintf("%v", v)
	if sout == string(s) {
		t.Errorf("in and out of hashtable do not match\n\t%v\n\t%v", sout, s)
	}
}

func TestHashEqual(t *testing.T) {
	someHashes := []struct {
		hash string
		val  hashval
	}{
		{"3a60eb03c76ce17f1d08e0b5844c0455f6136c9b4bd4dd54c98cad2783354635", 0},
		{"b4b9333757d79e1e766dbb5db3160108e907e110bd19cba4d1d4230b299d0eb", 0},
		{"99aff80fc35d08b36c69ed0340ea80805f0c1b81ba7c734db6434b29a24c8391", 0},
	}
	for i, tc := range someHashes {
		// subtle point here, need to use the array index, rather than
		// setting tc.val as tc is a copy of the entry, not a pointer to it
		// See https://golang.org/ref/spec#RangeClause
		someHashes[i].val = internHash(tc.hash)
	}
	// do n^2 compares
	for i := range someHashes {
		tc := &someHashes[i]
		for j := range someHashes {
			tc2 := &someHashes[j]
			if HashEquals(tc.val, tc2.val) != (i == j) {
				t.Errorf("HashEquals returns incorrect result %d %d %v %v",
					i, j, tc.hash, tc2.hash)
			}
		}
	}
}

// Tip, to generate random hash values use this.
// hexdump -n32 -e '32 "%02x" "\n"' /dev/random

const (
	Dir  = 0040000
	Reg  = 0100000
	Link = 0120000
)

// TestGenHash checks that the internal data hashing is correct
func TestGenHash(t *testing.T) {
	testCases := []struct {
		info   syscall.Stat_t
		data   []byte
		result string
	}{
		{syscall.Stat_t{Mode: (Dir + 0755)},
			[]byte("DIRECTORY"), directoryhash},
		{syscall.Stat_t{Mode: (Dir + 01777)},
			[]byte("DIRECTORY"),
			"d93a5e9129361e28b9e244fe422234e3a1794b001a082aeb78e16fd881673a2b"},
		{syscall.Stat_t{Mode: Reg + 0644, Uid: 1000, Gid: 1000},
			[]byte(""),
			"b85f1dc2c2317a20f47a36d3257313b131124ffa6d4f19bb060d43014fd386b0"},
		{syscall.Stat_t{Mode: Reg + 0644, Uid: 1000, Gid: 201},
			[]byte(""),
			"0a3978d8b6ea47b779a2dfb5d6a7f57c93d28e131870bcd187470da3678d1298"},
		{syscall.Stat_t{Mode: Reg + 0644, Uid: 1000, Gid: 201, Size: 6},
			[]byte("hello\n"),
			"53b40563c1162a14d9ce0233a6b346cd0a4cbce54c40affbdf0fc286fd3bfe7b"},
	}

	for _, tc := range testCases {
		r := genHash(tc.info, tc.data)
		if r != tc.result {
			t.Errorf("Unexpected result %s for\n%v\n", r, tc)
		}
	}
}

const (
	// hash for a rwxr-xr-x root owned directory
	directoryhash = "6c27df6efcd6fc401ff1bc67c970b83eef115f6473db4fb9d57e5de317eba96e"
	missinghash   = "0000000000000000000000000000000000000000000000000000000000000000"
)

func TestHashcalc(t *testing.T) {
	testCases := []struct {
		filename string
		result   string
	}{
		{"/", directoryhash},
		{"/does not exist", missinghash},
		{"/usr", directoryhash},
		{"/dev/null", missinghash},
	}

	for _, tc := range testCases {
		r, _ := Hashcalc(tc.filename)
		h := internHash(tc.result)
		if r != h {
			t.Errorf("Expected %s for hash of %s, got %s", tc.result, tc.filename, r)
		}
	}
}

func TestHashcalcRealFile(t *testing.T) {
	// test cases for files which may or may not exist
	testCases := []struct {
		filename string
		result   string
	}{
		{"/etc/protocols", "cfc5cc64ea94da67920936286d5f37152a46bbf908d383fc5d50d0ecde2ddc3e"},
		{"/usr/share/defaults/etc/protocols", "cfc5cc64ea94da67920936286d5f37152a46bbf908d383fc5d50d0ecde2ddc3e"},
		{"/usr/share/doc/systemd/LICENSE.GPL2", "d9d34a1e44f3684286dd07c6a9e1747a1307e4421cd5d70f71a548c446a9ca54"},
	}
	for _, tc := range testCases {
		if _, err := os.Stat(tc.filename); os.IsNotExist(err) {
			continue
		}
		r, _ := Hashcalc(tc.filename)
		h := internHash(tc.result)
		if r != h {
			t.Errorf("Expected %s for hash of %s, got %s", tc.result, tc.filename, r)
		}
	}

}
