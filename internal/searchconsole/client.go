package searchconsole

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	searchconsoleapi "google.golang.org/api/searchconsole/v1"
)

const ReadOnlyScope = "https://www.googleapis.com/auth/webmasters.readonly"

type Client interface {
	AuthCodeURL(state string, redirectURL string) (string, error)
	ExchangeCode(ctx context.Context, code string, redirectURL string) (Token, error)
	ListProperties(ctx context.Context, token Token) ([]Property, error)
	QuerySearchAnalytics(ctx context.Context, token Token, query SearchAnalyticsQuery) ([]SearchAnalyticsRow, error)
}

type Token struct {
	AccessToken        string
	RefreshToken       string
	TokenType          string
	Scope              string
	Expiry             time.Time
	GoogleAccountEmail string
	GoogleAccountID    string
}

type Property struct {
	URI             string
	PermissionLevel string
}

const (
	DataStateFinal = "final"

	CategoryQuotaLimited         ErrorCategory = "quota_limited"
	CategoryAuthorizationRevoked ErrorCategory = "authorization_revoked"
	CategoryTokenRefreshFailed   ErrorCategory = "token_refresh_failed"
	CategoryPropertyAccessLost   ErrorCategory = "property_access_lost"
	CategoryCredentialsInvalid   ErrorCategory = "credentials_invalid"
	CategoryCredentialsMissing   ErrorCategory = "credentials_missing"
	CategoryAPIDisabled          ErrorCategory = "api_disabled"
	CategoryGoogleUnavailable    ErrorCategory = "google_unavailable"
	CategoryUnknown              ErrorCategory = "unknown"
)

type ErrorCategory string

type Error struct {
	Category ErrorCategory
	Err      error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return string(CategoryUnknown)
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ClassifiedError(category ErrorCategory, err error) error {
	if err == nil {
		err = fmt.Errorf("%s", category)
	}
	return &Error{Category: category, Err: err}
}

func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return ""
	}
	var classified *Error
	if errors.As(err, &classified) && classified.Category != "" {
		return classified.Category
	}
	var googleErr *googleapi.Error
	if errors.As(err, &googleErr) {
		return classifyGoogleAPIError(googleErr)
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "quota") || strings.Contains(msg, "rate limit"):
		return CategoryQuotaLimited
	case strings.Contains(msg, "refresh") || strings.Contains(msg, "cannot fetch token") || strings.Contains(msg, "invalid_grant"):
		return CategoryTokenRefreshFailed
	case strings.Contains(msg, "credential"):
		return CategoryCredentialsInvalid
	case strings.Contains(msg, "permission") || strings.Contains(msg, "access"):
		return CategoryPropertyAccessLost
	default:
		return CategoryUnknown
	}
}

func classifyGoogleAPIError(err *googleapi.Error) ErrorCategory {
	if err.Code == http.StatusUnauthorized {
		return CategoryAuthorizationRevoked
	}
	if err.Code == http.StatusTooManyRequests || googleErrorHasReason(err, "quota") || googleErrorHasReason(err, "rateLimit") {
		return CategoryQuotaLimited
	}
	if googleErrorHasReason(err, "accessNotConfigured") || googleErrorHasReason(err, "SERVICE_DISABLED") || googleErrorHasReason(err, "api has not been used") || googleErrorHasReason(err, "disabled") {
		return CategoryAPIDisabled
	}
	if err.Code == http.StatusForbidden {
		return CategoryPropertyAccessLost
	}
	if err.Code >= 500 {
		return CategoryGoogleUnavailable
	}
	if err.Code == http.StatusBadRequest {
		return CategoryCredentialsInvalid
	}
	return CategoryUnknown
}

func googleErrorHasReason(err *googleapi.Error, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	if strings.Contains(strings.ToLower(err.Message), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(err.Body), needle) {
		return true
	}
	for _, item := range err.Errors {
		if strings.Contains(strings.ToLower(item.Reason), needle) || strings.Contains(strings.ToLower(item.Message), needle) {
			return true
		}
	}
	return false
}

type SearchAnalyticsQuery struct {
	SiteURL         string
	StartDate       time.Time
	EndDate         time.Time
	Dimensions      []string
	DataState       string
	AggregationType string
	RowLimit        int64
}

type SearchAnalyticsRow struct {
	Date            time.Time
	Query           string
	Page            string
	Country         string
	Device          string
	Clicks          int
	Impressions     int
	CTR             float64
	Position        float64
	AggregationType string
	DataState       string
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	APIBaseURL   string
}

type GoogleClient struct {
	config OAuthConfig
}

func NewGoogleClient(config OAuthConfig) *GoogleClient {
	return &GoogleClient{config: config}
}

func (c *GoogleClient) AuthCodeURL(state string, redirectURL string) (string, error) {
	oauthConfig, err := c.oauthConfig(redirectURL)
	if err != nil {
		return "", err
	}
	return oauthConfig.AuthCodeURL(strings.TrimSpace(state), oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}

func (c *GoogleClient) ExchangeCode(ctx context.Context, code string, redirectURL string) (Token, error) {
	oauthConfig, err := c.oauthConfig(redirectURL)
	if err != nil {
		return Token{}, err
	}
	token, err := oauthConfig.Exchange(ctx, strings.TrimSpace(code))
	if err != nil {
		return Token{}, fmt.Errorf("exchange Google Search Console OAuth code: %w", err)
	}
	return Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        ReadOnlyScope,
		Expiry:       token.Expiry.UTC(),
	}, nil
}

func (c *GoogleClient) ListProperties(ctx context.Context, token Token) ([]Property, error) {
	httpClient, err := c.httpClient(ctx, token)
	if err != nil {
		return nil, err
	}
	service, err := NewOfficialService(ctx, httpClient, c.config.APIBaseURL)
	if err != nil {
		return nil, err
	}
	response, err := service.Sites.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list Google Search Console properties: %w", err)
	}
	properties := make([]Property, 0, len(response.SiteEntry))
	for _, site := range response.SiteEntry {
		if site == nil || strings.TrimSpace(site.SiteUrl) == "" {
			continue
		}
		properties = append(properties, Property{
			URI:             strings.TrimSpace(site.SiteUrl),
			PermissionLevel: strings.TrimSpace(site.PermissionLevel),
		})
	}
	return properties, nil
}

func (c *GoogleClient) QuerySearchAnalytics(ctx context.Context, token Token, query SearchAnalyticsQuery) ([]SearchAnalyticsRow, error) {
	httpClient, err := c.httpClient(ctx, token)
	if err != nil {
		return nil, err
	}
	service, err := NewOfficialService(ctx, httpClient, c.config.APIBaseURL)
	if err != nil {
		return nil, err
	}
	dimensions := upperDimensions(query.Dimensions)
	dataState := strings.ToUpper(defaultString(query.DataState, DataStateFinal))
	aggregationType := strings.ToUpper(defaultString(query.AggregationType, "auto"))
	rowLimit := query.RowLimit
	if rowLimit <= 0 {
		rowLimit = 25000
	}
	var rows []SearchAnalyticsRow
	startRow := int64(0)
	for {
		request := &searchconsoleapi.SearchAnalyticsQueryRequest{
			StartDate:       searchConsoleDateString(query.StartDate),
			EndDate:         searchConsoleDateString(query.EndDate),
			Dimensions:      dimensions,
			DataState:       dataState,
			AggregationType: aggregationType,
			RowLimit:        rowLimit,
			StartRow:        startRow,
			Type:            "WEB",
		}
		response, err := service.Searchanalytics.Query(strings.TrimSpace(query.SiteURL), request).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("query Google Search Console search analytics: %w", err)
		}
		for _, row := range response.Rows {
			if row == nil {
				continue
			}
			rows = append(rows, searchAnalyticsRowFromAPI(row, dimensions, response.ResponseAggregationType, dataState))
		}
		if int64(len(response.Rows)) < rowLimit {
			break
		}
		startRow += int64(len(response.Rows))
	}
	return rows, nil
}

func searchAnalyticsRowFromAPI(row *searchconsoleapi.ApiDataRow, dimensions []string, aggregationType, dataState string) SearchAnalyticsRow {
	result := SearchAnalyticsRow{
		Clicks:          int(row.Clicks),
		Impressions:     int(row.Impressions),
		CTR:             row.Ctr,
		Position:        row.Position,
		AggregationType: strings.ToLower(strings.TrimSpace(aggregationType)),
		DataState:       strings.ToLower(strings.TrimSpace(dataState)),
	}
	for i, dimension := range dimensions {
		if i >= len(row.Keys) {
			break
		}
		assignSearchAnalyticsDimension(&result, dimension, row.Keys[i])
	}
	return result
}

func assignSearchAnalyticsDimension(row *SearchAnalyticsRow, dimension, value string) {
	switch strings.ToLower(strings.TrimSpace(dimension)) {
	case "date":
		if parsed, err := time.Parse(time.DateOnly, strings.TrimSpace(value)); err == nil {
			row.Date = parsed
		}
	case "query":
		row.Query = strings.TrimSpace(value)
	case "page":
		row.Page = strings.TrimSpace(value)
	case "country":
		row.Country = strings.TrimSpace(value)
	case "device":
		row.Device = strings.TrimSpace(value)
	}
}

func searchConsoleDateString(value time.Time) string {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

func upperDimensions(dimensions []string) []string {
	out := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		if trimmed := strings.TrimSpace(dimension); trimmed != "" {
			out = append(out, strings.ToUpper(trimmed))
		}
	}
	return out
}

func (c *GoogleClient) httpClient(ctx context.Context, token Token) (*http.Client, error) {
	oauthConfig, err := c.oauthConfig("http://localhost/oauth2callback")
	if err != nil {
		return nil, err
	}
	tokenType := strings.TrimSpace(token.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return oauthConfig.Client(ctx, &oauth2.Token{
		AccessToken:  strings.TrimSpace(token.AccessToken),
		RefreshToken: strings.TrimSpace(token.RefreshToken),
		TokenType:    tokenType,
		Expiry:       token.Expiry,
	}), nil
}

func (c *GoogleClient) oauthConfig(redirectURL string) (*oauth2.Config, error) {
	clientID := strings.TrimSpace(c.config.ClientID)
	clientSecret := strings.TrimSpace(c.config.ClientSecret)
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("google search console OAuth credentials are not configured")
	}
	redirectURL = strings.TrimSpace(redirectURL)
	if redirectURL == "" {
		return nil, fmt.Errorf("google search console redirect URL is required")
	}

	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{ReadOnlyScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  defaultString(c.config.AuthURL, google.Endpoint.AuthURL),
			TokenURL: defaultString(c.config.TokenURL, google.Endpoint.TokenURL),
		},
	}, nil
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func NewOfficialService(ctx context.Context, httpClient *http.Client, apiBaseURL ...string) (*searchconsoleapi.Service, error) {
	opts := []option.ClientOption{}
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	if len(apiBaseURL) > 0 && strings.TrimSpace(apiBaseURL[0]) != "" {
		opts = append(opts, option.WithEndpoint(strings.TrimRight(strings.TrimSpace(apiBaseURL[0]), "/")+"/"))
	}
	service, err := searchconsoleapi.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create Google Search Console service: %w", err)
	}
	return service, nil
}
