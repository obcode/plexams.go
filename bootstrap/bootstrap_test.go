package bootstrap

import (
	"strings"
	"testing"
)

// The guard that matters: selfPath must keep naming this package's own file, or
// callerPrefix silently becomes "" and every caller field goes back to being an
// absolute build-machine path -- which still logs fine and still fingerprints,
// just wrongly and invisibly.
func TestCallerPrefixWasDetermined(t *testing.T) {
	if callerPrefix == "" {
		t.Fatalf("callerPrefix is empty: %q no longer names this file", selfPath)
	}
	if !strings.HasSuffix(callerPrefix, "/") {
		t.Errorf("callerPrefix = %q, want it to end at a directory boundary", callerPrefix)
	}
}

func TestRepoRelativeCaller(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "a file in the repository loses the build path",
			file: callerPrefix + "graph/nta.resolvers.go",
			want: "graph/nta.resolvers.go:44",
		},
		{
			// A frame from a dependency or the standard library does not start
			// with the prefix. Better a long path than a truncated, wrong one.
			name: "a path outside the repository is left alone",
			file: "/usr/local/go/src/net/http/server.go",
			want: "/usr/local/go/src/net/http/server.go:44",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repoRelativeCaller(0, tt.file, 44); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
