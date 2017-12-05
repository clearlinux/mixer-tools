package swupd

// Hashes is a global map of indices to hashes
var Hashes = []*string{}

// internHash adds only new hashes to the Hashes slice and returns the index at
// which they are located
func internHash(hash string) int {
	for idx, val := range Hashes {
		if *val == hash {
			return idx
		}
	}

	Hashes = append(Hashes, &hash)
	return len(Hashes) - 1
}
