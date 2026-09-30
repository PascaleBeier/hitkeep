package mcpserver

type listSitesOutput struct {
	Sites []mcpSite `json:"sites"`
}

type mcpSite struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id"`
	Domain            string `json:"domain"`
	OwnerEmail        string `json:"owner_email,omitempty"`
	DataRetentionDays int    `json:"data_retention_days"`
	CreatedAt         string `json:"created_at"`
}
