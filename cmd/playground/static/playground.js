// Router Playground JavaScript

let lastResult = null;
let lastModelRanking = null;
const $ = (id) => document.getElementById(id);
let uiLanguage = localStorage.getItem('router-playground-language') || 'en';

const uiText = {
    en: {
        title: 'Router Playground', router_tab: '🔬 Router', dataset_tab: '📁 Dataset', history_tab: '📜 Debug History',
        takeover_disabled: 'Takeover: Disabled', has_image: 'Has Image', has_document: 'Has Document', has_csv: 'Has CSV/Data',
        run: 'Run', copy_result: 'Copy Result', raw_json: 'Show Raw JSON', routing_summary: '🔄 Routing Summary',
        history_heading: '📜 Debug History', history_subtitle: 'Automatically displays Token Cloud shadow calls. It does not affect live scheduling.',
        switch_language: '中文', loading: 'Loading...', local_records: 'Local Playground records', selector_records: 'Token Cloud forwarded records',
        no_selector_records: 'No /v1/model-selector/select calls received yet.', selector_unavailable: 'Token Cloud history is temporarily unavailable.',
        details: 'View Details', time: 'Time', request_id: 'Request ID', prompt_summary: 'Prompt Summary', candidates: 'Candidate Models', top_model: 'Top Model'
    },
    zh: {
        title: '路由调试台', router_tab: '🔬 路由', dataset_tab: '📁 数据集', history_tab: '📜 调试历史',
        takeover_disabled: '接管：已禁用', has_image: '包含图片', has_document: '包含文档', has_csv: '包含 CSV/数据',
        run: '运行', copy_result: '复制结果', raw_json: '显示原始 JSON', routing_summary: '🔄 路由摘要',
        history_heading: '📜 调试历史', history_subtitle: '自动显示 Token 云转发到模型选择器的旁路调用，不影响真实调度。',
        switch_language: 'English', loading: '正在加载...', local_records: '本地 Playground 记录', selector_records: 'Token 云转发记录',
        no_selector_records: '尚未收到 /v1/model-selector/select 调用。', selector_unavailable: 'Token 云历史暂时不可用。',
        details: '查看详情', time: '时间', request_id: '请求 ID', prompt_summary: 'Prompt 摘要', candidates: '候选模型', top_model: '最高分模型'
    }
};

const cardLabelText = {
    'Service Status': '服务状态', 'Strong Win Probability': '强模型胜率', 'Suggested Tier': '建议 Tier', 'Weak Threshold': '弱模型阈值', 'Strong Threshold': '强模型阈值',
    'Shadow Only': '仅旁路', 'Upstream Called': '是否调用上游', 'Rule Tier': '规则 Tier', 'Final Tier': '最终 Tier', 'Tier Source': 'Tier 来源', 'Agreement': '一致性', 'Latency': '延迟',
    'Boundary Eligible': '边界判定可用', 'Boundary Reasons': '边界原因', 'Hybrid Suggested Tier': 'Hybrid 建议 Tier', 'Decision Reason': '决策原因', 'Used For Final': '用于最终结果',
    'Primary Pool': '主 Pool', 'Secondary Pool': '次 Pool', 'Matched Rules': '命中规则', 'Hard Rules': '硬规则', 'Confidence': '置信度', 'Top1-Top2 Margin': 'Top1-Top2 差距', 'Decision Source': '决策来源',
    'Actions': '动作', 'Objects': '对象', 'Input Modalities': '输入模态', 'Output Artifacts': '输出产物', 'Primary Intent': '主意图', 'Detected Intents': '识别意图', 'Required Capabilities': '所需能力', 'Ambiguous': '存在歧义',
    'Pool Top Scores': 'Pool 最高分', 'Prompt Length': 'Prompt 长度', 'Step Count': '步骤数', 'Constraint Count': '约束数', 'Multi-Intent Count': '多意图数', 'Complexity Score': '复杂度分数', 'Thresholds': '阈值', 'Requested Tier': '请求 Tier', 'Selected Tier': '选中 Tier',
    'Best Pool': '最佳 Pool', 'Best Score': '最高分', 'Second Pool': '第二 Pool', 'Second Score': '第二分数', 'Margin': '差距', 'Invoked': '是否调用', 'Reason': '原因', 'Score Margin': '分数差距', 'Model': '模型', 'Inference Latency': '推理延迟', 'HTTP Latency': 'HTTP 延迟',
    'Eligible': '是否符合条件', 'Override': '是否覆盖', 'Override Reason': '覆盖原因', 'Final Pool': '最终 Pool', 'Recommended Model': '推荐模型', 'Old → New Model': '旧模型 → 新模型', 'Old/New Agreement': '旧/新一致性', 'Ranking Margin': '排名差距', 'Recommendation Reason': '推荐原因', 'Candidate Count': '候选数量', 'Candidate Profiles': '候选模型画像', 'Selected Account': '选中账号', 'Candidate Models': '候选模型数', 'Candidate List': '候选列表', 'Selection Source': '选择来源', 'Pool': 'Pool', 'Tier (Scheduler)': 'Scheduler Tier', 'Tier (Rule)': '规则 Tier', 'Tier Agreement': 'Tier 一致性', 'Layer': '调度层', 'Capability Match': '能力匹配', 'Candidates': '候选模型',
    'Provider': '提供方', 'Decision': '判断', 'Category': '分类', 'Mapped Pool': '映射 Pool', 'Logical Pool': '逻辑 Pool', 'Physical Model Group': '物理模型组', 'Hybrid Candidate': 'Hybrid 候选', 'Hybrid Source': 'Hybrid 来源', 'Pool Agreement': 'Pool 一致性', 'Semantic Agreement': '语义一致性',
    'Output Mode': '输出模式', 'Requires Multimodal': '需要多模态', 'Multimodal Type': '多模态类型', 'Domain': '领域', 'Tool Profile': '工具画像', 'Fine Intent': '细分意图', 'Has Image': '包含图片', 'Has Document': '包含文档', 'Has CSV': '包含 CSV', 'Chart Analysis': '图表分析', 'Meme Analysis': '梗图分析', 'Text-Image Fusion': '图文融合', 'Image Generation': '图像生成', 'Video Generation': '视频生成', 'Multimodal by Metadata': '由元数据判定多模态', 'Detected Modalities': '检测到的模态',
    'V2 Group': 'V2 分组', 'Rule Type': '规则类型', 'Official vLLM Scores': 'Official vLLM 分数', 'Technical Score': '技术分数', 'General Score': '通用分数', 'Official Decision': 'Official 判断', 'Score Source': '分数来源', 'E5 Scores': 'E5 分数', 'Domain Score': '领域分数', 'Second Domain': '第二领域', 'Domain Margin': '领域差距',
    'Triggered': '是否触发', 'Disagreement': '存在分歧', 'Candidate Group': '候选分组', 'Override Eligible': '允许覆盖', 'Override Threshold': '覆盖阈值', 'Primary Tool': '主工具', 'Secondary Tools': '次工具', 'Complexity Candidate': '复杂度建议', 'RouteLLM Candidate': 'RouteLLM 建议', 'Policy Minimum': '策略最低 Tier', 'Selected Tier Source': '选中 Tier 来源', 'Local V2 Group': '本地 V2 分组', 'Official V2 Group': 'Official V2 分组', 'V2 Group Agreement': 'V2 分组一致性'
};

function translateRenderedCards() {
    document.querySelectorAll('.card .label:not([data-i18n]), .card h3:not([data-i18n]), .card h4:not([data-i18n]), .card-subtitle, .v2-header span').forEach(function(element) {
        var source = element.dataset.i18nSource || element.textContent.trim();
        if (!element.dataset.i18nSource) element.dataset.i18nSource = source;
        if (uiLanguage === 'zh' && cardLabelText[source]) element.textContent = cardLabelText[source];
        else if (uiLanguage === 'en') element.textContent = source;
    });
    document.querySelectorAll('.card .value').forEach(function(element) {
        var source = element.dataset.i18nValue || element.textContent.trim();
        if (!element.dataset.i18nValue) element.dataset.i18nValue = source;
        if (uiLanguage === 'zh' && source === 'Yes') element.textContent = '是';
        else if (uiLanguage === 'zh' && source === 'No') element.textContent = '否';
        else if (uiLanguage === 'en') element.textContent = source;
    });
}

function t(key) { return (uiText[uiLanguage] || uiText.en)[key] || key; }

function applyLanguage() {
    document.documentElement.lang = uiLanguage === 'zh' ? 'zh-CN' : 'en';
    document.querySelectorAll('[data-i18n]').forEach(function(element) { element.textContent = t(element.dataset.i18n); });
    var button = $('language-toggle');
    if (button) button.textContent = t('switch_language');
    translateRenderedCards();
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', function() {
    applyLanguage();
    var languageButton = $('language-toggle');
    if (languageButton) languageButton.addEventListener('click', function() {
        uiLanguage = uiLanguage === 'zh' ? 'en' : 'zh';
        localStorage.setItem('router-playground-language', uiLanguage);
        applyLanguage();
        if ($('tab-history') && $('tab-history').classList.contains('active')) loadHistory();
    });
    checkEmbeddingHealth();
    checkRouteLLMHealth();
    setupEventListeners();
    // Tab buttons
    document.querySelectorAll('.tab-btn').forEach(function(btn) {
        btn.addEventListener('click', function() {
            switchTab(this.dataset.tab);
        });
    });
    // Save Result
    var saveBtn = $('save-result-btn');
    if (saveBtn) saveBtn.addEventListener('click', saveResult);
    // Dataset
    var dpBtn = $('dataset-preview-btn');
    if (dpBtn) dpBtn.addEventListener('click', datasetPreview);
    var dcBtn = $('dataset-confirm-column-btn');
    if (dcBtn) dcBtn.addEventListener('click', datasetConfirmColumn);
    var dci = $('dataset-column-input');
    if (dci) dci.addEventListener('input', datasetColumnManualInput);
    var drBtn = $('dataset-run-btn');
    if (drBtn) drBtn.addEventListener('click', datasetRunBatch);
    var dsBtn = $('dataset-stop-btn');
    if (dsBtn) dsBtn.addEventListener('click', datasetStopBatch);
    var deBtn = $('dataset-export-btn');
    if (deBtn) deBtn.addEventListener('click', datasetExportResults);
    // History
    var haBtn = $('hist-filter-apply-btn');
    if (haBtn) haBtn.addEventListener('click', loadHistory);
    var heBtn = $('hist-export-btn');
    if (heBtn) heBtn.addEventListener('click', historyExport);
    // Review
    var rsBtn = $('review-submit-btn');
    if (rsBtn) rsBtn.addEventListener('click', reviewSubmit);
    var rcBtn = $('review-cancel-btn');
    if (rcBtn) rcBtn.addEventListener('click', reviewClose);

    // The TokenCloud selector history is a live, shadow-only feed. Refresh it
    // while the history tab is visible so a peer's /select request appears
    // without requiring a manual page reload.
    window.setInterval(function() {
        if ($('tab-history') && $('tab-history').classList.contains('active')) loadHistory();
    }, 5000);
});

// Check embedding service health
async function checkEmbeddingHealth() {
    try {
        const response = await fetch('/debug/embedding-health');
        const data = await response.json();
        updateHealthStatus(data);
    } catch (error) {
        updateHealthStatus({ status: 'unavailable', error: error.message });
    }
}

async function checkRouteLLMHealth() {
    const healthEl = $('routellm-status');
    try {
        const response = await fetch('/debug/routellm-tier-health');
        const data = await response.json();
        healthEl.textContent = 'RouteLLM Tier: ' + (data.status || 'unavailable') + ' (Shadow Only)';
        healthEl.className = 'status-item ' + (data.status === 'ok' ? 'success' : 'info');
    } catch (error) {
        healthEl.textContent = 'RouteLLM Tier: Unavailable (Shadow Only)';
        healthEl.className = 'status-item error';
    }
}

function updateHealthStatus(data) {
    const healthEl = $('embedding-status');
    if (data.status === 'healthy' || data.available) {
        healthEl.textContent = 'Embedding Service: Available';
        healthEl.className = 'status-item success';
    } else {
        healthEl.textContent = 'Embedding Service: Unavailable';
        healthEl.className = 'status-item error';
    }
}

// Setup event listeners
function setupEventListeners() {
    var runBtn = $('run-btn');
    var copyBtn = $('copy-btn');
    var rawBtn = $('toggle-raw-btn');
    if (runBtn) runBtn.addEventListener('click', handleRun);
    if (copyBtn) copyBtn.addEventListener('click', handleCopy);
    if (rawBtn) rawBtn.addEventListener('click', handleToggleRaw);
    var copyRankingBtn = $('copy-ranking-btn');
    var downloadRankingBtn = $('download-ranking-btn');
    if (copyRankingBtn) copyRankingBtn.addEventListener('click', handleCopyRanking);
    if (downloadRankingBtn) downloadRankingBtn.addEventListener('click', handleDownloadRanking);
}

// Handle Run button click
async function handleRun() {
    const runBtn = $('run-btn');
    runBtn.disabled = true;
    runBtn.textContent = 'Running...';

    try {
        const mode = document.querySelector('input[name="mode"]:checked').value;
        const body = {
            prompt: $('prompt').value,
            has_image: $('has-image').checked,
            has_document: $('has-document').checked,
            has_csv: $('has-csv').checked,
            mode: mode
        };

        const response = await fetch('/v1/debug/router/playground', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        lastResult = await response.json();
        renderResult(lastResult);
    } catch (error) {
        showError('Request failed: ' + error.message);
    } finally {
        runBtn.disabled = false;
        runBtn.textContent = 'Run';
    }
}

// Render the result
function renderResult(result) {
    // Update debug info
    const debug = result.debug || {};
    $('debug-scheduler').textContent = 'Scheduler Source: ' + (debug.scheduler_source || 'mock');
    $('debug-dryrun').textContent = 'Dry Run: ' + (debug.dry_run !== false ? 'True' : 'False');
    $('debug-upstream').textContent = 'Upstream Called: ' + (debug.upstream_called ? 'True' : 'False');

    // Get selected mode
    const mode = document.querySelector('input[name="mode"]:checked').value;
    const modeLabels = { baseline: 'Baseline', full_embedding: 'Full Embedding', selective: 'Selective', compare: 'Compare All' };
    $('current-mode').textContent = 'Current Mode: ' + (modeLabels[mode] || 'Baseline');

    // Render cards
    renderDecisionTrace(result.decision_trace || {}, result.hard_rules || {});
    renderTaskUnderstanding(result.decision_trace || {});
    renderTierDecision(result.decision_trace || {});
    renderBaseline(result.baseline || {});
    renderEmbedding(result.embedding || {});
    renderSelective(result.selective || {});
	renderRouteLLMTier(result.routellm_tier || {});
	renderHybridBoundary(result);
    renderScheduler(result.scheduler || {}, result);
    renderLocalProvider(result);
    renderPoolCard(result);
    renderOfficialVLLM(result);
    renderAgreementCard(result);

    // V2 cards (shadow trace)
    renderV2Decision(result);

    // Show save-result row
    const saveRow = $('save-result-row');
    if (saveRow) saveRow.style.display = 'block';

    // Update raw JSON
    $('raw-content').textContent = JSON.stringify(result, null, 2);
    translateRenderedCards();
}

// RouteLLM is intentionally separate from Pool Embedding and never takes over Tier routing.
function renderRouteLLMTier(decision) {
    const container = $('routellm-tier-card');
    const probability = decision.routellm_probability;
    container.innerHTML = `
        <div class="field"><span class="label">Service Status</span><span class="value">${decision.routellm_error ? 'Unavailable' : 'Available (Shadow)'}</span></div>
        <div class="field"><span class="label">Router</span><span class="value">${decision.router || 'bert'}</span></div>
        <div class="field"><span class="label">Strong Win Probability</span><span class="value">${probability == null ? 'N/A' : probability.toFixed(4)}</span></div>
        <div class="field"><span class="label">Suggested Tier</span><span class="value">${decision.routellm_tier || 'N/A'}</span></div>
        <div class="field"><span class="label">Weak Threshold</span><span class="value">${decision.weak_threshold?.toFixed(2) || 'N/A'}</span></div>
        <div class="field"><span class="label">Strong Threshold</span><span class="value">${decision.strong_threshold?.toFixed(2) || 'N/A'}</span></div>
        <div class="field"><span class="label">Shadow Only</span><span class="value">${decision.shadow_only ? 'Yes' : 'No'}</span></div>
        <div class="field"><span class="label">Upstream Called</span><span class="value">${decision.upstream_called ? 'Yes' : 'No'}</span></div>
        <div class="field"><span class="label">Rule Tier</span><span class="value">${decision.rule_tier || 'N/A'}</span></div>
        <div class="field"><span class="label">Final Tier</span><span class="value highlight">${decision.final_tier || 'N/A'}</span></div>
        <div class="field"><span class="label">Tier Source</span><span class="value">${decision.final_tier_source || 'rule_shadow'}</span></div>
        <div class="field"><span class="label">Agreement</span><span class="value">${decision.is_agreement ? 'Yes ✓' : 'No ✗'}</span></div>
        <div class="field"><span class="label">Latency</span><span class="value">${(decision.routellm_latency_ms || 0).toFixed(2)}ms</span></div>
        ${decision.routellm_error ? `<div class="alert warning">${decision.routellm_error}</div>` : ''}
    `;
}

// Hybrid Boundary Tier Card
function renderHybridBoundary(result) {
    const container = $('hybrid-boundary-card');
    const boundary = result.decision_trace?.boundary || {};
    const hybrid = result.hybrid_shadow || {};

    container.innerHTML = `
        <div class="field"><span class="label">Boundary Eligible</span><span class="value">${boundary.eligible ? 'Yes' : 'No'}</span></div>
        <div class="field"><span class="label">Boundary Reasons</span><span class="value">${boundary.reasons?.join(', ') || 'None'}</span></div>
        <div class="field"><span class="label">Hybrid Suggested Tier</span><span class="value highlight">${hybrid.suggested_tier || 'N/A'}</span></div>
        <div class="field"><span class="label">Decision Reason</span><span class="value">${hybrid.decision_reason || 'N/A'}</span></div>
        <div class="field"><span class="label">Used For Final</span><span class="value">${hybrid.used_for_final ? 'Yes' : 'No'}</span></div>
    `;
}

// Decision Trace Card
function renderDecisionTrace(trace, hardRules) {
    const container = $('decision-trace');
    container.innerHTML = `
        <div class="field">
            <span class="label">Primary Pool</span>
            <span class="value highlight">${trace.primary_pool || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Secondary Pool</span>
            <span class="value">${trace.secondary_pool || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Matched Rules</span>
            <span class="value">${formatList(trace.matched_rules)}</span>
        </div>
        <div class="field">
            <span class="label">Hard Rules</span>
            <span class="value">${formatList(hardRules.matched)}</span>
        </div>
        <div class="field">
            <span class="label">Confidence</span>
            <span class="value">${(trace.confidence || 0).toFixed(3)}</span>
        </div>
        <div class="field">
            <span class="label">Top1-Top2 Margin</span>
            <span class="value">${(trace.top1_top2_margin || 0).toFixed(3)}</span>
        </div>
        <div class="field">
            <span class="label">Decision Source</span>
            <span class="value">${trace.decision_source || 'baseline'}</span>
        </div>
        ${trace.understanding_conflict ? '<div class="alert warning">⚠️ Understanding Conflict Detected</div>' : ''}
    `;
}

// Task Understanding Card
function renderTaskUnderstanding(trace) {
    const tu = trace.task_understanding || {};
    const container = $('task-understanding');
    container.innerHTML = `
        <div class="field">
            <span class="label">Actions</span>
            <span class="value">${formatList(tu.actions)}</span>
        </div>
        <div class="field">
            <span class="label">Objects</span>
            <span class="value">${formatList(tu.objects)}</span>
        </div>
        <div class="field">
            <span class="label">Input Modalities</span>
            <span class="value">${formatList(tu.modalities)}</span>
        </div>
        <div class="field">
            <span class="label">Output Artifacts</span>
            <span class="value">${formatList(tu.output_artifacts)}</span>
        </div>
        <div class="field">
            <span class="label">Primary Intent</span>
            <span class="value">${tu.primary_intent || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Detected Intents</span>
            <span class="value">${formatList(trace.detected_intents)}</span>
        </div>
        <div class="field">
            <span class="label">Required Capabilities</span>
            <span class="value">${formatList(trace.required_capabilities)}</span>
        </div>
        <div class="field">
            <span class="label">Confidence</span>
            <span class="value">${(tu.confidence || 0).toFixed(2)}</span>
        </div>
        <div class="field">
            <span class="label">Ambiguous</span>
            <span class="value">${tu.ambiguous ? 'Yes' : 'No'}</span>
        </div>
    `;

        // Render pool scores into decision-trace
    const dtContainer = $('decision-trace');
    const scores = trace.pool_top_scores || [];
    if (scores.length > 0 && dtContainer) {
        const scoresDiv = document.createElement('div');
        scoresDiv.className = 'field';
        scoresDiv.innerHTML = '<span class="label">Pool Top Scores</span><span class="value">' +
            scores.map(s => `${s.pool}=${s.score.toFixed(3)}`).join(', ') +
            '</span>';
        dtContainer.appendChild(scoresDiv);
    }
}

// Tier Decision Card
function renderTierDecision(trace) {
    const td = trace.tier_decision || {};
    const container = $('tier-card');
    container.innerHTML = `
        <div class="field">
            <span class="label">Prompt Length</span>
            <span class="value">${td.prompt_length || 0}</span>
        </div>
        <div class="field">
            <span class="label">Step Count</span>
            <span class="value">${td.step_count || 0}</span>
        </div>
        <div class="field">
            <span class="label">Constraint Count</span>
            <span class="value">${td.constraint_count || 0}</span>
        </div>
        <div class="field">
            <span class="label">Multi-Intent Count</span>
            <span class="value">${td.multi_intent_count || 0}</span>
        </div>
        <div class="field">
            <span class="label">Complexity Score</span>
            <span class="value highlight">${(td.complexity_score || 0).toFixed(2)}</span>
        </div>
        <div class="field">
            <span class="label">Thresholds</span>
            <span class="value">weak ≤ 0.15, medium < 0.50, strong ≥ 0.50 (已归一化到 0~1)</span>
        </div>
        <div class="field">
            <span class="label">Requested Tier</span>
            <span class="value highlight">${td.requested_tier || 'medium'}</span>
        </div>
        <div class="field">
            <span class="label">Selected Tier</span>
            <span class="value highlight">${td.selected_tier || 'medium'}</span>
        </div>
        ${td.tier_fallback ? `<div class="alert warning">⚠️ Tier Fallback: ${td.tier_fallback_reason || 'N/A'}</div>` : ''}
    `;
}

// Baseline Card
function renderBaseline(baseline) {
    const container = $('baseline-card');
    const topK = baseline.top_k || [];
    container.innerHTML = `
        <div class="field">
            <span class="label">Best Pool</span>
            <span class="value highlight">${baseline.best_pool || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Best Score</span>
            <span class="value">${(baseline.best_score || 0).toFixed(3)}</span>
        </div>
        <div class="field">
            <span class="label">Second Pool</span>
            <span class="value">${baseline.second_pool || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Second Score</span>
            <span class="value">${(baseline.second_score || 0).toFixed(3)}</span>
        </div>
        <div class="field">
            <span class="label">Margin</span>
            <span class="value">${(baseline.score_margin || 0).toFixed(3)}</span>
        </div>
        <div class="field">
            <span class="label">Latency</span>
            <span class="value">${(baseline.latency_ms || 0).toFixed(2)}ms</span>
        </div>
    `;

    const topKContainer = $('baseline-topk');
    if (topK.length > 0) {
        topKContainer.innerHTML = topK.map(k =>
            `<div class="pool-score-item"><span class="pool">${k.pool}</span><span class="score">${k.score.toFixed(3)}</span></div>`
        ).join('');
    } else {
        topKContainer.innerHTML = '<span class="value empty">None</span>';
    }
}

// Embedding Card
function renderEmbedding(embedding) {
    const container = $('embedding-card');
    const ranking = embedding.ranked_pools || [];
    container.innerHTML = `
        <div class="field">
            <span class="label">Invoked</span>
            <span class="value ${embedding.invoked ? 'success' : ''}">${embedding.invoked}</span>
        </div>
        <div class="field">
            <span class="label">Reason</span>
            <span class="value">${embedding.invocation_reason || 'N/A'}</span>
        </div>
        <div class="field">
            <span class="label">Best Pool</span>
            <span class="value highlight">${embedding.best_pool || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Second Pool</span>
            <span class="value">${embedding.second_pool || 'None'}</span>
        </div>
        <div class="field">
            <span class="label">Score Margin</span>
            <span class="value">${(embedding.score_margin || 0).toFixed(3)}</span>
        </div>
        <div class="field">
            <span class="label">Model</span>
            <span class="value">${embedding.model || 'N/A'}</span>
        </div>
        <div class="field">
            <span class="label">Inference Latency</span>
            <span class="value">${(embedding.model_inference_latency_ms || 0).toFixed(2)}ms</span>
        </div>
        <div class="field">
            <span class="label">HTTP Latency</span>
            <span class="value">${(embedding.http_roundtrip_latency_ms || 0).toFixed(2)}ms</span>
        </div>
    `;

    const rankingContainer = $('embedding-ranking');
    if (ranking.length > 0) {
        rankingContainer.innerHTML = ranking.map((r, i) =>
            `<div class="rank-item ${i === 0 ? 'best' : ''}"><span class="pool">${r.pool}</span><span class="score">${r.score.toFixed(3)}</span></div>`
        ).join('');
    } else {
        rankingContainer.innerHTML = '<span class="value empty">None</span>';
    }
}

// Selective Card
function renderSelective(selective) {
    const container = $('selective-card');
    container.innerHTML = `
        <div class="field">
            <span class="label">Eligible</span>
            <span class="value">${selective.eligible}</span>
        </div>
        <div class="field">
            <span class="label">Invoked</span>
            <span class="value ${selective.invoked ? 'success' : ''}">${selective.invoked}</span>
        </div>
        <div class="field">
            <span class="label">Override</span>
            <span class="value ${selective.override ? 'error' : ''}">${selective.override}</span>
        </div>
        <div class="field">
            <span class="label">Reason</span>
            <span class="value">${selective.reason || 'N/A'}</span>
        </div>
        ${selective.override_reason ? `
        <div class="field">
            <span class="label">Override Reason</span>
            <span class="value">${selective.override_reason}</span>
        </div>
        ` : ''}
        <div class="field">
            <span class="label">Final Pool</span>
            <span class="value highlight">${selective.final_pool || 'N/A'}</span>
        </div>
        <div class="field">
            <span class="label">Final Tier</span>
            <span class="value highlight">${selective.final_tier || 'N/A'}</span>
        </div>
        <div class="field">
            <span class="label">Decision Source</span>
            <span class="value">${selective.decision_source || 'baseline'}</span>
        </div>
    `;
}

// Scheduler Card
function renderScheduler(scheduler, result) {
    const groupContainer = $('scheduler-card');
    const schedulerContainer = $('scheduler-detail');
    var ruleTier = ((result.tier_card||{}).selected_tier || 'N/A');
    var schedTier = (scheduler.requested_tier || 'N/A');
    var tierAgree = (ruleTier !== 'N/A' && schedTier !== 'N/A' && ruleTier === schedTier);
    var candidateModels = Array.isArray(scheduler.candidate_models) ? scheduler.candidate_models.join(', ') : 'N/A';
    var candidateDetails = Array.isArray(scheduler.candidate_details) ? scheduler.candidate_details : [];
    var profileRows = candidateDetails.map(function (candidate) {
        var score = candidate.profile_score != null ? Number(candidate.profile_score).toFixed(4) : 'N/A';
        var meta = [candidate.provider || 'unknown', 'score ' + score];
        if (candidate.score_confidence != null) meta.push('confidence ' + Number(candidate.score_confidence).toFixed(2));
        if (candidate.cost_per_task) meta.push('cost ' + candidate.cost_per_task);
        if (candidate.latency_ms) meta.push('latency ' + candidate.latency_ms + 'ms');
        return '<div class="field"><span class="label">' + escapeHtml(candidate.model_id || 'unknown') + '</span><span class="value" style="font-size:0.85em">' + escapeHtml(meta.join(' · ')) + '</span></div>';
    }).join('');
    groupContainer.innerHTML = [
        '<div class="alert info">Profile Registry → capability-filtered candidates</div>',
        '<div class="field"><span class="label">Recommended Model</span><span class="value highlight">' + escapeHtml(scheduler.recommended_model || scheduler.selected_model || 'None') + '</span></div>',
        '<div class="field"><span class="label">Old → New Model</span><span class="value">' + escapeHtml((scheduler.old_selected_model || scheduler.selected_model || 'None') + ' → ' + (scheduler.new_suggested_model || scheduler.recommended_model || 'None')) + '</span></div>',
        '<div class="field"><span class="label">Old/New Agreement</span><span class="value ' + (scheduler.old_vs_new_agreement ? 'success' : 'warning') + '">' + (scheduler.old_vs_new_agreement ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Ranking Margin</span><span class="value">' + (scheduler.ranking_margin != null ? Number(scheduler.ranking_margin).toFixed(4) : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Recommendation Reason</span><span class="value" style="font-size:0.85em">' + escapeHtml(scheduler.recommendation_reason || scheduler.decision_source || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Candidate Count</span><span class="value">' + (scheduler.candidate_count != null ? scheduler.candidate_count : 'N/A') + '</span></div>',
        profileRows || '<div class="field"><span class="label">Candidate Profiles</span><span class="value">N/A</span></div>'
    ].join('');
    schedulerContainer.innerHTML = [
        '<div class="alert info">Mode: Dry Run (Shadow Only, no upstream called)</div>',
        '<div class="field"><span class="label">Selected Account</span><span class="value">' + (scheduler.selected_account_id || 'None') + '</span></div>',
        '<div class="field"><span class="label">Model</span><span class="value">' + (scheduler.selected_model || 'None') + '</span></div>',
        '<div class="field"><span class="label">Candidate Models</span><span class="value">' + (scheduler.candidate_count != null ? scheduler.candidate_count : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Candidate List</span><span class="value" style="font-size:0.85em">' + escapeHtml(candidateModels) + '</span></div>',
        '<div class="field"><span class="label">Selection Source</span><span class="value">' + (scheduler.decision_source || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Pool</span><span class="value">' + (scheduler.requested_pool || 'None') + '</span></div>',
        '<div class="field"><span class="label">Tier (Scheduler)</span><span class="value highlight">' + schedTier + '</span></div>',
        '<div class="field"><span class="label">Tier (Rule)</span><span class="value highlight">' + ruleTier + '</span></div>',
        '<div class="field"><span class="label">Tier Agreement</span><span class="value ' + (tierAgree ? 'success' : 'error') + '">' + (tierAgree ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Layer</span><span class="value">' + (scheduler.scheduler_source || 'None') + '</span></div>',
        '<div class="field"><span class="label">Capability Match</span><span class="value">' + (scheduler.capability_match !== false ? 'Yes' : 'No') + '</span></div>',
        (scheduler.fallback_reason ? '<div class="alert warning">Fallback: ' + scheduler.fallback_reason + '</div>' : '')
    ].join('');
}

// Handle Copy button
function handleCopy() {
    if (!lastResult) {
        showError('No result to copy');
        return;
    }
    navigator.clipboard.writeText(JSON.stringify(lastResult, null, 2))
        .then(() => {
            const btn = $('copy-btn');
            const originalText = btn.textContent;
            btn.textContent = 'Copied!';
            setTimeout(() => btn.textContent = originalText, 2000);
        })
        .catch(err => showError('Copy failed: ' + err.message));
}

// Handle Raw JSON toggle
function handleToggleRaw() {
    const content = $('raw-content');
    const btn = $('toggle-raw-btn');
    if (!content || !btn) return;
    if (!lastResult) {
        showError('Please run a prompt first');
        return;
    }
    content.classList.toggle('visible');
    btn.textContent = content.classList.contains('visible') ? 'Hide Raw JSON' : 'Show Raw JSON';
}

// Show error message
function showError(message) {
    alert(message);
}

// Format list helper
function formatList(arr) {
    if (!arr || arr.length === 0) return '<span class="value empty">None</span>';
    return arr.join(', ');
}

function escapeHtml(str) {
    if (!str) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

// ============================================================================
// NEW FEATURES: Tab switching, Save Result, Dataset, History
// ============================================================================

// Tab switching
function switchTab(tabName) {
    document.querySelectorAll('.tab-btn').forEach(function(btn) {
        btn.classList.toggle('active', btn.dataset.tab === tabName);
    });
    document.querySelectorAll('.tab-content').forEach(function(content) {
        content.classList.toggle('active', content.id === 'tab-' + tabName);
    });
    if (tabName === 'history') {
        loadHistory();
    }
}

// (Event listeners merged into the single DOMContentLoaded above)

// Save Result
async function saveResult() {
    if (!lastResult) {
        setSaveMsg('No result to save', 'error');
        return;
    }
    var btn = $('save-result-btn');
    btn.disabled = true;
    btn.textContent = 'Saving...';
    setSaveMsg('', '');
    try {
        var body = {
            prompt: $('prompt').value.trim(),
            has_image: $('has-image').checked,
            has_document: $('has-document').checked,
            has_csv: $('has-csv').checked,
            run_id: 'single-' + Date.now(),
            row_id: '' + Date.now()
        };
        var resp = await fetch('/v1/debug/records/save', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        var data = await resp.json();
        if (data.saved) {
            setSaveMsg('Saved (record_id: ' + data.record_id + ')', 'success');
        } else {
            setSaveMsg('Save failed: ' + (data.error || 'unknown'), 'error');
        }
    } catch (err) {
        setSaveMsg('Save error: ' + err.message, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = 'Save Result';
    }
}

function setSaveMsg(msg, type) {
    var el = $('save-result-msg');
    if (!el) return;
    el.textContent = msg;
    el.className = 'save-result-msg';
    if (type) el.classList.add('save-result-msg--' + type);
}

// ============================================================================
// DATASET IMPORT
// ============================================================================

var datasetState = {
    rows: [],
    promptColumn: '',
    confirmed: false,
    running: false,
    stopRequested: false,
    results: []
};

function datasetPreview() {
    var fileInput = $('dataset-file-input');
    if (!fileInput.files || fileInput.files.length === 0) {
        alert('Please select a file first');
        return;
    }
    var file = fileInput.files[0];
    var reader = new FileReader();
    reader.onload = function(e) {
        var content = e.target.result;
        var rows = [];
        var ext = file.name.split('.').pop().toLowerCase();
        if (ext === 'jsonl') {
            var lines = content.split('\n').filter(function(l) { return l.trim(); });
            for (var i = 0; i < lines.length; i++) {
                try { rows.push(JSON.parse(lines[i])); } catch (ex) {}
            }
        } else if (ext === 'csv') {
            var lines2 = content.split('\n').filter(function(l) { return l.trim(); });
            if (lines2.length < 2) { alert('CSV must have header + data rows'); return; }
            var headers = lines2[0].split(',').map(function(h) { return h.trim().replace(/^"|"$/g, ''); });
            for (var j = 1; j < lines2.length; j++) {
                var vals = lines2[j].split(',').map(function(v) { return v.trim().replace(/^"|"$/g, ''); });
                var row = {};
                for (var k = 0; k < headers.length; k++) { row[headers[k]] = vals[k] || ''; }
                rows.push(row);
            }
        } else {
            alert('Unsupported format. Use .jsonl or .csv');
            return;
        }
        datasetState.rows = rows;
        var candidates = ['prompt', 'text', 'content', 'instruction', 'query', 'question', 'user_message'];
        var detected = '';
        if (rows.length > 0) {
            var keys = Object.keys(rows[0]);
            for (var ci = 0; ci < candidates.length; ci++) {
                if (keys.indexOf(candidates[ci]) !== -1) { detected = candidates[ci]; break; }
            }
        }
        datasetState.promptColumn = detected;
        $('dataset-detected-column').textContent = detected || 'Not detected';
        $('dataset-confirm-column-btn').disabled = false;
        $('dataset-column-input').value = detected;
        var previewSection = $('dataset-preview-section');
        previewSection.style.display = 'block';
        var previewHtml = '<table class="history-table"><thead><tr>';
        if (rows.length > 0) {
            var sampleKeys = Object.keys(rows[0]);
            for (var si = 0; si < sampleKeys.length; si++) { previewHtml += '<th>' + escapeHtml(sampleKeys[si]) + '</th>'; }
        }
        previewHtml += '</tr></thead><tbody>';
        var previewCount = Math.min(5, rows.length);
        for (var pi = 0; pi < previewCount; pi++) {
            previewHtml += '<tr>';
            for (var pk in rows[pi]) {
                var cellVal = String(rows[pi][pk]);
                var isPromptCol = pk.toLowerCase().indexOf('prompt') !== -1 || pk.toLowerCase().indexOf('text') !== -1 || pk.toLowerCase().indexOf('content') !== -1 || pk.toLowerCase().indexOf('instruction') !== -1;
                if (isPromptCol) {
                    previewHtml += '<td class="prompt-cell" title="' + escapeHtml(cellVal) + '"><span class="prompt-text">' + escapeHtml(cellVal) + '</span></td>';
                } else {
                    previewHtml += '<td>' + escapeHtml(cellVal.substring(0, 40)) + '</td>';
                }
            }
            previewHtml += '</tr>';
        }
        previewHtml += '</tbody></table>';
        previewHtml += '<p class="field"><span class="label">Total rows: ' + rows.length + '</span></p>';
        $('dataset-preview-table').innerHTML = previewHtml;
        datasetState.results = [];
        datasetState.confirmed = false;
        $('dataset-results-section').style.display = 'none';
        $('dataset-progress').style.display = 'none';
        $('dataset-run-btn').disabled = true;
    };
    reader.readAsText(file);
}

function datasetConfirmColumn() {
    var col = $('dataset-column-input').value.trim();
    if (!col) { alert('Enter a column name first'); return; }
    datasetState.promptColumn = col;
    datasetState.confirmed = true;
    datasetState.results = [];
    $('dataset-run-btn').disabled = false;
    $('dataset-detected-column').textContent = col + ' confirmed';
}

function datasetColumnManualInput() {
    $('dataset-confirm-column-btn').disabled = false;
}

async function datasetRunBatch() {
    if (!datasetState.confirmed || datasetState.rows.length === 0) return;
    datasetState.running = true;
    datasetState.stopRequested = false;
    var col = datasetState.promptColumn;
    $('dataset-run-btn').disabled = true;
    $('dataset-stop-btn').disabled = false;
    var progressSection = $('dataset-progress');
    progressSection.style.display = 'block';
    var total = datasetState.rows.length;
    var results = [];
    for (var i = 0; i < total; i++) {
        if (datasetState.stopRequested) break;
        var row = datasetState.rows[i];
        var prompt = row[col] || '';
        var pct = Math.round((i / total) * 100);
        $('dataset-progress-fill').style.width = pct + '%';
        $('dataset-progress-text').textContent = (i + 1) + ' / ' + total;
        try {
            var resp = await fetch('/v1/debug/events/evaluate', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ prompt: prompt, has_image: !!row.has_image, has_document: !!row.has_document, has_csv: !!row.has_data, mode: 'baseline' })
            });
            var data = await resp.json();
            await fetch('/v1/debug/records/save', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ prompt: prompt, has_image: !!row.has_image, has_document: !!row.has_document, has_csv: !!row.has_data, run_id: 'batch-' + Date.now(), row_id: '' + i })
            });
            var localPool = (data.local_provider_result || {}).mapped_pool || '';
            var poolToGroup = {'code_pool':'technical_models','data_pool':'technical_models','document_pool':'technical_models','vision_pool':'vision_models','image_generation_pool':'image_models','general_text_pool':'general_chat_models','cheap_chat_pool':'general_chat_models','general_pool':'general_chat_models'};
            var localGroup = poolToGroup[localPool] || '';
            if (!localGroup) {
                localGroup = (data.pool_card || {}).physical_model_group || '';
            }
            var officialPool = (data.official_provider_result || {}).mapped_pool || '';
            var officialGroup = '';
            if (officialPool) {
                officialGroup = poolToGroup[officialPool] || '';
            }
            // Fallback: if official group unknown, use local group (no evidence of disagreement)
            if (!officialGroup) {
                officialGroup = localGroup;
            }
            var groupAgree = localGroup && officialGroup && localGroup === officialGroup;

            // V2 fields from shadow decision
            var v2 = data.v2_decision || {};
            var v2LocalGroup = (v2.local || {}).group || '';
            var v2OfficialGroup = (v2.official || {}).group || '';

            results.push({ index: i, prompt: prompt.substring(0, 50), local_pool: localPool, official_pool: officialPool, physical_group: localGroup, official_group: officialGroup, group_agreement: groupAgree, tier: (data.tier_card || {}).selected_tier || '', error: '', v2_local_group: v2LocalGroup, v2_official_group: v2OfficialGroup });
        } catch (err) {
            results.push({ index: i, prompt: prompt.substring(0, 50), local_pool: 'ERROR', official_pool: '', tier: '', error: err.message });
        }
    }
    $('dataset-progress-fill').style.width = '100%';
    $('dataset-progress-text').textContent = results.length + ' / ' + total;
    datasetState.running = false;
    $('dataset-run-btn').disabled = false;
    $('dataset-stop-btn').disabled = true;
    datasetState.results = results;
    var resultsSection = $('dataset-results-section');
    resultsSection.style.display = 'block';
    datasetRenderResults();
}

function datasetStopBatch() { datasetState.stopRequested = true; $('dataset-stop-btn').disabled = true; }

function datasetRenderResults() {
    // Compute V2 Group Agreement stats
    var total = datasetState.results.length;
    var v2AgreeCount = 0;
    var v2DisagreeCount = 0;
    var v2NoGroupCount = 0;
    for (var i = 0; i < total; i++) {
        var r = datasetState.results[i];
        var v2Local = r.v2_local_group || '';
        var v2Official = r.v2_official_group || '';
        if (v2Local && v2Official && v2Local === v2Official) {
            v2AgreeCount++;
        } else if (v2Local && v2Official) {
            v2DisagreeCount++;
        } else {
            v2NoGroupCount++;
        }
    }
    var v2AgreeRate = total > 0 ? (v2AgreeCount / total * 100).toFixed(1) : '0.0';

    var html = '<div class="stats-summary">';
    html += '<div class="stats-row"><span class="stats-label">Total Samples:</span><span class="stats-value">' + total + '</span></div>';
    html += '<div class="stats-row"><span class="stats-label">V2 Group Agreement:</span><span class="stats-value highlight">' + v2AgreeCount + '/' + total + ' (' + v2AgreeRate + '%)</span></div>';
    html += '<div class="stats-row"><span class="stats-label">V2 Group Disagreement:</span><span class="stats-value' + (v2DisagreeCount > 0 ? ' warning' : '') + '">' + v2DisagreeCount + '</span></div>';
    if (v2NoGroupCount > 0) {
        html += '<div class="stats-row"><span class="stats-label">No V2 Group:</span><span class="stats-value">' + v2NoGroupCount + '</span></div>';
    }
    html += '</div>';

    html += '<table class="history-table"><thead><tr><th>#</th><th>Prompt</th><th>Local Pool</th><th>Official Pool</th><th>Local Group</th><th>Official Group</th><th>Group Agree</th><th>Tier</th><th>V2 Local Group</th><th>V2 Official Group</th><th>V2 Agree</th><th>Error</th></tr></thead><tbody>';
    for (var i = 0; i < datasetState.results.length; i++) {
        var r = datasetState.results[i];
        var localGroup = r.physical_group || '';
        var officialGroup = r.official_group || '';
        var groupAgree = r.group_agreement;
        if (!groupAgree && localGroup && officialGroup && localGroup === officialGroup) {
            groupAgree = true;
        }
        var groupAgreeIcon = groupAgree ? '✓' : (localGroup ? '✗' : '—');
        var groupAgreeClass = groupAgree ? 'agree-yes' : (localGroup ? 'agree-no' : '');
        var v2Local = r.v2_local_group || '';
        var v2Official = r.v2_official_group || '';
        var v2Agree = v2Local && v2Official && v2Local === v2Official;
        var v2AgreeIcon = v2Agree ? '✓' : (v2Local ? '✗' : '—');
        html += '<tr><td>' + r.index + '</td><td>' + escapeHtml(r.prompt) + '</td><td>' + escapeHtml(r.local_pool) + '</td><td>' + escapeHtml(r.official_pool) + '</td><td>' + escapeHtml(localGroup) + '</td><td>' + escapeHtml(officialGroup) + '</td><td class="' + groupAgreeClass + '">' + groupAgreeIcon + '</td><td>' + escapeHtml(r.tier) + '</td><td>' + escapeHtml(v2Local) + '</td><td>' + escapeHtml(v2Official) + '</td><td>' + v2AgreeIcon + '</td><td>' + (r.error ? '<span class="error-text">' + escapeHtml(r.error) + '</span>' : '') + '</td></tr>';
    }
    html += '</tbody></table>';
    $('dataset-results-table').innerHTML = html;
}

function datasetExportResults() {
    var blob = new Blob([JSON.stringify(datasetState.results, null, 2)], { type: 'application/json' });
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url; a.download = 'batch_results.json';
    document.body.appendChild(a); a.click(); document.body.removeChild(a); URL.revokeObjectURL(url);
}

// ============================================================================
// HISTORY
// ============================================================================

async function loadHistory() {
    var container = $('history-table-wrapper');
    if (!container) return;
    container.innerHTML = '<div class="field"><span class="label">' + t('loading') + '</span></div>';
    var params = new URLSearchParams();
    var sourceTypeEl = $('hist-filter-source-type');
    if (sourceTypeEl && sourceTypeEl.value) params.set('source_type', sourceTypeEl.value);
    var runIdEl = $('hist-filter-run-id');
    if (runIdEl && runIdEl.value) params.set('run_id', runIdEl.value);
    var localPoolEl = $('hist-filter-local-pool');
    if (localPoolEl && localPoolEl.value) params.set('local_pool', localPoolEl.value);
    var officialPoolEl = $('hist-filter-official-pool');
    if (officialPoolEl && officialPoolEl.value) params.set('official_pool', officialPoolEl.value);
    var tierEl = $('hist-filter-tier');
    if (tierEl && tierEl.value) params.set('tier', tierEl.value);
    var agreeEl = $('hist-filter-pool-agreement');
    if (agreeEl && agreeEl.value === 'yes') params.set('pool_agreement', 'true');
    else if (agreeEl && agreeEl.value === 'no') params.set('pool_agreement', 'false');
    var revEl = $('hist-filter-has-review');
    if (revEl && revEl.value === 'yes') params.set('has_review', 'true');
    else if (revEl && revEl.value === 'no') params.set('has_review', 'false');
    params.set('limit', '50');
    try {
        var resp = await fetch('/v1/debug/records/list?' + params.toString());
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        var data = await resp.json();
        var records = data.records || [];
        var total = data.total || 0;
        if (records.length === 0) {
            container.innerHTML = '<div class="field"><span class="label">No records found (total: ' + total + ')</span></div>';
        } else {
            var html = '<div class="field"><span class="label">' + t('local_records') + '：' + records.length + ' / ' + total + '</span></div>';
            html += '<table class="history-table"><thead><tr><th>ID</th><th>Prompt</th><th>Local Pool</th><th>Official Pool</th><th>Local Group</th><th>Official Group</th><th>Group Agree</th><th>Pool Agree</th><th>Tier</th><th>V2 Local</th><th>V2 Official</th><th>V2 Agree</th><th>Reviewed</th><th>Actions</th></tr></thead><tbody>';
            for (var ri = 0; ri < records.length; ri++) {
                var rec = records[ri];
                var review = rec.review;
                var preview = (rec.prompt_text || rec.prompt_preview || '');

                // Compute groups from pool names using a consistent mapping,
                // instead of relying on server-computed values (which may be stale).
                var poolToGroup = {'code_pool':'technical_models','data_pool':'technical_models','document_pool':'technical_models','vision_pool':'vision_models','image_generation_pool':'image_models','general_text_pool':'general_chat_models','cheap_chat_pool':'general_chat_models','general_pool':'general_chat_models'};
                var localPool = rec.local_pool || '';
                var officialPool = rec.official_pool || '';
                var localGroup = poolToGroup[localPool] || rec.physical_model_group || 'N/A';
                var officialGroup = poolToGroup[officialPool] || rec.official_physical_group || localGroup;
                var groupAgree = (localGroup !== 'N/A' && officialGroup !== 'N/A' && localGroup === officialGroup);
                var groupAgreeClass = groupAgree ? 'agree-yes' : 'agree-no';
                var v2Local = rec.v2_local_group || '';
                var v2Official = rec.v2_official_group || '';
                var v2Agree = v2Local && v2Official && v2Local === v2Official;
                html += '<tr><td>' + rec.id + '</td><td class="prompt-cell" title="' + escapeHtml(preview) + '"><span class="prompt-text">' + escapeHtml(preview) + '</span></td><td>' + escapeHtml(rec.local_pool || '') + '</td><td>' + escapeHtml(rec.official_pool || '') + '</td><td>' + escapeHtml(localGroup) + '</td><td>' + escapeHtml(officialGroup) + '</td><td class="' + groupAgreeClass + '">' + (groupAgree ? 'Yes' : 'No') + '</td><td>' + (rec.pool_agreement ? 'Yes' : 'No') + '</td><td>' + escapeHtml(rec.selected_tier || '') + '</td><td>' + escapeHtml(v2Local) + '</td><td>' + escapeHtml(v2Official) + '</td><td class="' + (v2Agree ? 'agree-yes' : 'agree-no') + '">' + (v2Agree ? 'Yes' : 'No') + '</td><td>' + (review ? 'Yes' : 'No') + '</td><td><button class="secondary small" onclick="openReview(' + rec.id + ')">Review</button></td></tr>';
                if (review) {
                    html += '<tr class="detail-row"><td colspan="8">Expected Pool: ' + escapeHtml(review.expected_pool || 'N/A') + ' | Tier: ' + escapeHtml(review.expected_tier || 'N/A') + ' | Conf: ' + escapeHtml(review.review_confidence || 'N/A') + ' | Ambiguous: ' + (review.ambiguous ? 'Yes' : 'No');
                    if (review.review_note) html += ' | Note: ' + escapeHtml(review.review_note);
                    html += '</td></tr>';
                }
            }
            html += '</tbody></table>';
            container.innerHTML = html;
        }
    } catch (err) {
        container.innerHTML = '<div class="field error"><span class="label">本地记录加载失败: ' + escapeHtml(err.message) + '</span></div>';
    }
    await loadSelectorHistory();
}

async function loadSelectorHistory() {
    var container = $('history-table-wrapper');
    if (!container) return;
    try {
        var resp = await fetch('/v1/model-selector/history?limit=50');
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        var envelope = await resp.json();
        var records = (envelope.data || {}).records || [];
        var html = '<div class="field"><span class="label">' + t('selector_records') + '：' + records.length + ' 条（每 5 秒自动刷新，Shadow Only）</span></div>';
        if (records.length === 0) {
            html += '<div class="field"><span class="label">' + t('no_selector_records') + '</span></div>';
        } else {
            html += '<table class="history-table"><thead><tr><th>' + t('time') + '</th><th>' + t('request_id') + '</th><th>' + t('prompt_summary') + '</th><th>' + t('candidates') + '</th><th>' + t('top_model') + '</th><th>Action</th></tr></thead><tbody>';
            for (var i = 0; i < records.length; i++) {
                var rec = records[i];
                var top = (rec.model_score_list || [])[0] || {};
                html += '<tr><td>' + escapeHtml(formatHistoryTime(rec.created_at)) + '</td><td>' + escapeHtml(rec.request_id || '-') + '</td><td class="prompt-cell" title="' + escapeHtml(rec.prompt_preview || '') + '"><span class="prompt-text">' + escapeHtml(rec.prompt_preview || '') + '</span></td><td>' + escapeHtml((rec.model_list || []).join(', ')) + '</td><td>' + escapeHtml(top.model_id || '-') + (top.score !== undefined ? ' (' + Number(top.score).toFixed(4) + ')' : '') + '</td><td><button class="secondary small" onclick="openSelectorDetail(' + escapeHtml(String(rec.id)) + ')">' + t('details') + '</button></td></tr>';
            }
            html += '</tbody></table>';
        }
        var detail = $('selector-history-detail');
        if (!detail) {
            detail = document.createElement('div');
            detail.id = 'selector-history-detail';
            detail.className = 'card';
            container.parentNode.insertBefore(detail, container.nextSibling);
        }
        var feed = $('selector-history-feed');
        if (!feed) {
            feed = document.createElement('div');
            feed.id = 'selector-history-feed';
            container.parentNode.insertBefore(feed, container);
        }
        feed.innerHTML = html;
        window.selectorHistoryRecords = records;
    } catch (err) {
        var feed = $('selector-history-feed');
        if (!feed) {
            feed = document.createElement('div'); feed.id = 'selector-history-feed'; container.parentNode.insertBefore(feed, container);
        }
        feed.innerHTML = '<div class="field error"><span class="label">' + t('selector_unavailable') + ' (' + escapeHtml(err.message) + ')</span></div>';
    }
}

function formatHistoryTime(value) {
    if (!value) return '-';
    var time = new Date(value);
    return isNaN(time.getTime()) ? value : time.toLocaleString('zh-CN', { hour12: false });
}

function openSelectorDetail(id) {
    var records = window.selectorHistoryRecords || [];
    var rec = records.find(function(item) { return String(item.id) === String(id); });
    var target = $('selector-history-detail');
    if (!rec || !target) return;
    var semanticRows = (rec.semantics || []).map(function(item) {
        return '<tr><td>' + escapeHtml(item.dimension || '') + '</td><td>' + Number(item.score || 0).toFixed(4) + '</td><td>' + escapeHtml(item.description || '') + '</td></tr>';
    }).join('');
    var modelRows = (rec.model_score_list || []).map(function(item, index) {
        return '<tr><td>' + (index + 1) + '</td><td>' + escapeHtml(item.model_id || '') + '</td><td>' + Number(item.score || 0).toFixed(4) + '</td></tr>';
    }).join('');
    target.innerHTML = '<h3>调用详情 #' + escapeHtml(String(rec.id)) + '</h3>' +
        '<div class="field"><span class="label">请求 ID</span><span class="value">' + escapeHtml(rec.request_id || '-') + '</span></div>' +
        '<div class="field"><span class="label">Prompt 摘要</span><span class="value">' + escapeHtml(rec.prompt_preview || '-') + '</span></div>' +
        '<div class="field"><span class="label">解码请求</span><pre class="raw-json">' + escapeHtml(JSON.stringify(rec.decoded_request || {}, null, 2)) + '</pre></div>' +
        '<h4>语义与 Official vLLM 分数</h4><table class="history-table"><thead><tr><th>维度</th><th>分数</th><th>说明</th></tr></thead><tbody>' + semanticRows + '</tbody></table>' +
        '<h4>候选模型最终分数</h4><table class="history-table"><thead><tr><th>排名</th><th>模型</th><th>最终分数</th></tr></thead><tbody>' + modelRows + '</tbody></table>' +
        '<div class="field"><span class="label">安全状态</span><span class="value">Shadow Only；未调用上游模型</span></div>';
    target.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

// Review Modal
function openReview(recordId) {
    var modal = $('review-modal');
    if (!modal) return;
    modal.style.display = 'block';
    var idEl = document.getElementById('review-record-id');
    if (idEl) idEl.textContent = '' + recordId;
    var poolEl = $('review-expected-pool');
    if (poolEl) poolEl.value = '';
    var tierEl = $('review-expected-tier');
    if (tierEl) tierEl.value = '';
    var reviewerEl = $('review-reviewer');
    if (reviewerEl) reviewerEl.value = '';
}

function reviewClose() { var modal = $('review-modal'); if (modal) modal.style.display = 'none'; }

async function reviewSubmit() {
    var idEl = document.getElementById('review-record-id');
    var recordId = parseInt(idEl ? idEl.textContent : '', 10);
    if (!recordId) { alert('Invalid record ID'); return; }
    var body = {
        record_id: recordId,
        expected_pool: ($('review-expected-pool') || {}).value || '',
        expected_tier: ($('review-expected-tier') || {}).value || '',
        review_confidence: ($('review-confidence') || {}).value || 'medium',
        ambiguous: ($('review-ambiguous') || {}).value === 'true',
        reviewer: ($('review-reviewer') || {}).value || '',
        review_note: ($('review-note') || {}).value || ''
    };
    try {
        var resp = await fetch('/v1/debug/records/review', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
        var data = await resp.json();
        if (data.saved) { alert('Review saved!'); reviewClose(); loadHistory(); }
        else { alert('Save failed: ' + (data.error || 'unknown')); }
    } catch (err) { alert('Error: ' + err.message); }
}

// ============================================================================
// New layout card renderers
// ============================================================================

function renderLocalProvider(result) {
    const container = $('local-provider');
    if (!container) return;
    const local = result.local_provider_result || {};
    container.innerHTML = `
        <div class="field"><span class="label">Provider</span><span class="value">${escapeHtml(local.provider || 'N/A')}</span></div>
        <div class="field"><span class="label">Decision</span><span class="value highlight">${escapeHtml(local.decision_name || 'N/A')}</span></div>
        <div class="field"><span class="label">Category</span><span class="value">${escapeHtml(local.semantic_category || 'N/A')}</span></div>
        <div class="field"><span class="label">Mapped Pool</span><span class="value highlight">${escapeHtml(local.mapped_pool || 'N/A')}</span></div>
        <div class="field"><span class="label">Confidence</span><span class="value">${(local.confidence || 0).toFixed(4)}</span></div>
        <div class="field"><span class="label">Used For Final</span><span class="value">${local.used_for_final ? 'Yes' : 'No'}</span></div>
        ${local.error ? `<div class="alert warning">${escapeHtml(local.error)}</div>` : ''}
    `;
}

function renderPoolCard(result) {
    const container = $('pool-card');
    if (!container) return;
    const pool = result.pool_card || {};
    const hybrid = result.hybrid_pool || {};
    container.innerHTML = `
        <div class="field"><span class="label">Logical Pool</span><span class="value highlight">${escapeHtml(pool.logical_pool || 'N/A')}</span></div>
        <div class="field"><span class="label">Physical Model Group</span><span class="value">${escapeHtml(pool.physical_model_group || 'N/A')}</span></div>
        <div class="field"><span class="label">Hybrid Candidate</span><span class="value">${escapeHtml(hybrid.candidate_pool || 'N/A')}</span></div>
        <div class="field"><span class="label">Hybrid Source</span><span class="value">${escapeHtml(hybrid.source || 'N/A')}</span></div>
        <div class="field"><span class="label">Pool Agreement</span><span class="value">${result.pool_agreement ? 'Yes' : 'No'}</span></div>
        <div class="field"><span class="label">Semantic Agreement</span><span class="value">${result.semantic_agreement ? 'Yes' : 'No'}</span></div>
    `;
}

function renderOfficialVLLM(result) {
    const container = $('official-vllm-card');
    if (!container) return;
    const off = result.official_provider_result || {};
    const shadow = result.official_vllm_semantic_shadow || {};
    container.innerHTML = `
        <div class="field"><span class="label">Provider</span><span class="value">${escapeHtml(off.provider || 'N/A')}</span></div>
        <div class="field"><span class="label">Decision</span><span class="value highlight">${escapeHtml(off.decision_name || 'N/A')}</span></div>
        <div class="field"><span class="label">Category</span><span class="value">${escapeHtml(off.semantic_category || 'N/A')}</span></div>
        <div class="field"><span class="label">Mapped Pool</span><span class="value">${escapeHtml(off.mapped_pool || 'N/A')}</span></div>
        <div class="field"><span class="label">Confidence</span><span class="value">${(off.confidence || 0).toFixed(4)}</span></div>
        <div class="field"><span class="label">Latency</span><span class="value">${(off.latency_ms || 0).toFixed(2)}ms</span></div>
        <div class="field"><span class="label">Shadow Only</span><span class="value">${!off.used_for_final ? 'Yes' : 'No'}</span></div>
        ${off.error ? `<div class="alert warning">${escapeHtml(off.error)}</div>` : ''}
    `;
}

// History export
async function historyExport() {
    try {
        var format = ($('hist-export-format') || {}).value || 'jsonl';
        var params = new URLSearchParams();
        params.set('format', format); params.set('limit', '10000');
        var resp = await fetch('/v1/debug/records/export?' + params.toString());
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        var blob = await resp.blob();
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = 'routing_records.' + (format === 'csv' ? 'csv' : 'jsonl');
        document.body.appendChild(a); a.click(); document.body.removeChild(a); URL.revokeObjectURL(url);
    } catch (err) { alert('Export failed: ' + err.message); }
}

function renderAgreementCard(result) {
    const container = $('agreement-card');
    if (!container) return;

    var localPool = (result.local_provider_result || {}).mapped_pool || 'N/A';
    var officialPool = (result.official_provider_result || {}).mapped_pool || 'N/A';

    // Compute groups from pool names using a consistent client-side mapping,
    // rather than relying on server-computed values which may be stale.
    var poolToGroup = {'code_pool':'technical_models','data_pool':'technical_models','document_pool':'technical_models','vision_pool':'vision_models','image_generation_pool':'image_models','general_text_pool':'general_chat_models','cheap_chat_pool':'general_chat_models','general_pool':'general_chat_models'};
    var localGroup = poolToGroup[localPool] || (result.pool_card||{}).physical_model_group || 'N/A';
    var officialGroup = poolToGroup[officialPool] || result.official_physical_group || localGroup;

    // Physical Group Agreement: computed from the mapped groups.
    var groupAgree = (localGroup !== 'N/A' && officialGroup !== 'N/A' && localGroup === officialGroup);

    container.innerHTML = [
        '<div class="field"><span class="label">Local Pool</span><span class="value">' + escapeHtml(localPool) + '</span></div>',
        '<div class="field"><span class="label">Official Pool</span><span class="value">' + escapeHtml(officialPool) + '</span></div>',
        '<div class="field"><span class="label">Local Group</span><span class="value">' + escapeHtml(localGroup) + '</span></div>',
        '<div class="field"><span class="label">Official Group</span><span class="value">' + escapeHtml(officialGroup) + '</span></div>',
        '<div class="field"><span class="label">Physical Group Agreement</span><span class="value ' + (groupAgree ? 'success' : 'error') + '">' + (groupAgree ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">RouteLLM Tier Agreement</span><span class="value ' + (result.routellm_agreement ? 'success' : 'error') + '">' + ((result.routellm_agreement !== undefined) ? (result.routellm_agreement ? 'Yes' : 'No') : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Pool Agreement</span><span class="value ' + (result.pool_agreement ? 'success' : 'error') + '">' + (result.pool_agreement ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Semantic Agreement</span><span class="value ' + (result.semantic_agreement ? 'success' : 'error') + '">' + (result.semantic_agreement ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Hybrid Candidate</span><span class="value">' + escapeHtml((result.hybrid_pool||{}).candidate_pool || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Hybrid Source</span><span class="value">' + escapeHtml((result.hybrid_pool||{}).source || 'N/A') + '</span></div>'
    ].join('');
}

// ============================================================================
// V2 Decision Trace Rendering
// ============================================================================

function renderV2Decision(result) {
    var v2 = result.v2_decision;
    if (!v2) {
        ['v2-task-understanding','v2-capability-window','v2-local','v2-official','v2-model-recommendation','v2-hybrid','v2-tool-profile','v2-tier','v2-agreement'].forEach(function(id) {
            var el = $(id);
            if (el) el.innerHTML = '<div class="field"><span class="label">V2 not available for this result</span></div>';
        });
        return;
    }

    renderV2TaskUnderstanding(v2.task_understanding);
    renderV2CapabilityWindow(v2.capability_window);
    renderV2Local(v2.local);
    renderV2Official(v2.official);
    renderV2ModelRecommendation(result.scheduler || {});
    renderV2Hybrid(v2.hybrid);
    renderV2ToolProfile(v2.tool_profile);
    renderV2Tier(v2.tier);
    renderV2Agreement(v2);
}

function renderV2ModelRecommendation(scheduler) {
    var el = $('v2-model-recommendation');
    if (!el) return;
    var ranking = scheduler.model_ranking || {};
    var rankingCandidates = Array.isArray(ranking.candidates) ? ranking.candidates : [];
    var details = Array.isArray(scheduler.candidate_details) ? scheduler.candidate_details : [];
    // Older Playground responses may not have model_ranking yet. Build the
    // same stable all-candidate view from candidate_details in that case.
    if (!rankingCandidates.length && details.length) {
        rankingCandidates = details.slice().sort(function (a, b) {
            var as = Number(a.final_score || a.profile_score || 0);
            var bs = Number(b.final_score || b.profile_score || 0);
            return bs - as;
        }).map(function (candidate, index) {
            return {
                rank: index + 1,
                account_id: candidate.account_id,
                model: candidate.model_id,
                final_score: Number(candidate.final_score || candidate.profile_score || 0)
            };
        });
        ranking = {
            version: 'model_ranking_v1',
            physical_group: scheduler.selected_pool || scheduler.requested_pool || '',
            recommended_account_id: rankingCandidates[0].account_id,
            recommended_model: rankingCandidates[0].model,
            ranking_margin: rankingCandidates.length > 1 ? rankingCandidates[0].final_score - rankingCandidates[1].final_score : 0,
            candidates: rankingCandidates,
            shadow_only: true,
            used_for_final: false
        };
    }
    lastModelRanking = rankingCandidates.length ? ranking : null;
    var rankingTools = $('candidate-ranking-tools');
    var rankingJSON = $('candidate-ranking-json');
    if (rankingTools && rankingJSON) {
        rankingTools.hidden = !lastModelRanking;
        rankingJSON.textContent = lastModelRanking ? JSON.stringify(lastModelRanking, null, 2) : '';
    }
    var candidates = rankingCandidates.length ? rankingCandidates.map(function (c) { return c.model; }).join(', ') : (Array.isArray(scheduler.candidate_models) ? scheduler.candidate_models.join(', ') : 'N/A');
    var profileRows = rankingCandidates.length ? rankingCandidates.map(function (candidate) {
        return '<div class="field"><span class="label">' + escapeHtml(candidate.model || 'unknown') + '</span><span class="value" style="font-size:0.82em">rank ' + candidate.rank + ' · final ' + Number(candidate.final_score || 0).toFixed(4) + '</span></div>';
    }).join('') : details.map(function (candidate) {
        var finalScore = candidate.final_score != null ? Number(candidate.final_score).toFixed(4) : 'N/A';
        var capability = candidate.capability_score != null ? Number(candidate.capability_score).toFixed(4) : 'N/A';
        var runtime = candidate.runtime_score != null ? Number(candidate.runtime_score).toFixed(4) : 'N/A';
        return '<div class="field"><span class="label">' + escapeHtml(candidate.model_id || 'unknown') + '</span><span class="value" style="font-size:0.82em">' + escapeHtml((candidate.provider || 'unknown') + ' · final ' + finalScore + ' · capability ' + capability + ' · runtime ' + runtime) + '</span></div>';
    }).join('');
    el.innerHTML = [
        '<div class="alert info">Shadow / DryRun only — does not affect V1 final pool</div>',
        '<div class="field"><span class="label">Recommended Model</span><span class="value highlight">' + escapeHtml(scheduler.recommended_model || scheduler.selected_model || 'None') + '</span></div>',
        '<div class="field"><span class="label">Old → New Model</span><span class="value">' + escapeHtml((scheduler.old_selected_model || scheduler.selected_model || 'None') + ' → ' + (scheduler.new_suggested_model || scheduler.recommended_model || 'None')) + '</span></div>',
        '<div class="field"><span class="label">Old/New Agreement</span><span class="value ' + (scheduler.old_vs_new_agreement ? 'success' : 'warning') + '">' + (scheduler.old_vs_new_agreement ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Ranking Margin</span><span class="value">' + (scheduler.ranking_margin != null ? Number(scheduler.ranking_margin).toFixed(4) : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Selected Account</span><span class="value highlight">' + escapeHtml(String(scheduler.selected_account_id || 'None')) + '</span></div>',
        '<div class="field"><span class="label">Pool</span><span class="value">' + escapeHtml(scheduler.selected_pool || scheduler.requested_pool || 'None') + '</span></div>',
        '<div class="field"><span class="label">Tier</span><span class="value">' + escapeHtml(scheduler.selected_tier || scheduler.requested_tier || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Scheduler Layer</span><span class="value">' + escapeHtml(scheduler.scheduler_layer || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Dry Run</span><span class="value">' + (scheduler.dry_run ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Upstream Called</span><span class="value">' + (scheduler.upstream_called ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Reason</span><span class="value" style="font-size:0.85em">' + escapeHtml(scheduler.recommendation_reason || scheduler.decision_source || 'N/A') + '</span></div>',
        (scheduler.fallback_reason ? '<div class="alert warning">Same-group fallback: ' + escapeHtml(scheduler.fallback_reason) + '</div>' : ''),
        '<div class="field"><span class="label">Candidates</span><span class="value" style="font-size:0.85em">' + escapeHtml(candidates) + '</span></div>',
        profileRows
    ].join('');
}

function handleCopyRanking() {
    if (!lastModelRanking) {
        showError('No candidate ranking available. Please run a prompt first.');
        return;
    }
    var text = JSON.stringify(lastModelRanking, null, 2);
    navigator.clipboard.writeText(text).then(function () {
        var btn = $('copy-ranking-btn');
        var original = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(function () { btn.textContent = original; }, 1500);
    }).catch(function (err) { showError('Copy failed: ' + err.message); });
}

function handleDownloadRanking() {
    if (!lastModelRanking) {
        showError('No candidate ranking available. Please run a prompt first.');
        return;
    }
    var blob = new Blob([JSON.stringify(lastModelRanking, null, 2) + '\n'], { type: 'application/json' });
    var url = URL.createObjectURL(blob);
    var link = document.createElement('a');
    link.href = url;
    link.download = 'model_ranking.json';
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
}

function renderV2TaskUnderstanding(tu) {
    var el = $('v2-task-understanding');
    if (!el) return;
    var mode = tu.output_mode || 'text';
    var rm = tu.requires_multimodal ? 'Yes' : 'No';
    var mmType = tu.multimodal_type || 'none';
    var domain = tu.domain || 'unknown';
    var tool = tu.tool_profile || 'none';
    var intent = tu.fine_intent || '-';

    var modeClass = mode === 'image' ? 'success' : (mode === 'video' ? 'warning' : '');
    el.innerHTML = [
        '<div class="field"><span class="label">Output Mode</span><span class="value ' + modeClass + '">' + escapeHtml(mode) + '</span></div>',
        '<div class="field"><span class="label">Requires Multimodal</span><span class="value ' + (tu.requires_multimodal ? 'success' : '') + '">' + rm + '</span></div>',
        '<div class="field"><span class="label">Multimodal Type</span><span class="value ' + (mmType !== 'none' ? 'info' : '') + '">' + escapeHtml(mmType) + '</span></div>',
        '<div class="field"><span class="label">Domain</span><span class="value ' + (domain === 'technical' ? 'highlight' : '') + '">' + escapeHtml(domain) + '</span></div>',
        '<div class="field"><span class="label">Tool Profile</span><span class="value">' + escapeHtml(tool) + '</span></div>',
        '<div class="field"><span class="label">Fine Intent</span><span class="value">' + escapeHtml(intent) + '</span></div>'
    ].join('');
}

function renderV2CapabilityWindow(cw) {
    var el = $('v2-capability-window');
    if (!el) return;
    var mmType = cw.multimodal_type || 'none';
    el.innerHTML = [
        '<div class="field"><span class="label">Has Image</span><span class="value">' + (cw.has_image ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Has Document</span><span class="value">' + (cw.has_document ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Has CSV</span><span class="value">' + (cw.has_csv ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Requires Multimodal</span><span class="value ' + (cw.requires_multimodal ? 'success' : '') + '">' + (cw.requires_multimodal ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Multimodal Type</span><span class="value ' + (mmType !== 'none' ? 'info' : '') + '">' + escapeHtml(mmType) + '</span></div>',
        '<div class="field"><span class="label">Chart Analysis</span><span class="value ' + (cw.chart_analysis ? 'success' : '') + '">' + (cw.chart_analysis ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Meme Analysis</span><span class="value ' + (cw.meme_analysis ? 'success' : '') + '">' + (cw.meme_analysis ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Text-Image Fusion</span><span class="value ' + (cw.text_image_fusion ? 'success' : '') + '">' + (cw.text_image_fusion ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Image Generation</span><span class="value ' + (cw.image_generation ? 'success' : '') + '">' + (cw.image_generation ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Video Generation</span><span class="value ' + (cw.video_generation ? 'warning' : '') + '">' + (cw.video_generation ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Multimodal by Metadata</span><span class="value">' + (cw.multimodal_by_metadata ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Detected Modalities</span><span class="value">' + escapeHtml((cw.detected_modalities||[]).join(', ')) + '</span></div>'
    ].join('');
}

function renderV2Local(local) {
    var el = $('v2-local');
    if (!el) return;
    var domain = local.domain || 'unknown';
    var group = local.group || 'unknown';
    el.innerHTML = [
        '<div class="field"><span class="label">Domain</span><span class="value ' + (domain === 'technical' ? 'highlight' : '') + '">' + escapeHtml(domain) + '</span></div>',
        '<div class="field"><span class="label">V2 Group</span><span class="value highlight">' + escapeHtml(group) + '</span></div>',
        '<div class="field"><span class="label">Confidence</span><span class="value">' + (local.confidence != null ? local.confidence.toFixed(4) : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Reason</span><span class="value">' + escapeHtml(local.reason || '') + '</span></div>',
        '<div class="field"><span class="label">Rule Type</span><span class="value">' + escapeHtml(local.rule_type || '') + '</span></div>'
    ].join('');
}

function renderV2Official(official) {
    var el = $('v2-official');
    if (!el) return;
    if (!official.available) {
        el.innerHTML = '<div class="field"><span class="label error">Official V2 not available</span></div>';
        return;
    }
    var domain = official.domain || 'unknown';
    var group = official.group || 'unknown';
    var officialScoreHtml = '';
    if (official.official_scores) {
        var officialKeys = Object.keys(official.official_scores).sort();
        var officialParts = [];
        for (var oi = 0; oi < officialKeys.length; oi++) {
            var ok = officialKeys[oi];
            officialParts.push(ok + ': ' + Number(official.official_scores[ok] || 0).toFixed(4));
        }
        officialScoreHtml = [
            '<div class="field"><span class="label">Official vLLM Scores</span><span class="value" style="font-size:0.85em">' + escapeHtml(officialParts.join(', ')) + '</span></div>',
            '<div class="field"><span class="label">Technical Score</span><span class="value highlight">' + Number(official.technical_score || 0).toFixed(4) + '</span></div>',
            '<div class="field"><span class="label">General Score</span><span class="value">' + Number(official.general_score || 0).toFixed(4) + '</span></div>',
            '<div class="field"><span class="label">Official Decision</span><span class="value">' + escapeHtml(official.official_decision || 'N/A') + '</span></div>',
            '<div class="field"><span class="label">Score Source</span><span class="value">' + escapeHtml(official.score_source || 'N/A') + '</span></div>'
        ].join('');
    }
    // Build E5 scores display
    var e5Html = '';
    if (official.e5_scores) {
        var sorted = Object.keys(official.e5_scores).sort();
        var scoreParts = [];
        for (var si = 0; si < sorted.length; si++) {
            var k = sorted[si];
            scoreParts.push(k + ': ' + official.e5_scores[k].toFixed(4));
        }
        e5Html = '<div class="field"><span class="label">E5 Scores</span><span class="value" style="font-size:0.85em">' + escapeHtml(scoreParts.join(', ')) + '</span></div>';
    }
    el.innerHTML = [
        '<div class="field"><span class="label">Domain</span><span class="value ' + (domain === 'technical' ? 'highlight' : '') + '">' + escapeHtml(domain) + '</span></div>',
        '<div class="field"><span class="label">Domain Score</span><span class="value">' + (official.domain_score != null ? official.domain_score.toFixed(4) : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Second Domain</span><span class="value">' + escapeHtml(official.second_domain || '') + '</span></div>',
        '<div class="field"><span class="label">Domain Margin</span><span class="value">' + (official.domain_margin != null ? official.domain_margin.toFixed(4) : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">V2 Group</span><span class="value highlight">' + escapeHtml(group) + '</span></div>',
        '<div class="field"><span class="label">Fine Intent</span><span class="value">' + escapeHtml(official.fine_intent || '') + '</span></div>',
        '<div class="field"><span class="label">Tool Profile</span><span class="value">' + escapeHtml(official.tool_profile || 'none') + '</span></div>',
        officialScoreHtml,
        e5Html
    ].join('');
}

function renderV2Hybrid(hybrid) {
    var el = $('v2-hybrid');
    if (!el) return;
    el.innerHTML = [
        '<div class="field"><span class="label">Triggered</span><span class="value ' + (hybrid.triggered ? 'warning' : '') + '">' + (hybrid.triggered ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Disagreement</span><span class="value ' + (hybrid.disagreement ? 'error' : 'success') + '">' + (hybrid.disagreement ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Candidate Group</span><span class="value">' + escapeHtml(hybrid.candidate_group || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Override Eligible</span><span class="value ' + (hybrid.override_eligible ? 'warning' : '') + '">' + (hybrid.override_eligible ? 'Yes' : 'No') + '</span></div>',
        '<div class="field"><span class="label">Override Threshold</span><span class="value">' + (hybrid.override_confidence_threshold != null ? Number(hybrid.override_confidence_threshold).toFixed(2) : 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Override Reason</span><span class="value">' + escapeHtml(hybrid.override_reason || 'N/A') + '</span></div>',
        '<div class="field"><span class="label">Used For Final</span><span class="value">' + (hybrid.used_for_final ? 'Yes' : 'No') + '</span></div>'
    ].join('');
}

function renderV2ToolProfile(tp) {
    var el = $('v2-tool-profile');
    if (!el) return;
    var secondary = (tp.secondary || []).join(', ');
    el.innerHTML = [
        '<div class="field"><span class="label">Primary Tool</span><span class="value highlight">' + escapeHtml(tp.primary || 'none') + '</span></div>',
        '<div class="field"><span class="label">Secondary Tools</span><span class="value">' + escapeHtml(secondary || 'none') + '</span></div>',
        '<div class="field"><span class="label">Reason</span><span class="value">' + escapeHtml(tp.reason || '') + '</span></div>'
    ].join('');
}

function renderV2Tier(tier) {
    var el = $('v2-tier');
    if (!el) return;
    el.innerHTML = [
        '<div class="field"><span class="label">Complexity Candidate</span><span class="value">' + escapeHtml(tier.complexity_candidate_tier || '') + '</span></div>',
        '<div class="field"><span class="label">RouteLLM Candidate</span><span class="value">' + escapeHtml(tier.routellm_candidate_tier || '') + '</span></div>',
        '<div class="field"><span class="label">Policy Minimum</span><span class="value">' + escapeHtml(tier.policy_minimum_tier || '') + '</span></div>',
        '<div class="field"><span class="label">Selected Tier</span><span class="value highlight">' + escapeHtml(tier.selected_tier || '') + '</span></div>',
        '<div class="field"><span class="label">Selected Tier Source</span><span class="value">' + escapeHtml(tier.selected_tier_source || '') + '</span></div>',
        '<div class="field"><span class="label">Tier Agreement</span><span class="value ' + (tier.tier_agreement ? 'success' : 'error') + '">' + (tier.tier_agreement ? 'Yes' : 'No') + '</span></div>'
    ].join('');
}

function renderV2Agreement(v2) {
    var el = $('v2-agreement');
    if (!el) return;
    var localGroup = (v2.local || {}).group || 'N/A';
    var officialGroup = (v2.official || {}).group || 'N/A';
    var agree = (localGroup !== 'N/A' && officialGroup !== 'N/A' && localGroup === officialGroup);
    el.innerHTML = [
        '<div class="field"><span class="label">Local V2 Group</span><span class="value">' + escapeHtml(localGroup) + '</span></div>',
        '<div class="field"><span class="label">Official V2 Group</span><span class="value">' + escapeHtml(officialGroup) + '</span></div>',
        '<div class="field"><span class="label">V2 Group Agreement</span><span class="value ' + (agree ? 'success' : 'error') + '">' + (agree ? 'Yes' : 'No') + '</span></div>'
    ].join('');
}
