package db

import (
	"os"
	"testing"
	"time"
)

// TestMain pins time.Local to Europe/Berlin for the whole db test binary, which
// is what main.go:22 does for the running server.
//
// Without it these tests assert a configuration that never exists in production
// and the result depends on the host: the DevContainer is Europe/Berlin and they
// pass, a GitHub runner is UTC and TestTimestamptzKeepsLocation and
// TestSeparatelyLoadedLocationIsADifferentMapKey fail. Both are about the
// location a time carries -- the thing more than ten map keys in plexams compare
// struct-wise -- so running them in the wrong zone tests nothing useful either
// way round.
//
// Setting it here rather than exporting TZ in the CI workflow keeps the suite
// correct on any machine, including a laptop that is not on German time. Both
// `package db` and `package db_test` files compile into this one binary, so this
// covers both.
func TestMain(m *testing.M) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic("cannot load Europe/Berlin: " + err.Error())
	}
	time.Local = loc

	os.Exit(m.Run())
}
