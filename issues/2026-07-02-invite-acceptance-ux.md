---
title: "[Bug]: Invite links redirect unauthenticated invitees to generic login"
labels: bug
---

## Summary
Invite email links send unauthenticated recipients to the generic login page instead of an invite acceptance flow.

## Environment
- **Product/Service**: HitKeep hosted cloud and self-hosted dashboard
- **Region/Version**: Current `main`
- **Browser/OS**: Any browser

## Reproduction Steps
1. Invite a new email address to a team.
2. Open the invite email as a recipient without an active HitKeep session.
3. Click the `/accept-invite?token=...` link.

## Expected Behavior
The invite link opens an invite-specific page where the recipient can set up the invited account or gets clear sign-in guidance for an existing account.

## Actual Behavior
There is no public Angular route for `/accept-invite`. The route falls through to `/dashboard`; the app-root `authGuard` redirects unauthenticated users to `/login?returnUrl=...`. The login page then shows normal sign-in and hosted EU/US signup links, which makes new invitees think they need to create a separate cloud area.

## Error Details
```text
No runtime error. Invite deep link is routed to the generic login flow.
```

## Visual Evidence
Not attached.

## Impact
**Medium** - Team invitation onboarding is confusing for new users and can block evaluation/self-hosting decisions, but the backend invite acceptance API still exists.

## Release Notes
Fix invitation links so invited users see the correct account setup or sign-in flow instead of the generic login page.

## Docs Impact
Needed: update user-facing collaboration/team invitation docs if they describe invite acceptance.

## Additional Context
Customer feedback: a user testing the free hosted version liked the product, but paused self-hosting because the invite email link led to login even though the invited person had no credentials and only saw EU/US signup options.

Relevant implementation:
- Team invites: `sendTeamInviteEmail` creates `/accept-invite?token=...` links and uses the `team_invite` mail templates.
- Site member invites: `NewUserInvite` creates `/accept-invite?token=...` links and uses the `user_invite` mail templates.
- Backend acceptance: `/api/auth/accept-invite` sets the password and accepts pending team invites.
- Frontend routing: no `/accept-invite` route exists; `**` redirects to `/dashboard`, then `authGuard` redirects unauthenticated users to login.
- Existing tests cover the backend accept-invite API, team invite persistence, email template rendering, team invite list/resend/revoke UI surfaces, and generic auth-guard redirects. They do not cover opening the actual emailed `/accept-invite?token=...` URL as an unauthenticated browser user.

Relevant history: `2d74412f` added persistent team invites and `/api/auth/accept-invite`; `9155cc1b` added `authGuard` to the app root and made unauthenticated deep links redirect to `/login?returnUrl=...`.
