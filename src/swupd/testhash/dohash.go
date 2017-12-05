// Hack to generate swupd hashes, without xattrs.
//
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"

	"golang.org/x/sys/unix"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s name1 name2 ...\n", os.Args[0])
		return
	}
	if len(os.Args) == 2 {
		fmt.Println(Hashcalc(os.Args[1]))
	} else {
		for _, filename := range os.Args[1:] {
			fmt.Printf("%s\t%s\n", filename,
				Hashcalc(filename))
		}
	}
}

func Hashcalc(filename string) string {
	key, err := hmac_compute_key(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error stating file '%s' %v\n", filename, err)
		return ""
	}
	// Only handle files for now..
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Read error for '%s' %v\n", filename, err)
		return ""
	}
	result := hmac_sha256_for_data(key, data)
	return string(result[:])
}

// hmac_sha256_for_data returns an ascii string of hex digits
func hmac_sha256_for_data(key []byte, data []byte) []byte {
	var result [64]byte

	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	hex.Encode(result[:], mac.Sum(nil))
	return result[:]
}

// This is what I want to have for the key
// type updatestat struct {
// 	st_mode uint64
// 	st_uid  uint64
// 	st_gid  uint64
// 	st_rdev uint64
// 	st_size uint64
// }

// set fills in a buffer with an int in little endian order
func set(out []byte, in int64) {
	for i := range out {
		out[i] = byte(in & 0xff)
		in >>= 8
	}
}

// return what should be an ascii string as an array of byte
func hmac_compute_key(filename string) ([]byte, error) {
	// Create the key
	updatestat := [40]byte{}
	var info unix.Stat_t
	if err := unix.Stat(filename, &info); err != nil {
		return nil, err
	}
	set(updatestat[24:32], 0)
	set(updatestat[0:8], int64(info.Mode))
	set(updatestat[8:16], int64(info.Uid))
	set(updatestat[16:24], int64(info.Gid))
	// 24:32 is rdev, but this is always zero
	set(updatestat[32:40], int64(info.Size))
	// fmt.Printf("key is %v\n", updatestat)
	key := hmac_sha256_for_data(updatestat[:], nil)
	return key, nil
}
