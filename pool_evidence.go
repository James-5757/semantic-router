package semanticrouter

// PoolEvidenceSpec 定义每个 Pool 的证据规范
// 包含：必需证据、支持证据、矛盾证据、最低证据数量
type PoolEvidenceSpec struct {
	PoolName           PreferredPool
	RequiredEvidence   []EvidenceType  // 至少需要 1 个
	SupportingEvidence []EvidenceType  // 越多越好
	ContradictingEvidence []EvidenceType // 存在则拒绝
	MinEvidenceCount   int              // 最低独立证据数量（除非有明确强证据）
	StrongEvidence     []EvidenceType   // 强证据（1个等于 MinEvidenceCount 个普通证据）
	Description        string
}

// EvidenceType 证据类型
type EvidenceType string

const (
	// 代码相关证据
	EvidenceCodingAction   EvidenceType = "coding_action"    // implement/debug/refactor/write code
	EvidenceTechnicalObject EvidenceType = "technical_object" // function/class/API/code/SQL/algorithm
	EvidenceProgrammingContext EvidenceType = "programming_context" // Python/Go/Java/JavaScript 等
	EvidenceCodeInput      EvidenceType = "code_input"       // 代码输入附件
	EvidenceCodeOutput     EvidenceType = "code_output"      // executable_code 输出

	// 数据相关证据
	EvidenceDataAction     EvidenceType = "data_action"       // analyze/visualize/clean/predict
	EvidenceDataObject     EvidenceType = "data_object"       // csv/excel/table/dataset
	EvidenceChartOutput    EvidenceType = "chart_output"      // chart/graph/visualization 输出
	EvidenceDataDomain     EvidenceType = "data_domain"       // statistics/machine_learning

	// 视觉相关证据
	EvidenceImageInput     EvidenceType = "image_input"       // 现有图片输入
	EvidenceVisionAction   EvidenceType = "vision_action"     // describe/analyze/recognize/OCR
	EvidenceImageOutput    EvidenceType = "image_output"      // 生成图片（非理解）

	// 文档相关证据
	EvidenceDocumentInput  EvidenceType = "document_input"    // 文档输入
	EvidenceDocumentAction EvidenceType = "document_action"   // summarize/extract/translate/review

	// 图像生成相关证据
	EvidenceCreativeAction EvidenceType = "creative_action"   // generate/create/draw/design
	EvidenceArtisticOutput EvidenceType = "artistic_output"   // 海报/插画/艺术图片
	EvidenceImageGenIntent EvidenceType = "image_gen_intent"  // 明确的图像生成意图

	// 通用证据
	EvidenceTextOnly       EvidenceType = "text_only"         // 纯文本输入
	EvidenceAmbiguous      EvidenceType = "ambiguous"         // 歧义/多义词
	EvidenceSimpleQuery    EvidenceType = "simple_query"      // 简单问答/闲聊
)

// PoolEvidenceSpecs 所有 Pool 的证据规范
var PoolEvidenceSpecs = map[PreferredPool]PoolEvidenceSpec{
	PoolCode: {
		PoolName: PoolCode,
		Description: "代码开发：需要编程动作 + (技术对象 | 编程语言) + (代码输入 | 代码输出)",
		RequiredEvidence: []EvidenceType{
			EvidenceCodingAction,
		},
		SupportingEvidence: []EvidenceType{
			EvidenceTechnicalObject,
			EvidenceProgrammingContext,
			EvidenceCodeInput,
			EvidenceCodeOutput,
		},
		ContradictingEvidence: []EvidenceType{
			EvidenceSimpleQuery,
		},
		MinEvidenceCount: 2, // 至少需要 2 个独立证据，或者 1 个强证据
		StrongEvidence: []EvidenceType{
			EvidenceProgrammingContext, // 明确的编程语言
			EvidenceCodeOutput,         // 明确要输出可执行代码
		},
	},
	PoolData: {
		PoolName: PoolData,
		Description: "数据分析：需要数据动作 + (数据对象 | 图表输出)",
		RequiredEvidence: []EvidenceType{
			EvidenceDataAction,
		},
		SupportingEvidence: []EvidenceType{
			EvidenceDataObject,
			EvidenceChartOutput,
			EvidenceDataDomain,
		},
		ContradictingEvidence: []EvidenceType{
			EvidenceSimpleQuery,
		},
		MinEvidenceCount: 2,
		StrongEvidence: []EvidenceType{
			EvidenceChartOutput,  // 明确要生成图表
			EvidenceDataDomain,   // 明确的机器学习/统计领域
		},
	},
	PoolVision: {
		PoolName: PoolVision,
		Description: "图像理解：需要图像输入 + 视觉动作",
		RequiredEvidence: []EvidenceType{
			EvidenceImageInput,
			EvidenceVisionAction,
		},
		SupportingEvidence: []EvidenceType{
			EvidenceTechnicalObject,
		},
		ContradictingEvidence: []EvidenceType{
			EvidenceImageOutput,
			EvidenceCreativeAction,
			EvidenceArtisticOutput,
		},
		MinEvidenceCount: 2,
		StrongEvidence: []EvidenceType{
			EvidenceImageInput, // 明确的图片输入
		},
	},
	PoolDocument: {
		PoolName: PoolDocument,
		Description: "文档处理：需要文档输入 + 文档动作",
		RequiredEvidence: []EvidenceType{
			EvidenceDocumentInput,
			EvidenceDocumentAction,
		},
		SupportingEvidence: []EvidenceType{
			EvidenceTechnicalObject,
		},
		ContradictingEvidence: []EvidenceType{
			EvidenceSimpleQuery,
		},
		MinEvidenceCount: 2,
		StrongEvidence: []EvidenceType{
			EvidenceDocumentInput,
		},
	},
	PoolImageGeneration: {
		PoolName: PoolImageGeneration,
		Description: "图像生成：需要生成动作 + 艺术输出 | 明确的图像生成意图",
		RequiredEvidence: []EvidenceType{
			EvidenceCreativeAction,
		},
		SupportingEvidence: []EvidenceType{
			EvidenceArtisticOutput,
			EvidenceImageGenIntent,
		},
		ContradictingEvidence: []EvidenceType{
			EvidenceImageInput,
			EvidenceVisionAction,
			EvidenceSimpleQuery,
		},
		MinEvidenceCount: 2,
		StrongEvidence: []EvidenceType{
			EvidenceArtisticOutput,
			EvidenceImageGenIntent,
		},
	},
	PoolCheap: {
		PoolName: PoolCheap,
		Description: "简单聊天：纯文本 + 简单查询",
		RequiredEvidence: []EvidenceType{
			EvidenceTextOnly,
			EvidenceSimpleQuery,
		},
		SupportingEvidence: []EvidenceType{},
		ContradictingEvidence: []EvidenceType{
			EvidenceCodingAction,
			EvidenceDataAction,
			EvidenceImageInput,
			EvidenceDocumentInput,
			EvidenceCodeOutput,
			EvidenceChartOutput,
			EvidenceArtisticOutput,
		},
		MinEvidenceCount: 1,
		StrongEvidence: []EvidenceType{},
	},
	PoolDefault: {
		PoolName: PoolDefault,
		Description: "通用池：复杂或无法明确分类的请求",
		RequiredEvidence: []EvidenceType{},
		SupportingEvidence: []EvidenceType{
			EvidenceTextOnly,
		},
		ContradictingEvidence: []EvidenceType{},
		MinEvidenceCount: 0,
		StrongEvidence: []EvidenceType{},
	},
}

// CandidatePool 候选池
type CandidatePool struct {
	Pool           PreferredPool `json:"pool"`
	CandidateScore float64       `json:"candidate_score"`
	// 证据相关
	SupportingEvidence     []EvidenceType `json:"supporting_evidence"`
	MissingRequiredEvidence []EvidenceType `json:"missing_required_evidence"`
	ContradictingEvidence  []EvidenceType  `json:"contradicting_evidence"`
	EvidenceCount          int             `json:"evidence_count"`
	// 验证结果
	Validated       bool   `json:"validated"`
	RejectionReason string `json:"rejection_reason"`
}

// PoolValidator 池验证器
type PoolValidator struct {
	specs map[PreferredPool]PoolEvidenceSpec
}

// NewPoolValidator 创建池验证器
func NewPoolValidator() *PoolValidator {
	return &PoolValidator{
		specs: PoolEvidenceSpecs,
	}
}

// ValidateCandidate 验证候选池
func (v *PoolValidator) ValidateCandidate(candidate *CandidatePool, taskUnderstanding *TaskSchema) *CandidatePool {
	spec, ok := v.specs[candidate.Pool]
	if !ok {
		candidate.Validated = false
		candidate.RejectionReason = "unknown_pool"
		return candidate
	}

	// 1. 检查矛盾证据
	for _, contradicting := range spec.ContradictingEvidence {
		for _, evidence := range candidate.SupportingEvidence {
			if evidence == contradicting {
				candidate.Validated = false
				candidate.RejectionReason = "contradicting_evidence:" + string(contradicting)
				return candidate
			}
		}
	}

	// 2. 检查必需证据
	missingRequired := []EvidenceType{}
	for _, required := range spec.RequiredEvidence {
		found := false
		for _, evidence := range candidate.SupportingEvidence {
			if evidence == required {
				found = true
				break
			}
		}
		if !found {
			missingRequired = append(missingRequired, required)
		}
	}
	candidate.MissingRequiredEvidence = missingRequired

	// 检查是否有强证据可以替代
	hasStrongEvidence := false
	for _, strong := range spec.StrongEvidence {
		for _, evidence := range candidate.SupportingEvidence {
			if evidence == strong {
				hasStrongEvidence = true
				break
			}
		}
		if hasStrongEvidence {
			break
		}
	}

	if len(missingRequired) > 0 && !hasStrongEvidence {
		candidate.Validated = false
		candidate.RejectionReason = "missing_required_evidence:" + joinEvidenceTypes(missingRequired)
		return candidate
	}

	// 3. 检查证据数量
	evidenceCount := len(candidate.SupportingEvidence)

	// 计算有效证据数（强证据计为 2）
	for _, evidence := range candidate.SupportingEvidence {
		for _, strong := range spec.StrongEvidence {
			if evidence == strong {
				evidenceCount++ // 强证据额外加 1
				break
			}
		}
	}

	candidate.EvidenceCount = evidenceCount

	if evidenceCount < spec.MinEvidenceCount && !hasStrongEvidence {
		candidate.Validated = false
		candidate.RejectionReason = "insufficient_evidence"
		return candidate
	}

	// 4. 如果 Task Understanding 显示歧义，降低验证通过可能性
	if taskUnderstanding != nil && taskUnderstanding.Ambiguous {
		// 如果任务理解有歧义，需要更多证据才能通过
		if evidenceCount < spec.MinEvidenceCount+1 {
			candidate.Validated = false
			candidate.RejectionReason = "ambiguous_task_insufficient_evidence"
			return candidate
		}
	}

	// 通过验证
	candidate.Validated = true
	return candidate
}

// ValidateCandidates 验证多个候选池
func (v *PoolValidator) ValidateCandidates(candidates []*CandidatePool, taskUnderstanding *TaskSchema) []*CandidatePool {
	validated := make([]*CandidatePool, 0, len(candidates))
	for _, candidate := range candidates {
		validated = append(validated, v.ValidateCandidate(candidate, taskUnderstanding))
	}
	return validated
}

// joinEvidenceTypes 连接证据类型
func joinEvidenceTypes(types []EvidenceType) string {
	result := ""
	for i, t := range types {
		if i > 0 {
			result += ","
		}
		result += string(t)
	}
	return result
}

// GetPoolSpec 获取池的证据规范
func GetPoolSpec(pool PreferredPool) (PoolEvidenceSpec, bool) {
	spec, ok := PoolEvidenceSpecs[pool]
	return spec, ok
}