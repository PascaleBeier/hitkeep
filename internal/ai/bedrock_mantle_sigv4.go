package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

const bedrockSigV4Service = "bedrock"

type bedrockMantleSigV4Transport struct {
	base   http.RoundTripper
	region string

	mu          sync.Mutex
	credentials aws.CredentialsProvider
	signer      *v4.Signer
	loadConfig  func(context.Context, string) (aws.Config, error)
	now         func() time.Time
}

func newBedrockMantleSigV4HTTPClient(region string) *http.Client {
	return &http.Client{
		Transport: &bedrockMantleSigV4Transport{
			base:       http.DefaultTransport,
			region:     strings.TrimSpace(region),
			loadConfig: loadDefaultAWSConfig,
			now:        time.Now,
		},
	}
}

func loadDefaultAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

func isBedrockMantleBaseURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasPrefix(host, "bedrock-mantle.") && strings.HasSuffix(host, ".api.aws")
}

func (t *bedrockMantleSigV4Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil {
		return nil, fmt.Errorf("bedrock mantle signer is not configured")
	}
	if t.region == "" {
		return nil, fmt.Errorf("bedrock mantle signer region is not configured")
	}

	body, err := readRequestBody(req)
	if err != nil {
		return nil, err
	}

	signed := req.Clone(req.Context())
	signed.Body = io.NopCloser(bytes.NewReader(body))
	signed.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	signed.ContentLength = int64(len(body))

	credentials, signer, err := t.signingDependencies(req.Context())
	if err != nil {
		return nil, err
	}
	creds, err := credentials.Retrieve(req.Context())
	if err != nil {
		return nil, fmt.Errorf("load AWS credentials for Bedrock Mantle: %w", err)
	}

	sum := sha256.Sum256(body)
	if err := signer.SignHTTP(req.Context(), creds, signed, hex.EncodeToString(sum[:]), bedrockSigV4Service, t.region, t.now()); err != nil {
		return nil, fmt.Errorf("sign Bedrock Mantle request: %w", err)
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(signed)
}

func readRequestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if closeErr := req.Body.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("read Bedrock Mantle request body for signing: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func (t *bedrockMantleSigV4Transport) signingDependencies(ctx context.Context) (aws.CredentialsProvider, *v4.Signer, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.credentials != nil && t.signer != nil {
		return t.credentials, t.signer, nil
	}

	loadConfig := t.loadConfig
	if loadConfig == nil {
		loadConfig = loadDefaultAWSConfig
	}
	cfg, err := loadConfig(ctx, t.region)
	if err != nil {
		return nil, nil, fmt.Errorf("load AWS config for Bedrock Mantle: %w", err)
	}
	t.credentials = cfg.Credentials
	t.signer = v4.NewSigner()
	return t.credentials, t.signer, nil
}
