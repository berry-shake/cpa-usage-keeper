package authfiles

import "time"

// AuthFilesResponse 是 CPA /management/auth-files 响应 DTO。
type AuthFilesResponse struct {
	Files []AuthFile `json:"files"`
}

// AuthFile 是 CPA /management/auth-files 中单个 auth file 的原始响应 DTO。
type AuthFile struct {
	AuthIndex   string           `json:"auth_index"`
	Name        string           `json:"name"`
	Email       string           `json:"email"`
	Type        string           `json:"type"`
	Provider    string           `json:"provider"`
	Label       string           `json:"label"`
	Status      string           `json:"status"`
	Source      string           `json:"source"`
	Disabled    bool             `json:"disabled"`
	Unavailable bool             `json:"unavailable"`
	RuntimeOnly bool             `json:"runtime_only"`
	IDToken     *AuthFileIDToken `json:"id_token"`
}

// AuthFileIDToken 是 Codex auth file 的 id_token 订阅元数据 DTO。
type AuthFileIDToken struct {
	AccountID   *string    `json:"chatgpt_account_id"`
	ActiveStart *time.Time `json:"chatgpt_subscription_active_start"`
	ActiveUntil *time.Time `json:"chatgpt_subscription_active_until"`
	PlanType    *string    `json:"plan_type"`
}
