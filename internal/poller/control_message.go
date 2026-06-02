package poller

import "strings"

const (
	redisControlNone = iota
	redisControlSupportRefresh
	redisControlRefresh
)

type RedisControlMessage struct {
	// IsControl 表示当前 raw JSON 只是一条 CPA 控制通知。
	IsControl bool
	// SupportRefresh 表示 CPA 已声明或已证明支持 refresh 通知。
	SupportRefresh bool
	// Refresh 表示 CPA 通知 metadata 相关配置已经变化。
	Refresh bool
}

func ClassifyRedisControlMessage(raw string) RedisControlMessage {
	// usage payload 必须有 request_id，看到它就直接放行，避免热路径 JSON 解析。
	if strings.Contains(raw, `"request_id"`) {
		return RedisControlMessage{}
	}
	// 无 request_id 的少量消息才进入轻量控制结构扫描。
	switch parseSingleTrueControlField(raw) {
	case redisControlSupportRefresh:
		// support_refresh=true 只表示后续可能收到 refresh 通知。
		return RedisControlMessage{IsControl: true, SupportRefresh: true}
	case redisControlRefresh:
		// refresh=true 本身也证明 CPA 支持 refresh 通知。
		return RedisControlMessage{IsControl: true, SupportRefresh: true, Refresh: true}
	default:
		// 未命中两个精确控制结构时保持 passthrough。
		return RedisControlMessage{}
	}
}

func parseSingleTrueControlField(raw string) int {
	// 从第一个非空白字符开始，避免 TrimSpace 分配。
	i := skipJSONWhitespace(raw, 0)
	// 控制消息必须是单个 JSON object。
	if i >= len(raw) || raw[i] != '{' {
		return redisControlNone
	}
	// 跳过 object 左括号。
	i++
	// 字段名前允许 JSON 空白。
	i = skipJSONWhitespace(raw, i)
	// 只接受 support_refresh 或 refresh 两个单字段 key。
	field, next := matchControlField(raw, i)
	// 未命中控制 key 时不再继续解析。
	if field == redisControlNone {
		return redisControlNone
	}
	// 移动到字段名之后的位置。
	i = next
	// 冒号前允许 JSON 空白。
	i = skipJSONWhitespace(raw, i)
	// 控制字段后必须紧跟冒号。
	if i >= len(raw) || raw[i] != ':' {
		return redisControlNone
	}
	// 跳过冒号。
	i++
	// true 前允许 JSON 空白。
	i = skipJSONWhitespace(raw, i)
	// 控制消息只接受 boolean true，不接受 false 或字符串。
	if !hasLiteralAt(raw, i, "true") {
		return redisControlNone
	}
	// 跳过 true 字面量。
	i += len("true")
	// true 后允许 JSON 空白。
	i = skipJSONWhitespace(raw, i)
	// 控制消息必须只有一个字段；看到逗号等其它内容就 passthrough。
	if i >= len(raw) || raw[i] != '}' {
		return redisControlNone
	}
	// 跳过 object 右括号。
	i++
	// 右括号后只允许 JSON 空白。
	i = skipJSONWhitespace(raw, i)
	// 后面还有其它字节时不是精确控制消息。
	if i != len(raw) {
		return redisControlNone
	}
	// 返回命中的控制字段类型。
	return field
}

func matchControlField(raw string, i int) (int, int) {
	// 先匹配更长的 support_refresh，避免和其它字段做无意义比较。
	if next, ok := matchJSONStringLiteral(raw, i, "support_refresh"); ok {
		return redisControlSupportRefresh, next
	}
	// 再匹配 refresh 控制字段。
	if next, ok := matchJSONStringLiteral(raw, i, "refresh"); ok {
		return redisControlRefresh, next
	}
	// 未命中控制字段。
	return redisControlNone, i
}

func matchJSONStringLiteral(raw string, i int, value string) (int, bool) {
	// JSON object key 必须以双引号开始。
	if i >= len(raw) || raw[i] != '"' {
		return i, false
	}
	// 跳过起始双引号。
	start := i + 1
	// end 是期望字段名结束的位置。
	end := start + len(value)
	// 字段名长度不足时直接失败。
	if end >= len(raw) {
		return i, false
	}
	// 字段名内容必须精确等于目标控制字段。
	if raw[start:end] != value {
		return i, false
	}
	// 字段名后必须是结束双引号。
	if raw[end] != '"' {
		return i, false
	}
	// 返回结束双引号之后的位置。
	return end + 1, true
}

func hasLiteralAt(raw string, i int, literal string) bool {
	// 字面量长度不足时直接失败。
	if i+len(literal) > len(raw) {
		return false
	}
	// 控制消息只需要比较 true 这一个字面量。
	return raw[i:i+len(literal)] == literal
}

func skipJSONWhitespace(raw string, i int) int {
	// 只跳过 JSON 标准空白字符，避免误接受其它不可见字符。
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			// 当前字节是 JSON 空白，继续向后扫描。
			i++
		default:
			// 第一个非空白字符即为调用方继续解析的位置。
			return i
		}
	}
	// 全部扫描完时返回 len(raw)。
	return i
}
