package semanticrouter

import (
	"time"
)

// EvalCase 评估样本
type EvalCase struct {
	Prompt          string   `json:"prompt"`
	Model           string   `json:"model"`
	Images          []string `json:"images,omitempty"`
	Documents       []string `json:"documents,omitempty"`
	ExpectedPool    string   `json:"expected_pool"`
	ExpectedTier    string   `json:"expected_tier"`
	ExpectedFallback bool    `json:"expected_fallback,omitempty"`
}

// TaskType 表示任务类型
type TaskType string

const (
	TaskTypeText          TaskType = "text"          // 普通文本对话
	TaskTypeVision        TaskType = "vision"        // 视觉/图片理解
	TaskTypeImageGenerate TaskType = "image_generate" // 图片生成
	TaskTypeDocument      TaskType = "document"      // 文档处理
	TaskTypeCode          TaskType = "code"          // 代码生成
	TaskTypeUnknown       TaskType = "unknown"
)

// Modality 表示请求的模态类型
type Modality string

const (
	ModalityTextOnly    Modality = "text_only"
	ModalityTextImage   Modality = "text_image"
	ModalityImageOnly   Modality = "image_only"
	ModalityDocument    Modality = "document"
	ModalityMultiModal  Modality = "multi_modal"
)

// PreferredTier 表示模型强弱等级
type PreferredTier string

const (
	TierWeak   PreferredTier = "weak"
	TierMedium PreferredTier = "medium"
	TierStrong PreferredTier = "strong"
)

// PreferredPool 表示首选账号池
type PreferredPool string

const (
	PoolDefault          PreferredPool = "default"
	PoolVision           PreferredPool = "vision"
	PoolDocument         PreferredPool = "document"
	PoolCode             PreferredPool = "code"
	PoolCheap            PreferredPool = "cheap"
	PoolData             PreferredPool = "data"
	PoolPrivate          PreferredPool = "private"
	PoolImageGeneration  PreferredPool = "image_generation"
)

// RequiredCapabilities 表示所需能力
type RequiredCapabilities struct {
	ImageCapability ImageCapabilityType // 图片能力要求
	VisionCapable   bool                 // 是否需要视觉能力
	DocumentCapable bool                 // 是否需要文档处理能力
}

// ImageCapabilityType 图片能力类型
type ImageCapabilityType string

const (
	ImageCapabilityNone   ImageCapabilityType = "none"
	ImageCapabilityBasic  ImageCapabilityType = "basic"
	ImageCapabilityNative ImageCapabilityType = "native"
)

// SemanticRouteDecision 语义路由决策
type SemanticRouteDecision struct {
	PreferredPool          PreferredPool
	RequiredCapabilities  RequiredCapabilities
	TaskType              TaskType
	Modality              Modality
	Confidence           float64 // 决策置信度 0-1
	MatchedRule          string  // 匹配的规则名称
	ProcessingHint       string  // 处理提示（如需要文件解析）
}

// TierRouteDecision Tier 路由决策
type TierRouteDecision struct {
	PreferredTier PreferredTier
	Confidence    float64
	MatchedRule   string
	Reason        string
}

// CombinedRouteDecision 组合路由决策（语义+Tier）
type CombinedRouteDecision struct {
	Semantic     SemanticRouteDecision
	Tier         TierRouteDecision
	FinalPool    PreferredPool // 最终账号池
	RequiresFileParsing bool   // 是否需要文件解析
	Timestamp    time.Time
}

// SemanticRouter 语义路由接口
type SemanticRouter interface {
	// Route 根据请求内容进行语义路由
	Route(ctx interface{}, request interface{}) (*SemanticRouteDecision, error)
	// GetName 获取路由名称
	GetName() string
}

// TierRouter Tier 路由接口
type TierRouter interface {
	// Route 根据请求和模型信息进行 Tier 路由
	Route(ctx interface{}, model string, taskType TaskType) (*TierRouteDecision, error)
	// GetName 获取路由名称
	GetName() string
}

// RoutingDecisionLogger 路由决策日志接口
type RoutingDecisionLogger interface {
	// LogDecision 记录路由决策
	LogDecision(decision *CombinedRouteDecision, requestID string) error
	// GetRecentDecisions 获取最近的路由决策
	GetRecentDecisions(limit int) ([]*RoutingLogEntry, error)
	// GetStats 获取统计信息
	GetStats() *RoutingStats
	// Close 关闭连接
	Close() error
	// Ping 检查连接
	Ping() error
}

// RoutingLogEntry 路由日志条目
type RoutingLogEntry struct {
	ID             string
	RequestID      string
	GroupID        int64
	Timestamp      time.Time
	TaskType       TaskType
	Modality       Modality
	PreferredPool  PreferredPool
	PreferredTier  PreferredTier
	MatchedRule    string
	TierRule       string
	Confidence     float64
	RequiresFileParsing bool
	ModelRequested string
	ModelResolved  string
}

// FileParser 文件解析接口
type FileParser interface {
	// Parse 解析文件内容
	Parse(fileData []byte, fileType string) (parsedContent string, err error)
	// GetSupportedTypes 获取支持的文件类型
	GetSupportedTypes() []string
}

// DocumentProcessor 文档处理服务
type DocumentProcessor interface {
	// Process 处理文档请求
	Process(ctx interface{}, request interface{}) (*DocumentProcessResult, error)
}

// DocumentProcessResult 文档处理结果
type DocumentProcessResult struct {
	Content      string
	Format       string
	PageCount    int
	ExtractedAt  time.Time
	Error        error
}