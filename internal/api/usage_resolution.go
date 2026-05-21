package api

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

// loadUsageResolutionData 为 Request Events 和 Credentials 加载 source 解析所需的全部 usage identities（含软删除）。
// Credentials 端 resolver 会把活跃/已删除分流到独立索引以保留历史可见性，Request Events 下拉自带 IsDeleted 过滤。
func loadUsageResolutionData(
	c *gin.Context,
	usageIdentityProvider service.UsageIdentityProvider,
) ([]entities.UsageIdentity, error) {
	if usageIdentityProvider == nil {
		return []entities.UsageIdentity{}, nil
	}

	return usageIdentityProvider.ListUsageIdentities(c.Request.Context())
}
