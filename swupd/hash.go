// Copyright 2017 Intel Corporation
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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"sync"
	"syscall"
)

// Hashval is the integer index of the interned hash
type Hashval int

// AllZeroHash is the string representation of a zero value hash
var AllZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Hashes is a global map of indices to hashes
var Hashes = []*string{&AllZeroHash}
var invHash = map[string]Hashval{AllZeroHash: 0}
var rwMutex sync.RWMutex

// internHash adds only new hashes to the Hashes slice and returns the index at
// which they are located
func internHash(hash string) Hashval {
	// Many reader locks can be acquired at the same time
	rwMutex.RLock()
	if key, ok := invHash[hash]; ok {
		rwMutex.RUnlock()
		return key
	}
	rwMutex.RUnlock()
	// We need to grab a full lock now and check that it still does not exist
	// because by the time we grab a lock and append, another thread could have
	// already added the same hash since many files can overlap. The lock says
	// no more reader locks can be acquired, and waits until all are released
	// before taking the lock continuing forward with the check and appending.
	rwMutex.Lock()
	if key, ok := invHash[hash]; ok {
		rwMutex.Unlock()
		return key
	}
	Hashes = append(Hashes, &hash)
	key := Hashval(len(Hashes) - 1)
	invHash[hash] = key
	rwMutex.Unlock()
	return key
}

func (h Hashval) String() string {
	return *Hashes[int(h)]
}

// HashEquals trivial equality function for Hashval
func HashEquals(h1 Hashval, h2 Hashval) bool {
	return h1 == h2
}

// Hashcalc returns the swupd hash for the given file
func Hashcalc(filename string) (Hashval, error) {
	r, err := GetHashForFile(filename)
	if err != nil {
		return 0, err
	}
	return internHash(r), nil
}

// set fills in a buffer with an int in little endian order.
func set(out []byte, in int64) {
	for i := range out {
		out[i] = byte(in & 0xff)
		in >>= 8
	}
}

// HashFileInfo contains the metadata of a file that is included as
// part of the swupd hash.
type HashFileInfo struct {
	Mode     uint32
	UID      uint32
	GID      uint32
	Size     int64
	Linkname string
}

// Hash is used to calculate the swupd Hash of a file. Create one with
// NewHash, use Write method to fill the contents (for regular files),
// and use Sum to get the final hash.
type Hash struct {
	hmac hash.Hash
}

// NewHash creates a struct that can be used to calculate the "swupd
// Hash" of a given file. For historical reasons, the hash is
// constructed as
//
//     stat     := file metadata
//     contents := file contents
//     HMAC(key, data)
//
//     swupd hash = HMAC(HMAC(stat, nil), contents)
//
// The data for the inner HMAC was used for file xattrs, but is not
// used at the moment.
func NewHash(info *HashFileInfo) (*Hash, error) {
	var data []byte
	switch info.Mode & syscall.S_IFMT {
	case syscall.S_IFREG:
	case syscall.S_IFDIR:
		info.Size = 0
		data = []byte("DIRECTORY")
	case syscall.S_IFLNK:
		info.Mode = 0
		data = []byte(info.Linkname)
		info.Size = int64(len(data))
	default:
		return nil, fmt.Errorf("invalid")
	}

	// The HMAC key for the data will itself be generated using
	// HMAC. The "key for the key" must have the bytes with the
	// same layout as the C struct
	//
	// struct update_stat {
	//     uint64_t st_mode;
	//     uint64_t st_uid;
	//     uint64_t st_gid;
	//     uint64_t st_rdev;
	//     uint64_t st_size;
	// };
	stat := [40]byte{}
	set(stat[0:8], int64(info.Mode))
	set(stat[8:16], int64(info.UID))
	set(stat[16:24], int64(info.GID))
	// 24:32 is rdev, but this is always zero.
	set(stat[32:40], info.Size)

	var key [64]byte
	mac := hmac.New(sha256.New, stat[:])
	_, err := mac.Write(nil)
	if err != nil {
		return nil, err
	}
	hex.Encode(key[:], mac.Sum(nil))

	// With the key in hand, create the HMAC struct that will be
	// used to write data to.
	h := &Hash{
		hmac: hmac.New(sha256.New, key[:]),
	}

	// Pre-write data we know so that directories and symbolic
	// links don't need further data from the caller.
	if data != nil {
		_, err = h.hmac.Write(data)
		if err != nil {
			return nil, err
		}
	}

	return h, nil
}

// Write more data to the hash being calculated.
func (h *Hash) Write(p []byte) (n int, err error) {
	return h.hmac.Write(p)
}

// Sum returns the string containing the Hash calculated using swupd
// algorithm of the data previously written.
func (h *Hash) Sum() string {
	var result [64]byte
	hex.Encode(result[:], h.hmac.Sum(nil))
	return string(result[:])
}

// GetHashForFile calculate the swupd hash for a file in the disk.
func GetHashForFile(filename string) (string, error) {
	var info syscall.Stat_t
	var err error
	if err = syscall.Lstat(filename, &info); err != nil {
		return "", fmt.Errorf("error statting file '%s' %v", filename, err)
	}

	hashInfo := &HashFileInfo{
		Mode: info.Mode,
		UID:  info.Uid,
		GID:  info.Gid,
		Size: info.Size,
	}

	if info.Mode&syscall.S_IFMT == syscall.S_IFLNK {
		var link string
		link, err = os.Readlink(filename)
		if err != nil {
			return "", err
		}
		hashInfo.Linkname = link
	}

	h, err := NewHash(hashInfo)
	if err != nil {
		return "", fmt.Errorf("error creating hash for file %s: %s", filename, err)
	}

	if info.Mode&syscall.S_IFMT == syscall.S_IFREG {
		f, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("read error for file %s: %s", filename, err)
		}
		_, err = io.Copy(h, f)
		_ = f.Close()
		if err != nil {
			return "", fmt.Errorf("error hashing file %s: %s", filename, err)
		}
	}

	return h.Sum(), nil
}

// GetHashForBytes calculate the hash for data already in memory and the
// associated metadata.
func GetHashForBytes(info *HashFileInfo, data []byte) (string, error) {
	h, err := NewHash(info)
	if err != nil {
		return "", err
	}
	if data != nil {
		_, err = h.Write(data)
		if err != nil {
			return "", err
		}
	}
	return h.Sum(), nil
}
