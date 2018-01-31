package swupd

import (
	"os"
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
