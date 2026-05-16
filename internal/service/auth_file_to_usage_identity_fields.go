package service

import (
	"strings"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/authfiles"
)

func resolveAuthFileProjectID(file authfiles.AuthFile) *string {
	switch strings.ToLower(strings.TrimSpace(file.Type)) {
	case "gemini", "gemini-cli", "gemini-cli-code-assist", "antigravity":
		return resolveGeminiCLIProjectID(file)
	default:
		return nil
	}
}

func resolveCodexAccountID(file authfiles.AuthFile) *string {
	var idTokenAccountID *string
	var idTokenAccountIDCamel *string
	if file.IDToken != nil {
		idTokenAccountID = file.IDToken.AccountID
		idTokenAccountIDCamel = file.IDToken.AccountIDCamel
	}
	return firstNonEmptyStringPtr(
		idTokenAccountID,
		idTokenAccountIDCamel,
	)
}

func resolveCodexPlanType(file authfiles.AuthFile) *string {
	var idTokenPlanType *string
	var idTokenPlanTypeCamel *string
	if file.IDToken != nil {
		idTokenPlanType = file.IDToken.PlanType
		idTokenPlanTypeCamel = file.IDToken.PlanTypeCamel
	}
	return firstNonEmptyStringPtr(
		idTokenPlanType,
		idTokenPlanTypeCamel,
	)
}

func resolveCodexActiveStart(file authfiles.AuthFile) *time.Time {
	if file.IDToken == nil {
		return nil
	}
	if file.IDToken.ActiveStart != nil {
		return file.IDToken.ActiveStart
	}
	return file.IDToken.ActiveStartCamel
}

func resolveCodexActiveUntil(file authfiles.AuthFile) *time.Time {
	if file.IDToken == nil {
		return nil
	}
	if file.IDToken.ActiveUntil != nil {
		return file.IDToken.ActiveUntil
	}
	return file.IDToken.ActiveUntilCamel
}

func resolveGeminiCLIProjectID(file authfiles.AuthFile) *string {
	return stringValue(file.ProjectID)
}

func firstNonEmptyStringPtr(values ...*string) *string {
	for _, value := range values {
		if value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*value)
		if trimmed == "" {
			continue
		}
		return &trimmed
	}
	return nil
}

func stringValue(value string) *string {
	return firstNonEmptyStringPtr(&value)
}
