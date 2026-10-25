package lk

import (
	"context"
	"iter"
	"time"

	"github.com/swiftj/golk/lk/internal/gen"
)

// MembersService manages the project's members (G§members and invites).
type MembersService struct{ c *Client }

// InvitesService manages the project's pending invites.
type InvitesService struct{ c *Client }

// Role is a member's or invite's role in the project. Values the SDK does
// not know are kept as read.
type Role string

// Roles.
const (
	RoleOwner  Role = "OWNER"
	RoleAdmin  Role = "ADMIN"
	RoleMember Role = "MEMBER"
)

// Member is a project member. ID is the user id. Email is empty unless the
// caller has the ADMIN role.
type Member struct {
	ID    string
	Name  string
	Email string
	Role  Role
	Image string
}

// InviteResult is the result of Members.Invite. An address that belongs to
// a member, or has a pending invite, succeeds with AlreadyMember or
// AlreadyInvited set, and no email is sent.
type InviteResult struct {
	Success        bool
	AlreadyMember  bool
	AlreadyInvited bool
}

// Invite is a pending invite. ExpiresAt is zero when the server sends none;
// an expired invite stays listed until someone tries to accept it.
type Invite struct {
	ID        string
	Email     string
	Role      Role
	CreatedAt time.Time
	ExpiresAt time.Time
}

// List returns every member of the project on one page.
func (s *MembersService) List(ctx context.Context, opts ...CallOption) (*Page[Member], error) {
	var w gen.MemberList
	if err := s.c.call(ctx, gen.OpListMembers.Method, gen.OpListMembers.Path, nil, request{out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Member, len(w.Data))
	for i, m := range w.Data {
		out[i] = Member{ID: m.ID, Name: m.Name.Or(""), Email: m.Email.Or(""), Role: Role(m.Role.Or("")), Image: m.Image.Or("")}
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// All iterates over the project's members.
func (s *MembersService) All(ctx context.Context, opts ...CallOption) iter.Seq2[Member, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Member], error) { return s.List(ctx, opts...) })
}

// Invite invites email as a MEMBER: a pending invite that expires after 7
// days, and an email with an accept link. It needs the ADMIN role. Too many
// pending invites fails with ErrTooManyPendingInvites.
func (s *MembersService) Invite(ctx context.Context, email string, opts ...CallOption) (*InviteResult, error) {
	var w gen.InviteResult
	if err := s.c.call(ctx, gen.OpInviteMember.Method, gen.OpInviteMember.Path, nil,
		request{body: gen.MemberInviteParams{Email: email}, out: &w}, opts); err != nil {
		return nil, err
	}
	return &InviteResult{Success: w.Success, AlreadyMember: w.AlreadyMember.Or(false), AlreadyInvited: w.AlreadyInvited.Or(false)}, nil
}

// Remove removes the member with user id userID. A caller may remove a
// member whose role ranks below theirs, or themselves unless they are the
// owner; anything else fails with ErrForbidden.
func (s *MembersService) Remove(ctx context.Context, userID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpRemoveMember.Method, gen.OpRemoveMember.Path, map[string]string{"userId": userID}, request{}, opts)
}

// List returns the project's pending invites on one page. It needs the
// ADMIN role.
func (s *InvitesService) List(ctx context.Context, opts ...CallOption) (*Page[Invite], error) {
	var w gen.InviteList
	if err := s.c.call(ctx, gen.OpListInvites.Method, gen.OpListInvites.Path, nil, request{out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Invite, len(w.Data))
	for i, v := range w.Data {
		out[i] = Invite{ID: v.ID, Email: v.Email.Or(""), Role: Role(v.Role.Or("")),
			CreatedAt: utc(v.CreatedAt.Or(time.Time{})), ExpiresAt: utc(v.ExpiresAt.Or(time.Time{}))}
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// All iterates over the project's pending invites.
func (s *InvitesService) All(ctx context.Context, opts ...CallOption) iter.Seq2[Invite, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Invite], error) { return s.List(ctx, opts...) })
}

// Revoke revokes a pending invite so its link no longer works. It needs the
// ADMIN role.
func (s *InvitesService) Revoke(ctx context.Context, inviteID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpRevokeInvite.Method, gen.OpRevokeInvite.Path, map[string]string{"inviteId": inviteID}, request{}, opts)
}
