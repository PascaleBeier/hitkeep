//go:build billing

package entitlements

import (
	"strings"

	"hitkeep/config"
)

func NewProvider(conf *config.Config) Provider {
	if conf == nil || !conf.CloudHosted {
		return NewDefaultProvider()
	}

	planEntitlements := CloudPlanEntitlements(strings.TrimSpace(conf.CloudPlanCode))
	if planEntitlements == nil {
		planEntitlements = CloudPlanEntitlements(PlanCodeFree)
	}

	return NewStaticProvider(Entitlements{
		MaxTeams:                      conf.CloudMaxTeams,
		MaxSitesPerTeam:               conf.CloudMaxSitesPerTeam,
		MaxRetentionDays:              conf.CloudMaxRetentionDays,
		MaxTeamMembers:                conf.CloudMaxTeamMembers,
		MaxAskAIAnswersPerDay:         planEntitlements.MaxAskAIAnswersPerDay,
		AllowSSO:                      conf.CloudAllowSSO,
		AllowCustomBranding:           conf.CloudAllowCustomBranding,
		AllowExternalReportRecipients: strings.TrimSpace(conf.CloudPlanCode) != "" && strings.TrimSpace(conf.CloudPlanCode) != PlanCodeFree,
	}, PlanInfo{
		Code:       conf.CloudPlanCode,
		Name:       conf.CloudPlanName,
		UpgradeURL: conf.CloudUpgradeURL,
		SupportURL: conf.CloudSupportURL,
	})
}
