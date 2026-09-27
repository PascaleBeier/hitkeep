package mailables

import (
	"hitkeep/internal/mailer"
)

// TeamInvite implements the mailer.Mailable interface for team membership invites.
type TeamInvite struct {
	LocaleCode           string
	Link                 string
	TeamName             string
	Inviter              string
	Role                 string
	RequiresAccountSetup bool
}

func NewTeamInvite(link, teamName, inviter, role string, requiresAccountSetup bool, locale string) mailer.Mailable {
	return &TeamInvite{
		LocaleCode:           locale,
		Link:                 link,
		TeamName:             teamName,
		Inviter:              inviter,
		Role:                 role,
		RequiresAccountSetup: requiresAccountSetup,
	}
}

func (m *TeamInvite) Subject() string {
	if m.RequiresAccountSetup {
		return mailer.Translatef(m.LocaleCode, "subject.team_invite_new", m.TeamName)
	}
	return mailer.Translatef(m.LocaleCode, "subject.team_invite_existing", m.TeamName)
}

func (m *TeamInvite) Template() string {
	return "team_invite.mjml"
}

func (m *TeamInvite) Data() any {
	return struct {
		Link                 string
		TeamName             string
		Inviter              string
		Role                 string
		RequiresAccountSetup bool
	}{
		Link:                 m.Link,
		TeamName:             m.TeamName,
		Inviter:              m.Inviter,
		Role:                 m.Role,
		RequiresAccountSetup: m.RequiresAccountSetup,
	}
}

func (m *TeamInvite) Locale() string { return m.LocaleCode }
