package principal

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestPortalOnlyLoginIsNoStaffUser(t *testing.T) {
	ctx := With(context.Background(), &Principal{Email: "t@example.test", TeacherIDs: []int{7}})
	if UserFromContext(ctx) != nil {
		t.Fatal("a login without app_user row must not look like a staff user")
	}
	if !From(ctx).MayUsePortal() {
		t.Error("a ZPA person may use the portal")
	}
}

func TestWithUserKeepsTheStaffSide(t *testing.T) {
	u := &model.User{Email: "a@example.test", Role: model.RoleAdmin}
	ctx := WithUser(context.Background(), u)
	if UserFromContext(ctx) != u {
		t.Error("WithUser must round-trip through UserFromContext")
	}
	if UserFromContext(context.Background()) != nil {
		t.Error("no principal means no user")
	}
}

func TestMaySpeakFor(t *testing.T) {
	p := &Principal{TeacherIDs: []int{7}, ActsFor: []int{9}}
	for id, want := range map[int]bool{7: true, 9: true, 8: false, 0: false} {
		if got := p.MaySpeakFor(id); got != want {
			t.Errorf("MaySpeakFor(%d) = %v, want %v", id, got, want)
		}
	}
	p.Blocked = true
	if p.MaySpeakFor(7) || p.MayUsePortal() {
		t.Error("a blocked login speaks for nobody")
	}
	var none *Principal
	if none.MaySpeakFor(7) || none.MayUsePortal() || none.MayWrite() {
		t.Error("a nil principal may nothing")
	}
}

func TestReadTokenMayNotWrite(t *testing.T) {
	if !(&Principal{}).MayWrite() {
		t.Error("a session may write")
	}
	if (&Principal{Token: &Token{Scope: TokenRead}}).MayWrite() {
		t.Error("a READ token may not write")
	}
	if !(&Principal{Token: &Token{Scope: TokenWrite}}).MayWrite() {
		t.Error("a WRITE token may write")
	}
}
