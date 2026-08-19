package semanticrouter

// Migration 用于创建路由决策日志表的 SQL migration
// 可以将此迁移脚本集成到现有的 ent migrate 系统中

const RoutingDecisionLogTableName = "routing_decision_log"

// RoutingDecisionLogMigration 返回创建路由决策日志表的 SQL
func RoutingDecisionLogMigration() string {
	return `
-- 路由决策日志表
-- 用于记录所有路由决策，方便后续评估和分析
CREATE TABLE IF NOT EXISTS ` + RoutingDecisionLogTableName + ` (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    request_id      VARCHAR(64) NOT NULL COMMENT '请求ID',
    api_key_id      BIGINT DEFAULT 0 COMMENT 'API Key ID',
    group_id        BIGINT COMMENT '分组ID',
    prompt_hash     VARCHAR(64) COMMENT 'Prompt SHA-256 hash',
    task_type       VARCHAR(32) NOT NULL COMMENT '任务类型',
    modality        VARCHAR(32) NOT NULL COMMENT '模态类型',
    preferred_pool  VARCHAR(32) NOT NULL COMMENT '首选账号池',
    preferred_tier  VARCHAR(16) NOT NULL COMMENT '模型强弱等级',
    matched_rule    VARCHAR(128) COMMENT '匹配的语义路由规则',
    matched_rules   JSON COMMENT '匹配规则列表',
    tier_rule       VARCHAR(128) COMMENT '匹配的Tier路由规则',
    semantic_scores JSON COMMENT '各 pool 语义分数',
    model_ranking_json JSON COMMENT '候选模型及 final_score 排名',
    confidence      DOUBLE COMMENT '决策置信度',
    final_decision_source VARCHAR(32) COMMENT '最终决策来源',
    fallback_reason VARCHAR(128) COMMENT 'fallback 原因',
    requires_file_parsing BOOLEAN DEFAULT FALSE COMMENT '是否需要文件解析',
    selected_account_id BIGINT DEFAULT 0 COMMENT 'semantic-router dry-run 选择账号',
    model_requested VARCHAR(128) COMMENT '请求的模型',
    model_resolved  VARCHAR(128) COMMENT '解析后的模型',
    selected_model  VARCHAR(128) COMMENT 'semantic-router dry-run 选择模型',
    scheduler_layer VARCHAR(64) COMMENT 'semantic-router dry-run 调度层',
    old_scheduler_account_id BIGINT DEFAULT 0 COMMENT '旧 Scheduler 实际选择账号',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',

    INDEX idx_request_id (request_id),
    INDEX idx_api_key_id (api_key_id),
    INDEX idx_created_at (created_at),
    INDEX idx_task_type (task_type),
    INDEX idx_preferred_pool (preferred_pool),
    INDEX idx_selected_account_id (selected_account_id),
    INDEX idx_group_id (group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='路由决策日志表';
`
}

// RoutingRuleMigration 返回创建语义路由规则表的 SQL
func RoutingRuleMigration() string {
	return `
-- 语义路由规则表
-- 用于配置基于规则的语义路由
CREATE TABLE IF NOT EXISTS semantic_routing_rule (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name            VARCHAR(128) NOT NULL UNIQUE COMMENT '规则名称',
    description     VARCHAR(512) COMMENT '规则描述',
    priority        INT DEFAULT 0 COMMENT '规则优先级(数字越大优先级越高)',
    enabled         BOOLEAN DEFAULT TRUE COMMENT '是否启用',

    -- 匹配条件
    model_pattern   VARCHAR(256) COMMENT '模型名称匹配模式(支持 * 和 ? 通配符)',
    prompt_contains VARCHAR(512) COMMENT 'Prompt 包含的关键字',
    prompt_regex    VARCHAR(512) COMMENT 'Prompt 正则匹配',
    content_type    VARCHAR(64) COMMENT 'Content-Type 匹配',
    has_image       BOOLEAN COMMENT '是否包含图片',
    has_document    BOOLEAN COMMENT '是否包含文档',

    -- 路由结果
    task_type       VARCHAR(32) NOT NULL COMMENT '任务类型',
    modality        VARCHAR(32) NOT NULL COMMENT '模态类型',
    preferred_pool  VARCHAR(32) NOT NULL COMMENT '首选账号池',
    vision_capable  BOOLEAN DEFAULT FALSE COMMENT '是否需要视觉能力',
    document_capable BOOLEAN DEFAULT FALSE COMMENT '是否需要文档处理能力',
    confidence      DOUBLE DEFAULT 1.0 COMMENT '默认置信度',

    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_priority (priority DESC),
    INDEX idx_enabled (enabled),
    INDEX idx_model_pattern (model_pattern)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='语义路由规则表';
`
}

// TierRoutingRuleMigration 返回创建 Tier 路由规则表的 SQL
func TierRoutingRuleMigration() string {
	return `
-- Tier 路由规则表
-- 用于配置强弱模型分流规则
CREATE TABLE IF NOT EXISTS tier_routing_rule (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name            VARCHAR(128) NOT NULL UNIQUE COMMENT '规则名称',
    description     VARCHAR(512) COMMENT '规则描述',
    priority        INT DEFAULT 0 COMMENT '规则优先级',
    enabled         BOOLEAN DEFAULT TRUE COMMENT '是否启用',

    -- 匹配条件
    model_pattern   VARCHAR(256) COMMENT '模型名称匹配模式',
    task_type       VARCHAR(32) COMMENT '任务类型',
    prompt_length_min INT COMMENT 'Prompt 最小长度',
    prompt_length_max INT COMMENT 'Prompt 最大长度',

    -- 路由结果
    preferred_tier  VARCHAR(16) NOT NULL COMMENT '首选Tier: weak/medium/strong',
    reason          VARCHAR(256) COMMENT '路由原因',

    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_priority (priority DESC),
    INDEX idx_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Tier路由规则表';
`
}

// AllMigrations 返回所有路由相关的 migration
func AllMigrations() []string {
	return []string{
		RoutingDecisionLogMigration(),
		RoutingRuleMigration(),
		TierRoutingRuleMigration(),
	}
}
