package minutebucket

import (
	"testing"
)

func TestBucketMinuteFormat(t *testing.T) {
	got := bucketMinute(testNow())
	if got != "202608251200" {
		t.Errorf("bucketMinute = %q", got)
	}
}
