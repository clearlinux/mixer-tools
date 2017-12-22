package swupd

// Pack is an object containing delta files and full files for downloads
type Pack struct {
	Bundle        string
	FromVersion   uint32
	ToVersion     uint32
	FullFileCount uint32
	Manifest      *Manifest
}
