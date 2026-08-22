package directplay

import "testing"

func TestDirectPlayLockKeyIsStableAndContentScoped(t *testing.T) {
	baseline := directPlayLockKey("playback-1", directPlaySourceSHA1, 1024)
	if baseline != directPlayLockKey("playback-1", directPlaySourceSHA1, 1024) {
		t.Fatal("directPlayLockKey() is not stable")
	}
	variants := []int32{
		directPlayLockKey("playback-2", directPlaySourceSHA1, 1024),
		directPlayLockKey("playback-1", "1123456789ABCDEF0123456789ABCDEF01234567", 1024),
		directPlayLockKey("playback-1", directPlaySourceSHA1, 2048),
	}
	for _, variant := range variants {
		if variant == baseline {
			t.Fatalf("directPlayLockKey() collision in fixed scope test: %d", baseline)
		}
	}
}
