package shared

import (
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"hitkeep/internal/database"
)

const (
	authStateCacheSize               = 4096
	pendingTOTPSetupTTL              = 10 * time.Minute
	passkeyRegistrationTTL           = 5 * time.Minute
	passkeyLoginChallengeTTL         = 5 * time.Minute
	googleSearchConsoleOAuthStateTTL = 10 * time.Minute
)

type PendingTOTPSetup struct {
	Secret    string
	ExpiresAt time.Time
}

type PasskeyRegistrationChallenge struct {
	Challenge     string
	RequestedName string
	ExpiresAt     time.Time
	Session       *webauthn.SessionData
}

type MFAEmailLink struct {
	ChallengeID uuid.UUID
	ReturnPath  string
	ExpiresAt   time.Time
}

type GoogleSearchConsoleOAuthState struct {
	UserID     uuid.UUID
	TeamID     uuid.UUID
	ReturnPath string
	ExpiresAt  time.Time
}

type AuthStateStore struct {
	pendingTOTP          *lru.LRU[uuid.UUID, PendingTOTPSetup]
	passkeyRegistrations *lru.LRU[uuid.UUID, PasskeyRegistrationChallenge]
	mfaEmailLinks        *lru.LRU[uuid.UUID, MFAEmailLink]
	googleSearchConsole  *lru.LRU[uuid.UUID, GoogleSearchConsoleOAuthState]

	loginChallengesMu       sync.Mutex
	loginChallenges         *lru.LRU[uuid.UUID, database.LoginChallenge]
	loginChallengeIDsByUser *lru.LRU[uuid.UUID, map[uuid.UUID]struct{}]
}

func NewAuthStateStore() *AuthStateStore {
	return &AuthStateStore{
		pendingTOTP:          lru.NewLRU[uuid.UUID, PendingTOTPSetup](authStateCacheSize, nil, pendingTOTPSetupTTL),
		passkeyRegistrations: lru.NewLRU[uuid.UUID, PasskeyRegistrationChallenge](authStateCacheSize, nil, passkeyRegistrationTTL),
		mfaEmailLinks:        lru.NewLRU[uuid.UUID, MFAEmailLink](authStateCacheSize, nil, passkeyLoginChallengeTTL),
		googleSearchConsole:  lru.NewLRU[uuid.UUID, GoogleSearchConsoleOAuthState](authStateCacheSize, nil, googleSearchConsoleOAuthStateTTL),
		loginChallenges:      lru.NewLRU[uuid.UUID, database.LoginChallenge](authStateCacheSize, nil, passkeyLoginChallengeTTL),
		loginChallengeIDsByUser: lru.NewLRU[uuid.UUID, map[uuid.UUID]struct{}](
			authStateCacheSize,
			nil,
			passkeyLoginChallengeTTL,
		),
	}
}

func cloneChallengeIDSet(input map[uuid.UUID]struct{}) map[uuid.UUID]struct{} {
	if len(input) == 0 {
		return map[uuid.UUID]struct{}{}
	}

	cloned := make(map[uuid.UUID]struct{}, len(input))
	for id := range input {
		cloned[id] = struct{}{}
	}
	return cloned
}

func (s *AuthStateStore) addLoginChallengeUserIndex(userID uuid.UUID, challengeID uuid.UUID) {
	if s == nil || userID == uuid.Nil || challengeID == uuid.Nil {
		return
	}

	challengeIDs, _ := s.loginChallengeIDsByUser.Peek(userID)
	cloned := cloneChallengeIDSet(challengeIDs)
	cloned[challengeID] = struct{}{}
	s.loginChallengeIDsByUser.Add(userID, cloned)
}

func (s *AuthStateStore) removeLoginChallengeUserIndex(userID uuid.UUID, challengeID uuid.UUID) {
	if s == nil || userID == uuid.Nil || challengeID == uuid.Nil {
		return
	}

	challengeIDs, ok := s.loginChallengeIDsByUser.Peek(userID)
	if !ok {
		return
	}

	cloned := cloneChallengeIDSet(challengeIDs)
	delete(cloned, challengeID)
	if len(cloned) == 0 {
		s.loginChallengeIDsByUser.Remove(userID)
		return
	}
	s.loginChallengeIDsByUser.Add(userID, cloned)
}

func (s *AuthStateStore) CreatePendingTOTPSetup(userID uuid.UUID, secret string, expiresAt time.Time) {
	if s == nil {
		return
	}
	s.pendingTOTP.Add(userID, PendingTOTPSetup{
		Secret:    strings.TrimSpace(secret),
		ExpiresAt: expiresAt.UTC(),
	})
}

func (s *AuthStateStore) GetPendingTOTPSetup(userID uuid.UUID) (secret string, expiresAt time.Time, found bool) {
	if s == nil {
		return "", time.Time{}, false
	}

	entry, ok := s.pendingTOTP.Get(userID)
	if !ok || strings.TrimSpace(entry.Secret) == "" {
		return "", time.Time{}, false
	}
	if isExpired(entry.ExpiresAt) {
		s.pendingTOTP.Remove(userID)
		return "", time.Time{}, false
	}

	return entry.Secret, entry.ExpiresAt, true
}

func (s *AuthStateStore) HasPendingTOTPSetup(userID uuid.UUID) bool {
	_, _, found := s.GetPendingTOTPSetup(userID)
	return found
}

func (s *AuthStateStore) DeletePendingTOTPSetup(userID uuid.UUID) {
	if s == nil {
		return
	}
	s.pendingTOTP.Remove(userID)
}

func (s *AuthStateStore) CreatePasskeyChallenge(userID uuid.UUID, challenge string, requestedName string, expiresAt time.Time, session *webauthn.SessionData) {
	if s == nil {
		return
	}
	s.passkeyRegistrations.Add(userID, PasskeyRegistrationChallenge{
		Challenge:     strings.TrimSpace(challenge),
		RequestedName: strings.TrimSpace(requestedName),
		ExpiresAt:     expiresAt.UTC(),
		Session:       session,
	})
}

func (s *AuthStateStore) GetPasskeyChallenge(userID uuid.UUID) (challenge string, requestedName string, expiresAt time.Time, session *webauthn.SessionData, found bool) {
	if s == nil {
		return "", "", time.Time{}, nil, false
	}

	entry, ok := s.passkeyRegistrations.Get(userID)
	if !ok || strings.TrimSpace(entry.Challenge) == "" {
		return "", "", time.Time{}, nil, false
	}
	if isExpired(entry.ExpiresAt) {
		s.passkeyRegistrations.Remove(userID)
		return "", "", time.Time{}, nil, false
	}

	return entry.Challenge, entry.RequestedName, entry.ExpiresAt, entry.Session, true
}

func (s *AuthStateStore) DeletePasskeyChallenge(userID uuid.UUID) {
	if s == nil {
		return
	}
	s.passkeyRegistrations.Remove(userID)
}

func (s *AuthStateStore) CreateMFAEmailLink(challengeID uuid.UUID, returnPath string, expiresAt time.Time) uuid.UUID {
	tokenID := uuid.New()
	if s == nil {
		return tokenID
	}

	s.mfaEmailLinks.Add(tokenID, MFAEmailLink{
		ChallengeID: challengeID,
		ReturnPath:  strings.TrimSpace(returnPath),
		ExpiresAt:   expiresAt.UTC(),
	})
	return tokenID
}

func (s *AuthStateStore) ConsumeMFAEmailLink(tokenID uuid.UUID) (MFAEmailLink, bool) {
	if s == nil || tokenID == uuid.Nil {
		return MFAEmailLink{}, false
	}

	entry, ok := s.mfaEmailLinks.Get(tokenID)
	if !ok {
		return MFAEmailLink{}, false
	}
	if isExpired(entry.ExpiresAt) {
		s.mfaEmailLinks.Remove(tokenID)
		return MFAEmailLink{}, false
	}

	s.mfaEmailLinks.Remove(tokenID)
	return entry, true
}

func (s *AuthStateStore) CreateGoogleSearchConsoleOAuthState(userID, teamID uuid.UUID, returnPath string, expiresAt time.Time) string {
	stateID := uuid.New()
	if s == nil {
		return stateID.String()
	}
	s.googleSearchConsole.Add(stateID, GoogleSearchConsoleOAuthState{
		UserID:     userID,
		TeamID:     teamID,
		ReturnPath: strings.TrimSpace(returnPath),
		ExpiresAt:  expiresAt.UTC(),
	})
	return stateID.String()
}

func (s *AuthStateStore) ConsumeGoogleSearchConsoleOAuthState(rawState string) (GoogleSearchConsoleOAuthState, bool) {
	if s == nil {
		return GoogleSearchConsoleOAuthState{}, false
	}
	stateID, err := uuid.Parse(strings.TrimSpace(rawState))
	if err != nil || stateID == uuid.Nil {
		return GoogleSearchConsoleOAuthState{}, false
	}
	entry, ok := s.googleSearchConsole.Get(stateID)
	if !ok {
		return GoogleSearchConsoleOAuthState{}, false
	}
	s.googleSearchConsole.Remove(stateID)
	if isExpired(entry.ExpiresAt) {
		return GoogleSearchConsoleOAuthState{}, false
	}
	return entry, true
}

func (s *AuthStateStore) CreatePasskeyLoginChallenge(challenge string, input database.CreateLoginChallengeInput, expiresAt time.Time, session *webauthn.SessionData) uuid.UUID {
	challengeID := uuid.New()
	if s == nil {
		return challengeID
	}

	flow := strings.TrimSpace(input.Flow)
	if flow == "" {
		flow = "passwordless"
	}

	entry := database.LoginChallenge{
		ID:         challengeID,
		RememberMe: input.RememberMe,
		Flow:       flow,
		Challenge:  strings.TrimSpace(challenge),
		Session:    session,
		ExpiresAt:  expiresAt.UTC(),
	}
	if input.UserID != nil && *input.UserID != uuid.Nil {
		entry.UserID = *input.UserID
		entry.HasUserID = true
	}

	s.loginChallengesMu.Lock()
	defer s.loginChallengesMu.Unlock()

	s.loginChallenges.Add(challengeID, entry)
	if entry.HasUserID {
		s.addLoginChallengeUserIndex(entry.UserID, challengeID)
	}
	return challengeID
}

func (s *AuthStateStore) GetPasskeyLoginChallenge(challengeID uuid.UUID) (database.LoginChallenge, bool) {
	if s == nil {
		return database.LoginChallenge{}, false
	}

	s.loginChallengesMu.Lock()
	defer s.loginChallengesMu.Unlock()

	entry, ok := s.loginChallenges.Get(challengeID)
	if !ok || strings.TrimSpace(entry.Challenge) == "" {
		return database.LoginChallenge{}, false
	}
	if isExpired(entry.ExpiresAt) {
		s.loginChallenges.Remove(challengeID)
		if entry.HasUserID {
			s.removeLoginChallengeUserIndex(entry.UserID, challengeID)
		}
		return database.LoginChallenge{}, false
	}

	return entry, true
}

func (s *AuthStateStore) DeletePasskeyLoginChallenge(challengeID uuid.UUID) {
	if s == nil {
		return
	}

	s.loginChallengesMu.Lock()
	defer s.loginChallengesMu.Unlock()

	entry, ok := s.loginChallenges.Peek(challengeID)
	s.loginChallenges.Remove(challengeID)
	if ok && entry.HasUserID {
		s.removeLoginChallengeUserIndex(entry.UserID, challengeID)
	}
}

func (s *AuthStateStore) ClearUser(userID uuid.UUID) {
	if s == nil || userID == uuid.Nil {
		return
	}

	s.DeletePendingTOTPSetup(userID)
	s.DeletePasskeyChallenge(userID)

	s.loginChallengesMu.Lock()
	defer s.loginChallengesMu.Unlock()

	challengeIDs, ok := s.loginChallengeIDsByUser.Peek(userID)
	if !ok {
		return
	}
	for challengeID := range challengeIDs {
		s.loginChallenges.Remove(challengeID)
	}
	s.loginChallengeIDsByUser.Remove(userID)
}

func isExpired(expiresAt time.Time) bool {
	return !expiresAt.IsZero() && time.Now().UTC().After(expiresAt.UTC())
}
