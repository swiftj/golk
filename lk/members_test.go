package lk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

func TestMembers(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-MEM-01 list is one page", reply: testutil.JSON(200, `{"data":[{"id":"u1","name":"Finn","email":null,"role":"OWNER","image":null},{"id":"u2","role":"BARD"}],"hasMore":false,"nextCursor":null}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Members.List(ctx) },
			method: "GET", path: pP + "/members",
			check: func(t *testing.T, v any) {
				p := v.(*Page[Member])
				if len(p.Items) != 2 || p.Items[0] != (Member{ID: "u1", Name: "Finn", Role: RoleOwner}) || p.Items[1].Role != "BARD" {
					t.Errorf("members = %+v", p.Items)
				}
			}},
		{name: "C-MEM-02 invite sends the email with a key", reply: testutil.JSON(200, `{"success":true,"alreadyInvited":true}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Members.Invite(ctx, "jake@example.com") },
			method: "POST", path: pP + "/members", keyed: true, body: `{"email":"jake@example.com"}`,
			check: func(t *testing.T, v any) {
				if r := v.(*InviteResult); *r != (InviteResult{Success: true, AlreadyInvited: true}) {
					t.Errorf("result = %+v", r)
				}
			}},
		{name: "C-MEM-03 remove", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Members.Remove(ctx, "u2") },
			method: "DELETE", path: pP + "/members/u2"},
		{name: "C-INV-01 list is one page", reply: testutil.JSON(200, `{"data":[{"id":"i1","email":"jake@example.com","role":"MEMBER","createdAt":"2026-10-20T10:00:00Z","expiresAt":"2026-10-27T10:00:00Z"}],"hasMore":false}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Invites.List(ctx) },
			method: "GET", path: pP + "/invites",
			check: func(t *testing.T, v any) {
				p := v.(*Page[Invite])
				want := Invite{ID: "i1", Email: "jake@example.com", Role: RoleMember,
					CreatedAt: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)}
				if len(p.Items) != 1 || p.Items[0] != want {
					t.Errorf("invites = %+v", p.Items)
				}
			}},
		{name: "C-INV-02 revoke", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Invites.Revoke(ctx, "i1") },
			method: "DELETE", path: pP + "/invites/i1"},
	})
}

func TestMembers_C_MEM_02_TooManyInvites(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(409, `{"message":"Too many pending invites","code":"TOO_MANY_PENDING_INVITES","requestId":"rq1"}`))
	c, _ := newTestClient(t, srv)
	if _, err := c.Members.Invite(context.Background(), "x@example.com"); !errors.Is(err, ErrTooManyPendingInvites) {
		t.Errorf("err = %v, want ErrTooManyPendingInvites", err)
	}
}
