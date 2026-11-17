package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
)

// Administration of examiner-portal access: aliases, blocks, tokens and the identity
// report. ADMIN only; the resolvers check it.

// normalizeLogin is the form every login is stored and compared in.
func normalizeLogin(email string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" || strings.ContainsAny(e, " \t,;") || strings.Count(e, "@") != 1 ||
		strings.HasPrefix(e, "@") || strings.HasSuffix(e, "@") {
		return "", fmt.Errorf("not an e-mail address: %q", email)
	}
	return e, nil
}

func (p *Plexams) actorEmail(ctx context.Context) string {
	if id := p.actorID(ctx); id != nil {
		return *id
	}
	return "unknown"
}

// LoginAliases lists the aliases with the person's current name.
func (p *Plexams) LoginAliases(ctx context.Context) ([]*model.LoginAlias, error) {
	aliases, err := p.dbClient.LoginAliases(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(aliases))
	for _, a := range aliases {
		ids = append(ids, a.TeacherID)
	}
	names, err := p.dbClient.PortalTeachers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.LoginAlias, 0, len(aliases))
	for _, a := range aliases {
		name := ""
		if t, ok := names[a.TeacherID]; ok {
			name = t.Fullname
		}
		out = append(out, &model.LoginAlias{
			Email: a.Email, TeacherID: a.TeacherID, TeacherName: name,
			Note: a.Note, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
		})
	}
	return out, nil
}

// SetLoginAlias maps a login to a ZPA person. The person must be in a recent teacher
// mirror: an alias to a mistyped id would silently give a login nobody's exams.
func (p *Plexams) SetLoginAlias(ctx context.Context, email string, teacherID int, note *string) (*model.LoginAlias, error) {
	e, err := normalizeLogin(email)
	if err != nil {
		return nil, err
	}
	names, err := p.dbClient.PortalTeachers(ctx, []int{teacherID})
	if err != nil {
		return nil, err
	}
	teacher, ok := names[teacherID]
	if !ok || teacherID <= 0 {
		return nil, fmt.Errorf("no ZPA person with id %d", teacherID)
	}
	n := ""
	if note != nil {
		n = strings.TrimSpace(*note)
	}
	if err := p.dbClient.SetLoginAlias(ctx, e, teacherID, n, p.actorEmail(ctx)); err != nil {
		return nil, err
	}
	aliases, err := p.LoginAliases(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range aliases {
		if a.Email == e && a.TeacherID == teacherID {
			return a, nil
		}
	}
	return &model.LoginAlias{Email: e, TeacherID: teacherID, TeacherName: teacher.Fullname, Note: n}, nil
}

// RemoveLoginAlias deletes an alias.
func (p *Plexams) RemoveLoginAlias(ctx context.Context, email string, teacherID int) (bool, error) {
	e, err := normalizeLogin(email)
	if err != nil {
		return false, err
	}
	return p.dbClient.RemoveLoginAlias(ctx, e, teacherID)
}

// PortalLoginBlocks lists the blocked logins.
func (p *Plexams) PortalLoginBlocks(ctx context.Context) ([]*model.PortalLoginBlock, error) {
	blocks, err := p.dbClient.PortalLoginBlocks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PortalLoginBlock, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, &model.PortalLoginBlock{Email: b.Email, Reason: b.Reason, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt})
	}
	return out, nil
}

// SetPortalLoginBlock withdraws a login's portal access.
func (p *Plexams) SetPortalLoginBlock(ctx context.Context, email string, reason *string) (*model.PortalLoginBlock, error) {
	e, err := normalizeLogin(email)
	if err != nil {
		return nil, err
	}
	r := ""
	if reason != nil {
		r = strings.TrimSpace(*reason)
	}
	if err := p.dbClient.SetPortalLoginBlock(ctx, e, r, p.actorEmail(ctx)); err != nil {
		return nil, err
	}
	blocks, err := p.PortalLoginBlocks(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range blocks {
		if b.Email == e {
			return b, nil
		}
	}
	return nil, fmt.Errorf("block for %s not found after writing it", e)
}

// RemovePortalLoginBlock lifts a block.
func (p *Plexams) RemovePortalLoginBlock(ctx context.Context, email string) (bool, error) {
	e, err := normalizeLogin(email)
	if err != nil {
		return false, err
	}
	return p.dbClient.RemovePortalLoginBlock(ctx, e)
}

// PortalAccessTokens lists every token, without secrets.
func (p *Plexams) PortalAccessTokens(ctx context.Context) ([]*model.PortalAccessToken, error) {
	tokens, err := p.dbClient.AllPortalTokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PortalAccessToken, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, &model.PortalAccessToken{
			ID: t.TokenID, OwnerEmail: t.OwnerEmail, Description: t.Description, Scope: t.Scope,
			CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt,
		})
	}
	return out, nil
}

// RevokePortalAccessToken revokes any active token.
func (p *Plexams) RevokePortalAccessToken(ctx context.Context, id string) (bool, error) {
	return p.dbClient.RevokePortalToken(ctx, strings.TrimSpace(id))
}

// AccessIdentityReport lists what keeps people from reaching the portal, or would let
// a login be the wrong person, for the teachers of the active semester.
func (p *Plexams) AccessIdentityReport(ctx context.Context) ([]*model.IdentityProblem, error) {
	fromZPA := false
	teachers, err := p.GetTeachers(ctx, &fromZPA)
	if err != nil {
		return nil, err
	}
	exams, err := p.dbClient.GetZPAExams(ctx)
	if err != nil {
		return nil, err
	}
	aliases, err := p.dbClient.LoginAliases(ctx)
	if err != nil {
		return nil, err
	}
	return identityProblems(teachers, exams, aliases, p.dbClient.PortalTeachers, ctx)
}

// identityProblems is the pure core of the report, apart from looking up alias targets.
func identityProblems(teachers []*model.Teacher, exams []*model.ZPAExam, aliases []*db.LoginAlias,
	lookup func(context.Context, []int) (map[int]*db.PortalTeacher, error), ctx context.Context) ([]*model.IdentityProblem, error) {
	hasExams := make(map[int]bool)
	for _, e := range exams {
		if e.MainExamerID > 0 {
			hasExams[e.MainExamerID] = true
		}
	}
	aliased := make(map[int]bool)
	for _, a := range aliases {
		aliased[a.TeacherID] = true
	}

	problems := make([]*model.IdentityProblem, 0)
	byAddress := make(map[string][]*model.Teacher)
	for _, t := range teachers {
		if t.ID <= 0 {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(t.Email))
		if email != "" {
			byAddress[email] = append(byAddress[email], t)
		}
		if aliased[t.ID] || !hasExams[t.ID] {
			continue
		}
		switch {
		case email == "":
			problems = append(problems, &model.IdentityProblem{Kind: model.IdentityProblemKindNoAddress,
				TeacherID: t.ID, TeacherName: t.Fullname, HasExams: true})
		case !strings.HasSuffix(email, "@hm.edu"):
			problems = append(problems, &model.IdentityProblem{Kind: model.IdentityProblemKindExternalAddress,
				TeacherID: t.ID, TeacherName: t.Fullname, Email: email, HasExams: true})
		}
	}
	for email, shared := range byAddress {
		if len(shared) < 2 {
			continue
		}
		for _, t := range shared {
			problems = append(problems, &model.IdentityProblem{Kind: model.IdentityProblemKindSharedAddress,
				TeacherID: t.ID, TeacherName: t.Fullname, Email: email, HasExams: hasExams[t.ID]})
		}
	}

	ids := make([]int, 0, len(aliases))
	for _, a := range aliases {
		ids = append(ids, a.TeacherID)
	}
	known, err := lookup(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, a := range aliases {
		if _, ok := known[a.TeacherID]; !ok {
			problems = append(problems, &model.IdentityProblem{Kind: model.IdentityProblemKindAliasUnknownTeacher,
				TeacherID: a.TeacherID, Email: a.Email, HasExams: hasExams[a.TeacherID]})
		}
	}

	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Kind != problems[j].Kind {
			return problems[i].Kind < problems[j].Kind
		}
		return problems[i].TeacherName < problems[j].TeacherName
	})
	return problems, nil
}

// AccessResolveLogin shows how the server would resolve a login right now.
func (p *Plexams) AccessResolveLogin(ctx context.Context, email string) (*model.ResolvedLogin, error) {
	e, err := normalizeLogin(email)
	if err != nil {
		return nil, err
	}
	user, err := p.GetUserByEmail(ctx, e)
	if err != nil {
		return nil, err
	}
	login, err := p.ResolvePortalLogin(ctx, e)
	if err != nil {
		return nil, err
	}
	names, err := p.dbClient.PortalTeachers(ctx, login.TeacherIDs)
	if err != nil {
		return nil, err
	}
	out := &model.ResolvedLogin{
		Email:    e,
		Teachers: make([]*model.Teacher, 0, len(login.TeacherIDs)),
		ActsFor:  login.ActsFor,
		Blocked:  login.Blocked,
	}
	if user != nil {
		role := user.Role
		out.Role = &role
	}
	for _, id := range login.TeacherIDs {
		t := &model.Teacher{ID: id}
		if n, ok := names[id]; ok {
			t.Fullname, t.Shortname = n.Fullname, n.Shortname
		}
		out.Teachers = append(out.Teachers, t)
	}
	out.PortalAccess = !login.Blocked && (user != nil || len(login.TeacherIDs) > 0 || len(login.ActsFor) > 0 || len(login.Grants) > 0)
	return out, nil
}
