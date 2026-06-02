package poller

import (
	"context"
	"fmt"
	"time"

	"cpa-usage-keeper/internal/cpa"
)

type HTTPPullSource struct {
	// client 封装 CPA 管理 HTTP 接口调用。
	client *cpa.Client
	// batchSize 控制每次从 HTTP usage queue 拉取的最大数量。
	batchSize int
}

func NewHTTPPullSource(baseURL, managementKey string, timeout time.Duration, tlsSkipVerify bool, batchSize int) *HTTPPullSource {
	// HTTP source 只构造 client，不主动请求 CPA。
	return &HTTPPullSource{
		// CPA client 内部持有 baseURL、managementKey、timeout 和 TLS 配置。
		client: cpa.NewClient(baseURL, managementKey, timeout, tlsSkipVerify),
		// batchSize 与 Redis pull 使用同一批量配置，保持 ingest 速度一致。
		batchSize: batchSize,
	}
}

func (s *HTTPPullSource) Pull(ctx context.Context) ([]string, error) {
	if s == nil || s.client == nil {
		// client 缺失说明 app wiring 有问题。
		return nil, fmt.Errorf("http pull source client is nil")
	}
	// 通过 CPA 管理接口读取 usage queue；这里只拿 raw payload，不做 decode。
	result, err := s.client.FetchUsageQueue(ctx, s.batchSize)
	if err != nil {
		// HTTP/network/API 错误交给 runner 决定退避或记录。
		return nil, err
	}
	// 预分配 payload 数量大小；source 层保留 null，避免破坏 runner 的满批判断。
	messages := make([]string, 0, len(result.Payload))
	for _, item := range result.Payload {
		// HTTP 返回的是 json.RawMessage；先按 byte 处理空值，减少热路径 string 分配。
		trimmed := httpRawUsageMessage(item)
		// 空 payload 和 null 交给统一 writer 过滤，保证本函数返回数量等于远端 payload 数量。
		messages = append(messages, trimmed)
	}
	// 返回 HTTP payload 的 raw messages，后续由 writer 决定哪些可以入 inbox。
	return messages, nil
}

func httpRawUsageMessage(item []byte) string {
	// 只裁剪 JSON 标准空白，和上游 RawMessage 的合法外层空白保持一致。
	start, end := trimJSONRawMessageBounds(item)
	if start == end {
		// 全空白 payload 用空字符串占位，保留批量计数且避免分配。
		return ""
	}
	if isRawJSONNull(item[start:end]) {
		// null payload 用常量字符串占位，后续 writer 会统一丢弃。
		return "null"
	}
	// 普通 usage/control payload 需要转成 string，交给后续 writer 分类或落库。
	return string(item[start:end])
}

func trimJSONRawMessageBounds(item []byte) (int, int) {
	// start 指向第一个非 JSON 空白字节。
	start := 0
	for start < len(item) && isJSONWhitespaceByte(item[start]) {
		// 跳过 RawMessage 前置 JSON 空白。
		start++
	}
	// end 指向最后一个非 JSON 空白字节之后。
	end := len(item)
	for end > start && isJSONWhitespaceByte(item[end-1]) {
		// 跳过 RawMessage 后置 JSON 空白。
		end--
	}
	// 返回半开区间，避免创建中间 slice。
	return start, end
}

func isJSONWhitespaceByte(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t':
		// JSON 只定义这四类外层空白。
		return true
	default:
		// 其它字节由 JSON payload 原样保留。
		return false
	}
}

func isRawJSONNull(item []byte) bool {
	// null 是固定四字节字面量，逐字节比较避免临时 string。
	return len(item) == len("null") && item[0] == 'n' && item[1] == 'u' && item[2] == 'l' && item[3] == 'l'
}
