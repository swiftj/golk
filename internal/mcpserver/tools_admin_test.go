package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

const (
	amMembers = `{"data":[{"id":"u1","name":"Finn","email":"finn@example.com","role":"OWNER"},{"id":"u2","name":"Jake","role":"MEMBER"}],"hasMore":false}`
	amInvites = `{"data":[{"id":"i1","email":"bmo@example.com","role":"MEMBER","expiresAt":"2026-11-01T00:00:00Z"}],"hasMore":false}`
	amPath    = "/api/v3/projects/p1"
)

// X-MEM-01: list_members and list_invites read the lists.
func TestAdmin_X_MEM_01_Lists(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(amMembers), wtOK(amInvites))
	got := wtMustOK(t, h, "list_members", map[string]any{})
	if !strings.Contains(got, "- Finn (u1) OWNER <finn@example.com>\n- Jake (u2) MEMBER\n2 member(s).") {
		t.Errorf("text %q", got)
	}
	got = wtMustOK(t, h, "list_invites", map[string]any{})
	if !strings.Contains(got, "- bmo@example.com (i1) MEMBER, expires 2026-11-01T00:00:00Z") {
		t.Errorf("text %q", got)
	}
	reqs := h.lk.Requests()
	wtReq(t, reqs[0], http.MethodGet, amPath+"/members", "", "")
	wtReq(t, reqs[1], http.MethodGet, amPath+"/invites", "", "")
}

// X-MEM-02: invite_member posts the email (a dry run sends nothing) and
// reports an address that is already a member.
func TestAdmin_X_MEM_02_Invite(t *testing.T) {
	h := newHarness(t)
	if got := wtMustOK(t, h, "invite_member", map[string]any{"email": "bmo@example.com", "dry_run": true}); !strings.Contains(got, "Nothing was sent") || len(h.lk.Requests()) != 0 {
		t.Errorf("text %q, requests %d", got, len(h.lk.Requests()))
	}
	h.lk.Enqueue(wtOK(`{"success":true,"alreadyMember":true}`))
	if got := wtMustOK(t, h, "invite_member", map[string]any{"email": "jake@example.com"}); !strings.Contains(got, "already a member") {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.writes()[0], http.MethodPost, amPath+"/members", "", `{"email":"jake@example.com"}`)
	wtMustFail(t, h, "invite_member", map[string]any{"email": "nope"}, "email address")
}

// X-MEM-03: remove_member and revoke_invite find the member or invite
// first, name it in the dry run, then delete it.
func TestAdmin_X_MEM_03_RemoveAndRevoke(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(amMembers))
	if got := wtMustOK(t, h, "remove_member", map[string]any{"user_id": "u2", "dry_run": true}); !strings.Contains(got, "would remove Jake (u2) MEMBER") {
		t.Errorf("text %q", got)
	}
	h.lk.Enqueue(wtOK(amMembers), noContentReply())
	wtMustOK(t, h, "remove_member", map[string]any{"user_id": "u2"})
	wtReq(t, h.writes()[0], http.MethodDelete, amPath+"/members/u2", "", "")
	h.lk.Enqueue(wtOK(amMembers))
	wtMustFail(t, h, "remove_member", map[string]any{"user_id": "u9"}, "no member has user id u9")

	h.lk.Enqueue(wtOK(amInvites), noContentReply())
	wtMustOK(t, h, "revoke_invite", map[string]any{"invite_id": "i1"})
	wtReq(t, h.writes()[1], http.MethodDelete, amPath+"/invites/i1", "", "")
}

// Read-only mode keeps the admin reads and drops the admin writes.
func TestAdmin_X_RO_01_ReadOnly(t *testing.T) {
	h := newHarness(t, readOnlyMode())
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tool := range res.Tools {
		have[tool.Name] = true
	}
	if !have["list_members"] || !have["list_invites"] || have["invite_member"] || have["remove_member"] || have["revoke_invite"] {
		t.Errorf("read-only tools %v", have)
	}
}
