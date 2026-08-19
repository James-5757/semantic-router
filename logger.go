package semanticrouter

import (
	"sync"
	"time"
)

// InMemoryRoutingDecisionLogger 内存实现的路由决策日志记录器
// 用于测试和演示，生产环境建议使用数据库实现
type InMemoryRoutingDecisionLogger struct {
	mu       sync.RWMutex
	logs     []*RoutingLogEntry
	maxSize  int
}

// NewInMemoryRoutingDecisionLogger 创建内存日志记录器
func NewInMemoryRoutingDecisionLogger(maxSize int) *InMemoryRoutingDecisionLogger {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &InMemoryRoutingDecisionLogger{
		logs:    make([]*RoutingLogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// LogDecision 记录路由决策
func (l *InMemoryRoutingDecisionLogger) LogDecision(decision *CombinedRouteDecision, requestID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := &RoutingLogEntry{
		ID:                  requestID,
		RequestID:           requestID,
		Timestamp:           decision.Timestamp,
		TaskType:            decision.Semantic.TaskType,
		Modality:            decision.Semantic.Modality,
		PreferredPool:       decision.FinalPool,
		PreferredTier:       decision.Tier.PreferredTier,
		MatchedRule:         decision.Semantic.MatchedRule,
		TierRule:            decision.Tier.MatchedRule,
		Confidence:          decision.Semantic.Confidence * decision.Tier.Confidence,
		RequiresFileParsing: decision.RequiresFileParsing,
	}

	// 添加到日志末尾
	l.logs = append(l.logs, entry)

	// 如果超过最大尺寸，删除最早的记录
	if len(l.logs) > l.maxSize {
		l.logs = l.logs[len(l.logs)-l.maxSize:]
	}

	return nil
}

// GetRecentDecisions 获取最近的路由决策
func (l *InMemoryRoutingDecisionLogger) GetRecentDecisions(limit int) ([]*RoutingLogEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if limit <= 0 || limit > len(l.logs) {
		limit = len(l.logs)
	}

	// 返回最近的记录
	result := make([]*RoutingLogEntry, limit)
	copy(result, l.logs[len(l.logs)-limit:])
	return result, nil
}

// GetDecisionsByPool 获取指定账号池的路由决策
func (l *InMemoryRoutingDecisionLogger) GetDecisionsByPool(pool PreferredPool) []*RoutingLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*RoutingLogEntry, 0)
	for _, entry := range l.logs {
		if entry.PreferredPool == pool {
			result = append(result, entry)
		}
	}
	return result
}

// GetDecisionsByTaskType 获取指定任务类型的路由决策
func (l *InMemoryRoutingDecisionLogger) GetDecisionsByTaskType(taskType TaskType) []*RoutingLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*RoutingLogEntry, 0)
	for _, entry := range l.logs {
		if entry.TaskType == taskType {
			result = append(result, entry)
		}
	}
	return result
}

// GetStats 获取路由统计信息
func (l *InMemoryRoutingDecisionLogger) GetStats() *RoutingStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := &RoutingStats{
		TotalDecisions: len(l.logs),
		PoolCounts:     make(map[PreferredPool]int),
		TierCounts:     make(map[PreferredTier]int),
		TaskTypeCounts: make(map[TaskType]int),
	}

	now := time.Now()
	for _, entry := range l.logs {
		stats.PoolCounts[entry.PreferredPool]++
		stats.TierCounts[entry.PreferredTier]++
		stats.TaskTypeCounts[entry.TaskType]++

		// 统计过去小时
		if now.Sub(entry.Timestamp) < time.Hour {
			stats.LastHourDecisions++
		}
	}

	return stats
}

// RoutingStats 路由统计信息
type RoutingStats struct {
	TotalDecisions    int
	LastHourDecisions int
	PoolCounts        map[PreferredPool]int
	TierCounts        map[PreferredTier]int
	TaskTypeCounts    map[TaskType]int
}

// Clear 清除所有日志
func (l *InMemoryRoutingDecisionLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = make([]*RoutingLogEntry, 0, l.maxSize)
}

// Size 获取当前日志大小
func (l *InMemoryRoutingDecisionLogger) Size() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.logs)
}

// Close 关闭连接（内存版本无实际连接）
func (l *InMemoryRoutingDecisionLogger) Close() error {
	return nil
}

// Ping 检查连接（内存版本始终成功）
func (l *InMemoryRoutingDecisionLogger) Ping() error {
	return nil
}

// 确保接口实现
var _ RoutingDecisionLogger = (*InMemoryRoutingDecisionLogger)(nil)