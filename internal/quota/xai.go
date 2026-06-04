package quota

import (
	"context"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
)

type xaiProvider struct {
	caller ManagementAPICaller
	config APICallConfig
}

func NewXAIProvider(caller ManagementAPICaller, config APICallConfig) ProviderHandler {
	return xaiProvider{caller: caller, config: config}
}

func (p xaiProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	// xAI billing 只需要当前 auth_index 调用 Grok billing endpoint，金额字段保留 cents 口径交给前端展示。
	response, err := p.caller.CallManagementAPI(ctx, apicall.Request{
		AuthIndex: input.Identity.Identity,
		Method:    p.config.Method,
		URL:       p.config.URL,
		Header:    copyHeaders(p.config.Headers),
	})
	if err != nil {
		return ProviderOutput{}, err
	}
	billing, err := parseXAIBillingPayload(response)
	if err != nil {
		return ProviderOutput{}, err
	}
	return ProviderOutput{Provider: "xai", Result: XAIResult{Billing: billing}}, nil
}
