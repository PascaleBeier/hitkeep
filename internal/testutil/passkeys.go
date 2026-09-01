package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"

	json "hitkeep/jsonapi"
)

type PasskeyFixture struct {
	privateKey *ecdsa.PrivateKey
	credential webauthnlib.Credential
}

func NewPasskeyFixture() (*PasskeyFixture, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate passkey private key: %w", err)
	}

	credentialID := make([]byte, 32)
	if _, err := rand.Read(credentialID); err != nil {
		return nil, fmt.Errorf("generate credential id: %w", err)
	}

	publicKeyBytes, err := privateKey.PublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encode passkey public key: %w", err)
	}
	if len(publicKeyBytes) != 65 || publicKeyBytes[0] != 0x04 {
		return nil, fmt.Errorf("unexpected p-256 public key encoding length: %d", len(publicKeyBytes))
	}

	coseKey, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		KeyType:   int64(webauthncose.EllipticKey),
		Algorithm: int64(webauthncose.AlgES256),
		Curve:     int64(webauthncose.P256),
		XCoord:    append([]byte(nil), publicKeyBytes[1:33]...),
		YCoord:    append([]byte(nil), publicKeyBytes[33:65]...),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal credential public key: %w", err)
	}

	return &PasskeyFixture{
		privateKey: privateKey,
		credential: webauthnlib.Credential{
			ID:              credentialID,
			PublicKey:       coseKey,
			AttestationType: "none",
			Transport:       []protocol.AuthenticatorTransport{protocol.Internal},
			Flags: webauthnlib.CredentialFlags{
				UserPresent:    true,
				UserVerified:   true,
				BackupEligible: false,
				BackupState:    false,
			},
			Authenticator: webauthnlib.Authenticator{
				AAGUID:     make([]byte, 16),
				SignCount:  0,
				Attachment: protocol.Platform,
			},
		},
	}, nil
}

func (f *PasskeyFixture) Credential() webauthnlib.Credential {
	return f.credential
}

func (f *PasskeyFixture) CredentialID() string {
	return base64.RawURLEncoding.EncodeToString(f.credential.ID)
}

func (f *PasskeyFixture) LegacyPublicKey() (string, error) {
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&f.privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshal legacy passkey public key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(publicKeyDER), nil
}

func (f *PasskeyFixture) RegistrationResponse(challenge protocol.URLEncodedBase64, origin, rpID string) (protocol.CredentialCreationResponse, error) {
	clientDataJSON, err := marshalClientData("webauthn.create", challenge.String(), origin)
	if err != nil {
		return protocol.CredentialCreationResponse{}, err
	}

	authData, err := makeCredentialAuthData(rpID, f.credential.ID, f.credential.PublicKey)
	if err != nil {
		return protocol.CredentialCreationResponse{}, err
	}
	attestationObject, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	})
	if err != nil {
		return protocol.CredentialCreationResponse{}, fmt.Errorf("marshal attestation object: %w", err)
	}

	return protocol.CredentialCreationResponse{
		ID:    f.CredentialID(),
		Type:  string(protocol.PublicKeyCredentialType),
		RawID: f.credential.ID,
		AttestationResponse: protocol.AuthenticatorAttestationResponse{
			AuthenticatorResponse: protocol.AuthenticatorResponse{
				ClientDataJSON: clientDataJSON,
			},
			AttestationObject: attestationObject,
			Transports:        []string{string(protocol.Internal)},
		},
	}, nil
}

func (f *PasskeyFixture) AssertionResponse(challenge protocol.URLEncodedBase64, origin, rpID string, userHandle []byte, signCount uint32, userVerified bool) (protocol.CredentialAssertionResponse, error) {
	return f.AssertionResponseWithFlags(challenge, origin, rpID, userHandle, signCount, userVerified, false, false)
}

func (f *PasskeyFixture) AssertionResponseWithFlags(
	challenge protocol.URLEncodedBase64,
	origin string,
	rpID string,
	userHandle []byte,
	signCount uint32,
	userVerified bool,
	backupEligible bool,
	backupState bool,
) (protocol.CredentialAssertionResponse, error) {
	clientDataJSON, err := marshalClientData("webauthn.get", challenge.String(), origin)
	if err != nil {
		return protocol.CredentialAssertionResponse{}, err
	}

	authData := makeAssertionAuthData(rpID, signCount, userVerified, backupEligible, backupState)
	clientDataHash := sha256.Sum256(clientDataJSON)
	sigData := append(append([]byte{}, authData...), clientDataHash[:]...)
	digest := sha256.Sum256(sigData)
	signature, err := ecdsa.SignASN1(rand.Reader, f.privateKey, digest[:])
	if err != nil {
		return protocol.CredentialAssertionResponse{}, fmt.Errorf("sign assertion: %w", err)
	}

	return protocol.CredentialAssertionResponse{
		ID:    f.CredentialID(),
		Type:  string(protocol.PublicKeyCredentialType),
		RawID: f.credential.ID,
		AssertionResponse: protocol.AuthenticatorAssertionResponse{
			AuthenticatorResponse: protocol.AuthenticatorResponse{
				ClientDataJSON: clientDataJSON,
			},
			AuthenticatorData: authData,
			Signature:         signature,
			UserHandle:        userHandle,
		},
	}, nil
}

func marshalClientData(ceremonyType, challenge, origin string) ([]byte, error) {
	payload, err := json.Marshal(map[string]string{
		"type":      ceremonyType,
		"challenge": challenge,
		"origin":    origin,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal client data: %w", err)
	}
	return payload, nil
}

func makeCredentialAuthData(rpID string, credentialID, credentialPublicKey []byte) ([]byte, error) {
	rpIDHash := sha256.Sum256([]byte(rpID))

	authData := make([]byte, 0, 32+1+4+16+2+len(credentialID)+len(credentialPublicKey))
	authData = append(authData, rpIDHash[:]...)
	authData = append(authData, byte(protocol.FlagUserPresent|protocol.FlagUserVerified|protocol.FlagAttestedCredentialData))

	counter := make([]byte, 4)
	binary.BigEndian.PutUint32(counter, 0)
	authData = append(authData, counter...)

	authData = append(authData, make([]byte, 16)...)

	if len(credentialID) > math.MaxUint16 {
		return nil, fmt.Errorf("credential id too large: %d", len(credentialID))
	}
	var credentialIDSize uint16
	for range credentialID {
		credentialIDSize++
	}
	credentialIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(credentialIDLen, credentialIDSize)
	authData = append(authData, credentialIDLen...)
	authData = append(authData, credentialID...)
	authData = append(authData, credentialPublicKey...)

	return authData, nil
}

func makeAssertionAuthData(rpID string, signCount uint32, userVerified bool, backupEligible bool, backupState bool) []byte {
	rpIDHash := sha256.Sum256([]byte(rpID))

	flags := byte(protocol.FlagUserPresent)
	if userVerified {
		flags |= byte(protocol.FlagUserVerified)
	}
	if backupEligible {
		flags |= byte(protocol.FlagBackupEligible)
	}
	if backupState {
		flags |= byte(protocol.FlagBackupState)
	}

	authData := make([]byte, 0, 37)
	authData = append(authData, rpIDHash[:]...)
	authData = append(authData, flags)

	counter := make([]byte, 4)
	binary.BigEndian.PutUint32(counter, signCount)
	authData = append(authData, counter...)

	return authData
}
