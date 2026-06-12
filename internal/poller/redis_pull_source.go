package poller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"cpa-usage-keeper/internal/cpa"
	"github.com/sirupsen/logrus"
)

type RedisPullSource struct {
	// mu 保护首次探测后的 queue key 固化结果。
	mu sync.Mutex
	// clients 固定保存 usage/queue 两个兼容 Redis pull client，避免尝试其它 key。
	clients []redisPullQueueClient
	// selected 记录首次成功的 Redis queue key；一旦选定，后续不再双探测。
	selected *redisPullQueueClient
}

type redisPullQueueClient struct {
	// queueKey 是 CPA Redis LPOP 使用的实际 key，只能来自 usage/queue 候选列表。
	queueKey string
	// client 绑定 queueKey 执行一次性 Redis batch pull。
	client *cpa.RedisQueueClient
}

func NewRedisPullSource(opts cpa.RedisQueueOptions) *RedisPullSource {
	// Redis source 只构造候选 client，不主动连接 Redis。
	keys := redisPullQueueKeyCandidates()
	// clients 预分配成候选 key 数量，保证后续遍历只覆盖 usage/queue 两个固定入口。
	clients := make([]redisPullQueueClient, 0, len(keys))
	// 按固定顺序构造候选 client，顺序本身就是兼容策略的一部分。
	for _, key := range keys {
		// 每个候选都复制一份 options，避免候选 key 之间共享可变配置。
		candidateOpts := opts
		// QueueKey 只在候选 client 内部使用，不再从外部配置传入。
		candidateOpts.QueueKey = key
		// 每个候选 client 绑定自己的 key，后续成功时可以固化并记录来源。
		clients = append(clients, redisPullQueueClient{
			// queueKey 保存 CPA 实际 Redis key，用于成功后生成 redis_pull:<key>。
			queueKey: key,
			// client 仍复用底层 RedisQueueClient，不把 RESP 细节放进状态机。
			client: cpa.NewRedisQueueClientWithOptions(candidateOpts),
		})
	}
	// 构造阶段不探测远端，避免应用启动被 CPA 网络状态阻塞。
	return &RedisPullSource{clients: clients}
}

func (s *RedisPullSource) Pull(ctx context.Context) ([]string, error) {
	if s == nil {
		// Redis client 缺失时返回 Redis 相关错误，让 runner 走 fallback。
		return nil, cpa.ErrRedisQueueAuth
	}
	// PopUsage 现在是 Redis-only；是否降级 HTTP 完全由 RedisIngestRunner 决定。
	s.mu.Lock()
	if s.selected != nil {
		// 已固化路径只需要锁内复制 client，避免 Redis I/O 阻塞 SourceName 读取来源。
		selected := s.selected
		// 释放 RedisPullSource 状态锁后再做网络拉取；runner 的 opMu 仍负责串行化真实消费。
		s.mu.Unlock()
		// 首次成功后只使用已固化 key，不再在 usage/queue 之间循环尝试。
		return selected.client.PopUsage(ctx)
	}
	// 未固化前锁覆盖整个探测过程，避免并发 goroutine 分别消费 usage/queue。
	defer s.mu.Unlock()

	// pullErrs 收集本轮最多两个候选错误，最终返回给 runner 做降级判断。
	var pullErrs []error
	for i := range s.clients {
		// 未选定前只按候选顺序各尝试一次，最多 usage/queue 两次。
		candidate := &s.clients[i]
		// 每个候选只执行一次真实 LPOP，避免 CPA 不支持 key 时在内部卡住。
		messages, err := candidate.client.PopUsage(ctx)
		if err == nil {
			// 首个成功 key 立刻固化，后续所有拉取都走同一个 CPA key。
			s.selected = candidate
			// 选定日志只打一遍，帮助现场确认到底兼容到了 usage 还是 queue。
			logrus.WithField("redis_key", candidate.queueKey).Info("redis usage key selected")
			// 返回本次成功拉到的消息；空批次也算 key 可用并会固化。
			return messages, nil
		}
		// 错误里带上候选 key，避免用户只看到底层 unsupported channel 而不知道尝试顺序。
		pullErrs = append(pullErrs, fmt.Errorf("%s: %w", candidate.queueKey, err))
		if !redisPullCanTryNextQueueKey(err) {
			// 只有 key 不支持类错误才尝试下一个 key；认证/网络/超时直接交给 runner 降级。
			break
		}
	}
	// 到这里表示没有候选成功；errors.Join 保留两次尝试的完整上下文。
	return nil, errors.Join(pullErrs...)
}

func (s *RedisPullSource) SourceName() string {
	if s == nil {
		// nil source 只能返回默认来源名，避免调用方还要额外判空。
		return RedisIngestSourceRedisPull
	}
	s.mu.Lock()
	// SourceName 与 Pull 共用锁，保证读取到的 selected 与实际消费 key 一致。
	defer s.mu.Unlock()
	if s.selected == nil || strings.TrimSpace(s.selected.queueKey) == "" {
		// 尚未成功探测前使用默认 usage 来源；成功拉取后 runner 会再次读取精确值。
		return RedisIngestSourceRedisPull
	}
	// 成功后把实际 key 写进 source，形成 redis_pull:usage 或 redis_pull:queue。
	return RedisIngestSourceRedisPullPrefix + strings.TrimSpace(s.selected.queueKey)
}

func redisPullQueueKeyCandidates() []string {
	// 新版 CPA 使用 usage，旧版 CPA 使用 queue；顺序必须保持 usage -> queue。
	return []string{cpa.ManagementUsageQueueKey, cpa.ManagementUsageLegacyQueueKey}
}

func redisPullCanTryNextQueueKey(err error) bool {
	if err == nil {
		// 没有错误时不存在 fallback 判定。
		return false
	}
	// CPA 不同版本的错误文本可能叫 channel 或 queue，这里只识别 key 不支持类错误。
	message := strings.ToLower(err.Error())
	// 其它错误如认证失败、网络失败、RESP 解析失败都不能触发第二 key 尝试。
	return strings.Contains(message, "unsupported channel") || strings.Contains(message, "unsupported queue")
}
