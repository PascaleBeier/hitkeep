package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailer"
	"hitkeep/internal/reporting"
)

type reportCaptureDriver struct {
	subject   string
	htmlBody  string
	textBody  string
	messageID string
	headers   map[string]string
	sendErr   error
}

func (d *reportCaptureDriver) Send(_ []string, subject, htmlBody, textBody string) error {
	d.subject, d.htmlBody, d.textBody = subject, htmlBody, textBody
	return d.sendErr
}

func (d *reportCaptureDriver) SendWithHeaders(_ []string, subject, htmlBody, textBody, messageID string, headers map[string]string) error {
	d.subject, d.htmlBody, d.textBody, d.messageID, d.headers = subject, htmlBody, textBody, messageID, headers
	return d.sendErr
}

func (d *reportCaptureDriver) Close() error { return nil }

func TestCreateReportValidatesQuarterHourAndMailAvailability(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	site, err := store.CreateSite(context.Background(), userID, "report-handler.example.test")
	if err != nil {
		t.Fatal(err)
	}

	request := api.CreateReportRequest{
		Name: "Morning report", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{uuid.New()},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "08:10"},
		Status:   api.ReportStatusDraft,
	}
	response := performReportCreate(t, h, userID, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("off-grid schedule status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}

	request.Schedule.LocalTime = "08:15"
	request.Status = api.ReportStatusActive
	response = performReportCreate(t, h, userID, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("active report without mail status = %d, want %d", response.Code, http.StatusConflict)
	}

	request.Status = api.ReportStatusDraft
	response = performReportCreate(t, h, userID, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("draft report status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var report api.ReportDefinition
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Schedule.Timezone != "Europe/Berlin" || report.Schedule.LocalTime != "08:15" {
		t.Fatalf("stored schedule = %+v", report.Schedule)
	}
	if len(report.Recipients) != 1 || report.Recipients[0].UserID == nil || *report.Recipients[0].UserID != userID {
		t.Fatalf("personal recipients = %+v, want current user only", report.Recipients)
	}
}

func TestCreateTeamReportRequiresRecipientAccessToEverySite(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	memberID, err := store.CreateUser(ctx, "report-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.CreateTenant(ctx, ownerID, "Reporting Team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, team.ID, memberID, "member", ownerID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, ownerID, "team-report.example.test")
	if err != nil {
		t.Fatal(err)
	}

	request := api.CreateReportRequest{
		Name: "Team portfolio", Scope: api.ReportScopeTeam, TenantID: &team.ID, Preset: api.ReportPresetPortfolioDigest,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{memberID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "UTC", LocalTime: "08:00", WeeklyDay: new(1)},
		Status:   api.ReportStatusDraft,
	}
	response := performReportCreate(t, h, ownerID, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("recipient without site access status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if err := store.AddSiteMember(ctx, site.ID, memberID, "viewer", ownerID); err != nil {
		t.Fatal(err)
	}
	response = performReportCreate(t, h, ownerID, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("authorized team report status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
}

func TestExternalReportRecipientsRequireTeamManagerAndCanonicalizeMembers(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	memberID, err := store.CreateUser(ctx, "canonical-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.CreateTenant(ctx, ownerID, "External Reporting Team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, team.ID, memberID, "member", ownerID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, ownerID, "external-handler.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddSiteMember(ctx, site.ID, memberID, "viewer", ownerID); err != nil {
		t.Fatal(err)
	}

	personal := api.CreateReportRequest{
		Name: "Personal external", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{ownerID}, ExternalRecipientEmails: []string{"client@example.test"},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"}, Status: api.ReportStatusDraft,
	}
	if response := performReportCreate(t, h, ownerID, personal); response.Code != http.StatusBadRequest {
		t.Fatalf("personal external status = %d, want 400", response.Code)
	}

	teamRequest := personal
	teamRequest.Name = "Canonical member"
	teamRequest.Scope = api.ReportScopeTeam
	teamRequest.TenantID = &team.ID
	teamRequest.RecipientUserIDs = nil
	teamRequest.ExternalRecipientEmails = []string{" CANONICAL-MEMBER@EXAMPLE.TEST "}
	response := performReportCreate(t, h, ownerID, teamRequest)
	if response.Code != http.StatusCreated {
		t.Fatalf("canonical report status = %d: %s", response.Code, response.Body.String())
	}
	var report api.ReportDefinition
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if len(report.Recipients) != 1 || report.Recipients[0].Kind != api.ReportRecipientKindMember || report.Recipients[0].UserID == nil || *report.Recipients[0].UserID != memberID {
		t.Fatalf("canonical recipients = %+v", report.Recipients)
	}

	teamRequest.Name = "Member cannot manage"
	teamRequest.RecipientUserIDs = []uuid.UUID{memberID}
	teamRequest.ExternalRecipientEmails = []string{"client@example.test"}
	if response := performReportCreate(t, h, memberID, teamRequest); response.Code != http.StatusForbidden {
		t.Fatalf("member external status = %d, want 403: %s", response.Code, response.Body.String())
	}

	teamRequest.Name = "Too many recipients"
	teamRequest.RecipientUserIDs = []uuid.UUID{ownerID}
	teamRequest.ExternalRecipientEmails = make([]string, 26)
	for index := range teamRequest.ExternalRecipientEmails {
		teamRequest.ExternalRecipientEmails[index] = fmt.Sprintf("client-%d@example.test", index)
	}
	if response := performReportCreate(t, h, ownerID, teamRequest); response.Code != http.StatusBadRequest {
		t.Fatalf("recipient cap status = %d, want 400", response.Code)
	}
}

func TestExternalReportRecipientsRequirePaidCloudPlan(t *testing.T) {
	h, store, _ := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	managerID, err := store.CreateUser(ctx, "cloud-report-manager@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	memberID, err := store.CreateUser(ctx, "cloud-report-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.CreateTenant(ctx, managerID, "Cloud reporting team", "")
	if err != nil {
		t.Fatal(err)
	}
	teamID := team.ID
	if err := store.SetActiveTenantID(ctx, managerID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, teamID, memberID, "member", managerID); err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "cloud-plan-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddSiteMember(ctx, site.ID, memberID, "viewer", managerID); err != nil {
		t.Fatal(err)
	}

	h.ctx.Config.CloudHosted = true
	h.ctx.Entitlements = entitlements.NewStaticProvider(
		entitlements.Entitlements{},
		entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"},
	)
	request := api.CreateReportRequest{
		Name: "Cloud client report", Scope: api.ReportScopeTeam, TenantID: &teamID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{managerID},
		ExternalRecipientEmails: []string{"client@example.test"},
		Schedule:                api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:                  api.ReportStatusDraft,
	}
	response := performReportCreate(t, h, managerID, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"plan_upgrade_required"`) {
		t.Fatalf("Free external recipient status = %d, want 403 plan_upgrade_required: %s", response.Code, response.Body.String())
	}

	request.Name = "Canonical member on Free"
	request.RecipientUserIDs = nil
	request.ExternalRecipientEmails = []string{" CLOUD-REPORT-MEMBER@EXAMPLE.TEST "}
	response = performReportCreate(t, h, managerID, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("member canonicalization on Free status = %d, want 201: %s", response.Code, response.Body.String())
	}

	h.ctx.Entitlements = entitlements.NewStaticProvider(
		entitlements.Entitlements{AllowExternalReportRecipients: true},
		entitlements.PlanInfo{Code: "pro", Name: "Pro"},
	)
	request.Name = "Pro client report"
	request.RecipientUserIDs = []uuid.UUID{managerID}
	request.ExternalRecipientEmails = []string{"client@example.test"}
	response = performReportCreate(t, h, managerID, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("Pro external recipient status = %d, want 201: %s", response.Code, response.Body.String())
	}
	var report api.ReportDefinition
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	var external api.ReportRecipient
	for _, recipient := range report.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			external = recipient
		}
	}
	if external.ID == uuid.Nil {
		t.Fatal("Pro report did not retain its external recipient")
	}

	h.ctx.Mailer = mailer.NewWithDriver(&reportCaptureDriver{}, h.ctx.Config)
	h.ctx.Entitlements = entitlements.NewStaticProvider(
		entitlements.Entitlements{},
		entitlements.PlanInfo{Code: entitlements.PlanCodeFree, Name: "Free"},
	)
	paused := api.ReportStatusPaused
	update := performReportUpdate(t, h, managerID, report.ID, api.UpdateReportRequest{Status: &paused})
	if update.Code != http.StatusOK {
		t.Fatalf("Free pause with retained external recipient status = %d, want 200: %s", update.Code, update.Body.String())
	}
	portfolio := api.ReportPresetPortfolioDigest
	update = performReportUpdate(t, h, managerID, report.ID, api.UpdateReportRequest{Preset: &portfolio})
	if update.Code != http.StatusForbidden || !strings.Contains(update.Body.String(), `"code":"plan_upgrade_required"`) {
		t.Fatalf("Free material scope update status = %d, want 403 plan_upgrade_required: %s", update.Code, update.Body.String())
	}
	resendRequest := withTestUser(httptest.NewRequest(http.MethodPost, "/api/reports/"+report.ID.String()+"/recipients/"+external.ID.String()+"/confirmation", nil), managerID)
	resendRequest.SetPathValue("report_id", report.ID.String())
	resendRequest.SetPathValue("recipient_id", external.ID.String())
	resend := httptest.NewRecorder()
	h.handleResendReportRecipientConfirmation().ServeHTTP(resend, resendRequest)
	if resend.Code != http.StatusForbidden || !strings.Contains(resend.Body.String(), `"code":"plan_upgrade_required"`) {
		t.Fatalf("Free confirmation resend status = %d, want 403 plan_upgrade_required: %s", resend.Code, resend.Body.String())
	}
}

func TestReportRecipientConfirmationGETDoesNotMutateAndPOSTIsSingleUse(t *testing.T) {
	h, store, managerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "confirmation-handler.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, managerID, api.CreateReportRequest{
		Name: "Consent handler", Scope: api.ReportScopeTeam, TenantID: &tenantID, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{managerID}, ExternalRecipientEmails: []string{"client@example.test"},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"}, Status: api.ReportStatusDraft,
	}, h.ctx.Config.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	external := report.Recipients[0]
	if external.Kind != api.ReportRecipientKindExternal {
		external = report.Recipients[1]
	}
	prepared, err := store.PrepareReportRecipientConfirmation(ctx, report.ID, external.ID, "en", time.Now().UTC(), false)
	if err != nil {
		t.Fatal(err)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/report-recipient-confirmations/"+prepared.Token, nil)
	get.SetPathValue("opaque_token", prepared.Token)
	getResponse := httptest.NewRecorder()
	h.handleGetReportRecipientConfirmation().ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), site.Domain) {
		t.Fatalf("confirmation GET = %d %s", getResponse.Code, getResponse.Body.String())
	}
	afterGET, err := store.GetReportDefinition(ctx, report.ID)
	if err != nil || externalRecipientStatusForHandler(afterGET) != api.ReportRecipientStatusPending {
		t.Fatalf("GET mutated confirmation: %+v err=%v", afterGET, err)
	}

	body := bytes.NewBufferString(`{"action":"confirm"}`)
	post := httptest.NewRequest(http.MethodPost, "/api/report-recipient-confirmations/"+prepared.Token, body)
	post.SetPathValue("opaque_token", prepared.Token)
	postResponse := httptest.NewRecorder()
	h.handleConfirmReportRecipient().ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusNoContent {
		t.Fatalf("confirmation POST = %d %s", postResponse.Code, postResponse.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/api/report-recipient-confirmations/"+prepared.Token, bytes.NewBufferString(`{"action":"confirm"}`))
	second.SetPathValue("opaque_token", prepared.Token)
	secondResponse := httptest.NewRecorder()
	h.handleConfirmReportRecipient().ServeHTTP(secondResponse, second)
	if secondResponse.Code != http.StatusBadRequest {
		t.Fatalf("reused confirmation status = %d, want 400", secondResponse.Code)
	}
}

func TestExternalReportInvitationFailureIsSafeAndResendable(t *testing.T) {
	h, store, managerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	driver := &reportCaptureDriver{sendErr: errors.New("raw provider 550 response")}
	h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)
	ctx := context.Background()
	if err := store.UpsertUserPreferences(ctx, managerID, api.UserPreferences{DefaultLocale: "de"}); err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, managerID, "smtp-recovery.example.test")
	if err != nil {
		t.Fatal(err)
	}
	response := performReportCreate(t, h, managerID, api.CreateReportRequest{
		Name: "SMTP recovery", Scope: api.ReportScopeTeam, TenantID: &tenantID,
		Preset: api.ReportPresetSiteSummary, SiteMode: api.ReportSiteModeSelected,
		SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{managerID},
		ExternalRecipientEmails: []string{"smtp-client@example.test"},
		Schedule:                api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:                  api.ReportStatusActive,
	})
	if response.Code != http.StatusCreated || strings.Contains(response.Body.String(), "raw provider") {
		t.Fatalf("failed invitation response = %d %s", response.Code, response.Body.String())
	}
	var report api.ReportDefinition
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	var external api.ReportRecipient
	for _, recipient := range report.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			external = recipient
		}
	}
	if external.Status != api.ReportRecipientStatusPending || external.InvitationState != "failed" {
		t.Fatalf("safe invitation failure = %+v", external)
	}
	var safeErrorCode, externalLocale string
	if err := store.DB().QueryRowContext(ctx,
		"SELECT confirmation_error_code, external_locale FROM report_recipients WHERE id = ?",
		external.ID,
	).Scan(&safeErrorCode, &externalLocale); err != nil || safeErrorCode != "smtp_send_failed" || externalLocale != "de" {
		t.Fatalf("safe invitation state code=%q locale=%q err=%v", safeErrorCode, externalLocale, err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"UPDATE report_recipients SET confirmation_sent_at = ? WHERE id = ?",
		time.Now().UTC().Add(-16*time.Minute), external.ID,
	); err != nil {
		t.Fatal(err)
	}
	driver.sendErr = nil
	request := withTestUser(httptest.NewRequest(http.MethodPost, "/api/reports/"+report.ID.String()+"/recipients/"+external.ID.String()+"/confirmation", nil), managerID)
	request.SetPathValue("report_id", report.ID.String())
	request.SetPathValue("recipient_id", external.ID.String())
	resend := httptest.NewRecorder()
	h.handleResendReportRecipientConfirmation().ServeHTTP(resend, request)
	if resend.Code != http.StatusAccepted {
		t.Fatalf("resend status = %d: %s", resend.Code, resend.Body.String())
	}
	refreshed, err := store.GetReportDefinition(ctx, report.ID)
	if err != nil || externalRecipientStatusForHandler(refreshed) != api.ReportRecipientStatusPending {
		t.Fatalf("resend did not restore pending state: %+v err=%v", refreshed, err)
	}
}

func externalRecipientStatusForHandler(report *api.ReportDefinition) api.ReportRecipientStatus {
	for _, recipient := range report.Recipients {
		if recipient.Kind == api.ReportRecipientKindExternal {
			return recipient.Status
		}
	}
	return ""
}

func TestReportUnsubscribeTokenOptsOutOnlyItsRecipient(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "unsubscribe-report.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, userID, api.CreateReportRequest{
		Name: "Unsubscribe report", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{userID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusActive,
	}, h.ctx.Config.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	token := reporting.UnsubscribeToken(h.ctx.Config.JWTSecret, report.ID, report.Recipients[0].ID)
	req := httptest.NewRequest(http.MethodPost, "/api/reports/unsubscribe/"+token, nil)
	req.SetPathValue("opaque_token", token)
	response := httptest.NewRecorder()
	h.handleUnsubscribeReport().ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("unsubscribe status = %d, want %d: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	after, err := store.GetReportDefinition(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Recipients[0].OptedOutAt == nil {
		t.Fatal("recipient was not opted out")
	}

	tampered := token + "x"
	req = httptest.NewRequest(http.MethodPost, "/api/reports/unsubscribe/"+tampered, nil)
	req.SetPathValue("opaque_token", tampered)
	response = httptest.NewRecorder()
	h.handleUnsubscribeReport().ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("tampered token status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestReportPreviewUsesTheActualLocalizedReportContent(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "preview-content.example.test")
	if err != nil {
		t.Fatal(err)
	}
	reqBody, err := json.Marshal(api.ReportPreviewRequest{Definition: api.CreateReportRequest{
		Name: "Internal report name", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{userID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusDraft,
	}})
	if err != nil {
		t.Fatal(err)
	}
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/reports/preview", bytes.NewReader(reqBody)), userID)
	response := httptest.NewRecorder()
	h.handlePreviewReport().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var preview api.ReportPreview
	if err := json.NewDecoder(response.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if preview.Subject == "Internal report name" || !strings.Contains(preview.Subject, site.Domain) {
		t.Fatalf("preview subject = %q, want rendered site-report subject", preview.Subject)
	}
	if preview.Suppressed {
		t.Fatal("site summary preview was unexpectedly suppressed")
	}
}

func TestReportTestSendSendsTheActualReportToTheCurrentUser(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()
	driver := &reportCaptureDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "test-send-content.example.test")
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.CreateReportDefinition(ctx, userID, api.CreateReportRequest{
		Name: "Internal test name", Scope: api.ReportScopePersonal, Preset: api.ReportPresetSiteSummary,
		SiteMode: api.ReportSiteModeSelected, SiteIDs: []uuid.UUID{site.ID}, RecipientUserIDs: []uuid.UUID{userID},
		Schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:00"},
		Status:   api.ReportStatusDraft,
	}, h.ctx.Config.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/reports/"+report.ID.String()+"/test-send", nil), userID)
	req.SetPathValue("report_id", report.ID.String())
	response := httptest.NewRecorder()
	h.handleTestSendReport().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("test-send status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if driver.subject == "Internal test name" || !strings.Contains(driver.subject, site.Domain) {
		t.Fatalf("test-send subject = %q, want rendered site-report subject", driver.subject)
	}
	if !strings.Contains(driver.htmlBody, site.Domain) || !strings.Contains(driver.textBody, site.Domain) {
		t.Fatal("test-send did not render matching HTML and text report content")
	}
	if driver.messageID == "" || driver.headers["Auto-Submitted"] != "auto-generated" {
		t.Fatalf("test-send delivery metadata = message %q headers %+v", driver.messageID, driver.headers)
	}
}

func performReportCreate(t *testing.T, h *handler, userID uuid.UUID, request api.CreateReportRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(body)), userID)
	response := httptest.NewRecorder()
	h.handleCreateReport().ServeHTTP(response, req)
	return response
}

func performReportUpdate(t *testing.T, h *handler, userID, reportID uuid.UUID, request api.UpdateReportRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := withTestUser(httptest.NewRequest(http.MethodPatch, "/api/reports/"+reportID.String(), bytes.NewReader(body)), userID)
	req.SetPathValue("report_id", reportID.String())
	response := httptest.NewRecorder()
	h.handleUpdateReport().ServeHTTP(response, req)
	return response
}
