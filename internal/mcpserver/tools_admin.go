package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

// The admin tools manage who can see the project. Players are members, so
// these change who reads the wiki; the skill tells Claude to confirm with
// the user first. The guard has nothing to check: no content is written.

func (s *server) addAdminTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name:        "list_members",
		Description: "List the project's members (its players and admins) with their user id, name, role (OWNER, ADMIN or MEMBER) and, for an ADMIN key, email.",
		Annotations: readOnly("List members"),
	}, func(in ListMembersIn) string { return in.Project }, s.listMembers)
	addTool(s, ms, &mcp.Tool{
		Name:        "list_invites",
		Description: "List the project's pending invites with email, role and expiry. Needs an ADMIN key.",
		Annotations: readOnly("List invites"),
	}, func(in ListMembersIn) string { return in.Project }, s.listInvites)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name: "invite_member",
		Description: "Invite someone to the project by email, as a MEMBER: it sends them an email with a link to accept, valid 7 days. Needs an ADMIN key. " +
			"Confirm the address with the user first. An address that is already a member or invited is reported, and no email is sent.",
		Annotations: &mcp.ToolAnnotations{Title: "Invite member", DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, func(in InviteMemberIn) string { return in.Project }, serial(s, s.inviteMember))
	addTool(s, ms, &mcp.Tool{
		Name: "remove_member",
		Description: "Remove a member from the project by user id (list_members). You can remove a member whose role ranks below yours, or yourself unless you are the owner. " +
			"Confirm with the user first; use dry_run to see who it is.",
		Annotations: write("Remove member", true, true),
	}, func(in RemoveMemberIn) string { return in.Project }, serial(s, s.removeMember))
	addTool(s, ms, &mcp.Tool{
		Name:        "revoke_invite",
		Description: "Revoke a pending invite (list_invites) so its link no longer works. Needs an ADMIN key.",
		Annotations: write("Revoke invite", true, true),
	}, func(in RevokeInviteIn) string { return in.Project }, serial(s, s.revokeInvite))
}

// ListMembersIn is list_members' and list_invites' input.
type ListMembersIn struct {
	ProjectArg
}

// MemberOut is one member.
type MemberOut struct {
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role,omitempty"`
}

// MembersOut is list_members' output.
type MembersOut struct {
	Members []MemberOut `json:"members"`
}

func memberLabel(m lk.Member) string {
	who := m.Name
	if who == "" {
		who = m.Email
	}
	if who == "" {
		who = "(no name)"
	}
	return fmt.Sprintf("%s (%s) %s", who, m.ID, orDash(string(m.Role)))
}

func (s *server) listMembers(ctx context.Context, c *lk.Client, _ ListMembersIn) (toolResult[MembersOut], error) {
	var zero toolResult[MembersOut]
	p, err := c.Members.List(ctx)
	if err != nil {
		return zero, err
	}
	out := MembersOut{Members: []MemberOut{}}
	var b strings.Builder
	for _, m := range p.Items {
		out.Members = append(out.Members, MemberOut{UserID: m.ID, Name: m.Name, Email: m.Email, Role: string(m.Role)})
		fmt.Fprintf(&b, "- %s", memberLabel(m))
		if m.Email != "" && m.Name != "" {
			fmt.Fprintf(&b, " <%s>", m.Email)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%d member(s).", len(p.Items))
	return toolResult[MembersOut]{out: out, text: b.String()}, nil
}

// InviteOut is one pending invite.
type InviteOut struct {
	InviteID  string `json:"invite_id"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// InvitesOut is list_invites' output.
type InvitesOut struct {
	Invites []InviteOut `json:"invites"`
}

func inviteOut(v lk.Invite) InviteOut {
	o := InviteOut{InviteID: v.ID, Email: v.Email, Role: string(v.Role)}
	if !v.ExpiresAt.IsZero() {
		o.ExpiresAt = v.ExpiresAt.Format("2006-01-02T15:04:05Z")
	}
	return o
}

func (s *server) listInvites(ctx context.Context, c *lk.Client, _ ListMembersIn) (toolResult[InvitesOut], error) {
	var zero toolResult[InvitesOut]
	p, err := c.Invites.List(ctx)
	if err != nil {
		return zero, err
	}
	out := InvitesOut{Invites: []InviteOut{}}
	var b strings.Builder
	for _, v := range p.Items {
		o := inviteOut(v)
		out.Invites = append(out.Invites, o)
		fmt.Fprintf(&b, "- %s (%s) %s, expires %s\n", orDash(v.Email), v.ID, orDash(string(v.Role)), orDash(o.ExpiresAt))
	}
	fmt.Fprintf(&b, "%d pending invite(s).", len(p.Items))
	return toolResult[InvitesOut]{out: out, text: b.String()}, nil
}

// InviteMemberIn is invite_member's input.
type InviteMemberIn struct {
	ProjectArg
	DryRunArg
	Email string `json:"email" jsonschema:"the address to invite"`
}

// InviteMemberOut is invite_member's output.
type InviteMemberOut struct {
	Email          string `json:"email"`
	AlreadyMember  bool   `json:"already_member,omitempty"`
	AlreadyInvited bool   `json:"already_invited,omitempty"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

func (s *server) inviteMember(ctx context.Context, c *lk.Client, in InviteMemberIn) (toolResult[InviteMemberOut], error) {
	var zero toolResult[InviteMemberOut]
	email := strings.TrimSpace(in.Email)
	if email == "" || !strings.Contains(email, "@") {
		return zero, errors.New("email must be an email address")
	}
	out := InviteMemberOut{Email: email, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[InviteMemberOut]{out: out, text: fmt.Sprintf("Dry run: would invite %s as a MEMBER and email them an accept link valid for 7 days. Nothing was sent.", email)}, nil
	}
	r, err := c.Members.Invite(ctx, email)
	if err != nil {
		return zero, err
	}
	out.AlreadyMember, out.AlreadyInvited = r.AlreadyMember, r.AlreadyInvited
	switch {
	case r.AlreadyMember:
		return toolResult[InviteMemberOut]{out: out, text: fmt.Sprintf("%s is already a member; no email was sent.", email)}, nil
	case r.AlreadyInvited:
		return toolResult[InviteMemberOut]{out: out, text: fmt.Sprintf("%s already has a pending invite; no email was sent.", email)}, nil
	}
	return toolResult[InviteMemberOut]{out: out, text: fmt.Sprintf("Invited %s as a MEMBER; they were emailed an accept link valid for 7 days.", email)}, nil
}

// RemoveMemberIn is remove_member's input.
type RemoveMemberIn struct {
	ProjectArg
	DryRunArg
	UserID string `json:"user_id" jsonschema:"the member's user id (list_members)"`
}

func (s *server) removeMember(ctx context.Context, c *lk.Client, in RemoveMemberIn) (toolResult[MemberOut], error) {
	var zero toolResult[MemberOut]
	if in.UserID == "" {
		return zero, errors.New("user_id is required")
	}
	p, err := c.Members.List(ctx)
	if err != nil {
		return zero, err
	}
	var m *lk.Member
	for i := range p.Items {
		if p.Items[i].ID == in.UserID {
			m = &p.Items[i]
		}
	}
	if m == nil {
		return zero, fmt.Errorf("no member has user id %s. Hint: list_members lists them", in.UserID)
	}
	out := MemberOut{UserID: m.ID, Name: m.Name, Email: m.Email, Role: string(m.Role)}
	if in.DryRun {
		return toolResult[MemberOut]{out: out, text: fmt.Sprintf("Dry run: would remove %s from the project; they would lose access to the wiki. Nothing was written.", memberLabel(*m))}, nil
	}
	if err := c.Members.Remove(ctx, m.ID); err != nil {
		return zero, err
	}
	return toolResult[MemberOut]{out: out, text: fmt.Sprintf("Removed %s from the project.", memberLabel(*m))}, nil
}

// RevokeInviteIn is revoke_invite's input.
type RevokeInviteIn struct {
	ProjectArg
	DryRunArg
	InviteID string `json:"invite_id" jsonschema:"the pending invite (list_invites)"`
}

func (s *server) revokeInvite(ctx context.Context, c *lk.Client, in RevokeInviteIn) (toolResult[InviteOut], error) {
	var zero toolResult[InviteOut]
	if in.InviteID == "" {
		return zero, errors.New("invite_id is required")
	}
	p, err := c.Invites.List(ctx)
	if err != nil {
		return zero, err
	}
	var v *lk.Invite
	for i := range p.Items {
		if p.Items[i].ID == in.InviteID {
			v = &p.Items[i]
		}
	}
	if v == nil {
		return zero, fmt.Errorf("no pending invite %s. Hint: list_invites lists them", in.InviteID)
	}
	out := inviteOut(*v)
	if in.DryRun {
		return toolResult[InviteOut]{out: out, text: fmt.Sprintf("Dry run: would revoke the invite of %s (%s). Nothing was written.", orDash(v.Email), v.ID)}, nil
	}
	if err := c.Invites.Revoke(ctx, v.ID); err != nil {
		return zero, err
	}
	return toolResult[InviteOut]{out: out, text: fmt.Sprintf("Revoked the invite of %s (%s); its link no longer works.", orDash(v.Email), v.ID)}, nil
}
