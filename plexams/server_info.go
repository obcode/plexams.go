package plexams

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

const githubRepoURL = "https://github.com/obcode/plexams.go"

// ServerInfo reports the running server's build version and the database it is
// connected to, e.g. for the GUI footer.
//
// Which semester is active is NOT here: it is the `semester` query, and having it
// in two places is what let the GUI footer disagree with the switcher.
func (p *Plexams) ServerInfo(ctx context.Context) (*model.ServerInfo, error) {
	version := viper.GetString("Version")

	dbHost := ""
	if p.dbClient != nil {
		dbHost = p.dbClient.DBHost()
	}

	return &model.ServerInfo{
		Version:    version,
		Commit:     viper.GetString("Commit"),
		Date:       viper.GetString("Date"),
		BuiltBy:    viper.GetString("BuiltBy"),
		ReleaseURL: releaseURL(version),
		DbHost:     dbHost,
	}, nil
}

// releaseURL builds the GitHub release link for a build version. goreleaser sets
// main.version without a leading "v" (e.g. "1.99.0"); a `go install ...@vX.Y.Z`
// build keeps it (e.g. "v1.99.0"). Both map to the git tag "v1.99.0". Non-release
// versions ("dev", "none", "unknown") get no link.
func releaseURL(version string) *string {
	v := strings.TrimPrefix(version, "v")
	if v == "" || v[0] < '0' || v[0] > '9' {
		return nil
	}
	u := githubRepoURL + "/releases/tag/v" + v
	return &u
}
