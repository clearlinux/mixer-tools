package swupd

import "testing"

func TestInternHash(t *testing.T) {
	// reset Hashes so we get the expected indices
	Hashes = []*string{}
	testCases := []struct {
		hash     string
		expected int
	}{
		{"9bcc1718757db298fb656ae6e2ee143dde746f49fbf6805db7683cb574c36728", 0},
		{"33ccead640727d66c62be03e089a3ca3f4ef7c374a3eeab79764f9509075b0d8", 1},
		{"33ccead640727d66c62be03e089a3ca3f4ef7c374a3eeab79764f9509075b0d8", 1},
		{"b26f85ffaf3595ecd9a8b1e0c894f1b9e6e3ed0e8c3f28bcde3d66e63bfedd4d", 2},
		{"a49e68b3e2230855586e9ffd1b2962a2282411a488b80e3bd65851f068394c0a", 3},
		{"a49e68b3e2230855586e9ffd1b2962a2282411a488b80e3bd65851f068394c0a", 3},
		{"a49e68b3e2230855586e9ffd1b2962a2282411a488b80e3bd65851f068394c0a", 3},
		{"864f78102661c05b61cafcb59785349fd2fb7a956ec00a77198fe5bc2432de76", 4},
	}

	for _, tc := range testCases {
		t.Run("validHash", func(t *testing.T) {
			if idx := internHash(tc.hash); idx != tc.expected {
				t.Errorf("interned hash index %v did not match expected %v", idx, tc.expected)
			}
		})
	}
}
