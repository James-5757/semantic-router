package semanticrouter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DBLoggerConfig 数据库日志配置
type DBLoggerConfig struct {
	Driver          string // mysql, postgres, sqlite
	DSN             string // 数据库连接字符串
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// NewDBLogger 创建数据库日志记录器
func NewDBLogger(config *DBLoggerConfig) (RoutingDecisionLogger, error) {
	db, err := sql.Open(config.Driver, config.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DBLogger{
		db:     db,
		driver: config.Driver,
		mu:     sync.RWMutex{},
	}, nil
}

// DBLogger 数据库实现的日志记录器
type DBLogger struct {
	db     *sql.DB
	driver string
	mu     sync.RWMutex
}

// LogDecision 记录路由决策到数据库
func (l *DBLogger) LogDecision(decision *CombinedRouteDecision, requestID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	query := `
		INSERT INTO routing_decision_log (
			request_id, task_type, modality, preferred_pool, preferred_tier,
			matched_rule, tier_rule, confidence, requires_file_parsing,
			model_requested, model_resolved, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := l.db.ExecContext(context.Background(), l.bindQuery(query),
		requestID,
		string(decision.Semantic.TaskType),
		string(decision.Semantic.Modality),
		string(decision.FinalPool),
		string(decision.Tier.PreferredTier),
		decision.Semantic.MatchedRule,
		decision.Tier.MatchedRule,
		decision.Semantic.Confidence*decision.Tier.Confidence,
		decision.RequiresFileParsing,
		"", // model_requested - 可后续补充
		"", // model_resolved - 可后续补充
		decision.Timestamp,
	)

	return err
}

func (l *DBLogger) bindQuery(query string) string {
	if l.driver != "postgres" && l.driver != "pgx" {
		return query
	}
	for index := 1; strings.Contains(query, "?"); index++ {
		query = strings.Replace(query, "?", fmt.Sprintf("$%d", index), 1)
	}
	return query
}

func (l *DBLogger) LogRoutingDecision(entry *RoutingDecisionLogEntry) error {
	if entry == nil {
		return nil
	}

	matchedRules, _ := json.Marshal(entry.MatchedRules)
	semanticScores, _ := json.Marshal(entry.SemanticScores)
	createdAt := entry.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	query := `
		INSERT INTO routing_decision_log (
			request_id, api_key_id, group_id, prompt_hash, preferred_pool,
			preferred_tier, task_type, modality, matched_rule, tier_rule,
			confidence, matched_rules,
			semantic_scores, model_ranking_json, final_decision_source, fallback_reason,
			selected_account_id, selected_model, scheduler_layer,
			old_scheduler_account_id, requires_file_parsing, model_requested,
			model_resolved, shadow_latency_ms, old_selected_account_id,
			new_suggested_account_id, old_selected_model, new_suggested_model,
			old_selected_pool, new_suggested_pool, is_agree, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := l.db.ExecContext(context.Background(), l.bindQuery(query),
		entry.RequestID,
		entry.APIKeyID,
		entry.GroupID,
		entry.PromptHash,
		string(entry.PreferredPool),
		string(entry.PreferredTier),
		string(entry.TaskType),
		"",
		"",
		"",
		entry.Confidence,
		string(matchedRules),
		string(semanticScores),
		entry.ModelRankingJSON,
		entry.FinalDecisionSource,
		entry.FallbackReason,
		entry.SelectedAccountID,
		entry.SelectedModel,
		entry.SchedulerLayer,
		entry.OldSchedulerAccountID,
		false,
		"",
		entry.SelectedModel,
		entry.ShadowLatencyMs,
		entry.OldSelectedAccountID,
		entry.NewSuggestedAccountID,
		entry.OldSelectedModel,
		entry.NewSuggestedModel,
		entry.OldSelectedPool,
		entry.NewSuggestedPool,
		entry.IsAgree,
		createdAt,
	)
	return err
}

// GetRecentDecisions 从数据库获取最近的路由决策
func (l *DBLogger) GetRecentDecisions(limit int) ([]*RoutingLogEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, request_id, group_id, task_type, modality, preferred_pool,
			   preferred_tier, matched_rule, tier_rule, confidence,
			   requires_file_parsing, model_requested, model_resolved, created_at
		FROM routing_decision_log
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := l.db.QueryContext(context.Background(), l.bindQuery(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*RoutingLogEntry
	for rows.Next() {
		entry := &RoutingLogEntry{}
		var taskType, modality, pool, tier string

		err := rows.Scan(
			&entry.ID,
			&entry.RequestID,
			&entry.GroupID,
			&taskType,
			&modality,
			&pool,
			&tier,
			&entry.MatchedRule,
			&entry.TierRule,
			&entry.Confidence,
			&entry.RequiresFileParsing,
			&entry.ModelRequested,
			&entry.ModelResolved,
			&entry.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		entry.TaskType = TaskType(taskType)
		entry.Modality = Modality(modality)
		entry.PreferredPool = PreferredPool(pool)
		entry.PreferredTier = PreferredTier(tier)
		results = append(results, entry)
	}

	return results, rows.Err()
}

// GetStats 从数据库获取统计信息
func (l *DBLogger) GetStats() *RoutingStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := &RoutingStats{
		PoolCounts:     make(map[PreferredPool]int),
		TierCounts:     make(map[PreferredTier]int),
		TaskTypeCounts: make(map[TaskType]int),
	}

	// 总数
	l.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM routing_decision_log").
		Scan(&stats.TotalDecisions)

	// 按池统计
	rows, _ := l.db.QueryContext(context.Background(),
		"SELECT preferred_pool, COUNT(*) FROM routing_decision_log GROUP BY preferred_pool")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var pool string
			var count int
			rows.Scan(&pool, &count)
			stats.PoolCounts[PreferredPool(pool)] = count
		}
	}

	// 按 Tier 统计
	rows, _ = l.db.QueryContext(context.Background(),
		"SELECT preferred_tier, COUNT(*) FROM routing_decision_log GROUP BY preferred_tier")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var tier string
			var count int
			rows.Scan(&tier, &count)
			stats.TierCounts[PreferredTier(tier)] = count
		}
	}

	// 按任务类型统计
	rows, _ = l.db.QueryContext(context.Background(),
		"SELECT task_type, COUNT(*) FROM routing_decision_log GROUP BY task_type")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var taskType string
			var count int
			rows.Scan(&taskType, &count)
			stats.TaskTypeCounts[TaskType(taskType)] = count
		}
	}

	// 最近一小时统计
	l.db.QueryRowContext(context.Background(),
		l.bindQuery("SELECT COUNT(*) FROM routing_decision_log WHERE created_at > ?"),
		time.Now().Add(-time.Hour)).Scan(&stats.LastHourDecisions)

	return stats
}

// Close 关闭数据库连接
func (l *DBLogger) Close() error {
	return l.db.Close()
}

// Ping 检查数据库连接
func (l *DBLogger) Ping() error {
	return l.db.Ping()
}

// 确保 DBLogger 实现接口
var _ RoutingDecisionLogger = (*DBLogger)(nil)
var _ RoutingDecisionLogWriter = (*DBLogger)(nil)
