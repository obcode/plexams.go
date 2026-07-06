package plexams

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

const githubRepoURL = "https://github.com/obcode/plexams.go"

// ServerInfo reports the running server's build version and the MongoDB it is
// connected to, e.g. for the GUI footer.
func (p *Plexams) ServerInfo(ctx context.Context) (*model.ServerInfo, error) {
	version := viper.GetString("Version")

	mongoHost := ""
	mongoDatabase := ""
	if p.dbClient != nil {
		mongoHost = p.dbClient.MongoHost()
		mongoDatabase = p.dbClient.DatabaseName()
	}

	return &model.ServerInfo{
		Version:       version,
		Commit:        viper.GetString("Commit"),
		Date:          viper.GetString("Date"),
		BuiltBy:       viper.GetString("BuiltBy"),
		ReleaseURL:    releaseURL(version),
		MongoHost:     mongoHost,
		MongoDatabase: mongoDatabase,
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
