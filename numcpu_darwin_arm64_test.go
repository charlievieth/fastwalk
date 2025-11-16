package fastwalk

import "testing"

func TestDarwinNumPerfCores(t *testing.T) {
	n := darwinNumPerfCores()
	// Test that n is reasonable
	if !(0 < n && n < 4096) {
		t.Fatalf("expected a value between 0..4096 got: %d", n)
	}
}
