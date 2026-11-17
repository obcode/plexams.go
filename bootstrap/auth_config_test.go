package bootstrap

import "testing"

func TestCheckAuthConfig(t *testing.T) {
	cases := []struct {
		production, auth, wantErr bool
	}{
		{false, false, false}, // local development
		{false, true, false},
		{true, true, false}, // the server
		{true, false, true}, // every SSO login would be ADMIN
	}
	for _, c := range cases {
		err := checkAuthConfig(c.production, c.auth)
		if (err != nil) != c.wantErr {
			t.Errorf("production=%v auth=%v: err=%v, wantErr=%v", c.production, c.auth, err, c.wantErr)
		}
	}
}
