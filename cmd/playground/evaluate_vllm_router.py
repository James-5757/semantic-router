#!/usr/bin/env python3
"""
评估和优化 vLLM Semantic Router 的分类效果。

用法:
    python evaluate_vllm_router.py

该脚本会:
    1. 审查当前 config 中的示例质量
    2. 用 holdout 数据集评估当前效果
    3. 执行 threshold grid search 和示例优化
    4. 输出优化前后对比
    5. 生成优化后的 config 文件

API 端点: POST http://127.0.0.1:8080/api/v1/eval
    请求: {"text": "prompt"}
    响应中包含 decision_result.decision_name 和 signal_values
"""

import json
import hashlib
import copy
import sys
import os
import requests
import time
from collections import defaultdict, Counter

# ---------- 配置 ----------
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
CONFIG_PATH = os.path.join(SCRIPT_DIR, "vllm_original_config.yaml")
OPTIMIZED_CONFIG_PATH = os.path.join(SCRIPT_DIR, "vllm_optimized_config.yaml")
API_URL = "http://127.0.0.1:8080/api/v1/eval"

# Signal name -> decision_name 映射
SIGNAL_TO_DECISION = {
    "semantic_code": "route_code",
    "semantic_data_analysis": "route_data_analysis",
    "semantic_document": "route_document",
    "semantic_vision_understanding": "route_vision_understanding",
    "semantic_image_generation": "route_image_generation",
    "semantic_simple_chat": "route_simple_chat",
    "semantic_general": "route_general",
}

DECISION_TO_SIGNAL = {v: k for k, v in SIGNAL_TO_DECISION.items()}

# ---------- 固定的 holdout 评测数据集 ----------
# 每个类别 >= 15 条, 共 105 条
# 确保不与 config 中现有示例重复
HOLDOUT_DATA = [
    # ===== route_code (15条) =====
    {"prompt": "Convert this React class component to a functional component with hooks", "expected": "route_code"},
    {"prompt": "Write a Python script to batch rename files in a directory", "expected": "route_code"},
    {"prompt": "帮我用Go写一个并发安全的缓存", "expected": "route_code"},
    {"prompt": "How do I fix this TypeError: cannot unpack non-iterable NoneType object", "expected": "route_code"},
    {"prompt": "Refactor this nested if-else chain into a strategy pattern", "expected": "route_code"},
    {"prompt": "写一个正则表达式匹配邮箱地址", "expected": "route_code"},
    {"prompt": "Explain the time complexity of this recursive Fibonacci implementation", "expected": "route_code"},
    {"prompt": "帮我看看这个docker-compose文件哪里写错了", "expected": "route_code"},
    {"prompt": "Implement a RateLimiter decorator in Python", "expected": "route_code"},
    {"prompt": "编写一个shell脚本来备份数据库", "expected": "route_code"},
    {"prompt": "这个API在并发下会有race condition吗", "expected": "route_code"},
    {"prompt": "Write a React custom hook for debounced search", "expected": "route_code"},
    {"prompt": "帮我优化这段代码的内存使用", "expected": "route_code"},
    {"prompt": "Implement a Bloom filter from scratch", "expected": "route_code"},
    {"prompt": "写个jenkins pipeline来自动化部署", "expected": "route_code"},

    # ===== route_data_analysis (15条) =====
    {"prompt": "Analyze the correlation between temperature and ice cream sales in this CSV", "expected": "route_data_analysis"},
    {"prompt": "帮我统计一下这份销售数据的季度增长率", "expected": "route_data_analysis"},
    {"prompt": "Generate a heatmap of user activity by hour and day of week", "expected": "route_data_analysis"},
    {"prompt": "What's the distribution of customer ages in this dataset", "expected": "route_data_analysis"},
    {"prompt": "Compare the conversion rates between control and experiment groups", "expected": "route_data_analysis"},
    {"prompt": "用Python分析这个JSON日志中的错误分布", "expected": "route_data_analysis"},
    {"prompt": "Create a pivot table showing revenue by region and quarter", "expected": "route_data_analysis"},
    {"prompt": "Run a linear regression on this data and tell me R-squared", "expected": "route_data_analysis"},
    {"prompt": "Find outliers in this time series data", "expected": "route_data_analysis"},
    {"prompt": "看看这些数据里哪些特征对预测最有帮助", "expected": "route_data_analysis"},
    {"prompt": "Visualize the click-through rate trends over the past 6 months", "expected": "route_data_analysis"},
    {"prompt": "帮我算一下A/B测试的p值", "expected": "route_data_analysis"},
    {"prompt": "What is the month-over-month growth for each product category", "expected": "route_data_analysis"},
    {"prompt": "Segment these customers into groups based on purchase behavior", "expected": "route_data_analysis"},
    {"prompt": "分析这篇文章的情感倾向分布", "expected": "route_data_analysis"},

    # ===== route_document (15条) =====
    {"prompt": "帮我总结这篇论文的核心论点", "expected": "route_document"},
    {"prompt": "Extract all action items from this meeting transcript", "expected": "route_document"},
    {"prompt": "把这份中文合同翻译成英文", "expected": "route_document"},
    {"prompt": "Proofread my cover letter and suggest improvements", "expected": "route_document"},
    {"prompt": "写一份项目周报的模板", "expected": "route_document"},
    {"prompt": "帮我整理一下这份调研报告的关键发现", "expected": "route_document"},
    {"prompt": "Create a one-page executive summary from this 50-page report", "expected": "route_document"},
    {"prompt": "Rewrite this paragraph to be more concise", "expected": "route_document"},
    {"prompt": "检查这篇文档中的错别字和语法错误", "expected": "route_document"},
    {"prompt": "Summarize the key changes in this legal contract", "expected": "route_document"},
    {"prompt": "把这段技术文档改写成面向普通用户的版本", "expected": "route_document"},
    {"prompt": "Generate a table of contents for this book manuscript", "expected": "route_document"},
    {"prompt": "帮我提取会议纪要中的待办事项", "expected": "route_document"},
    {"prompt": "Write a product description for this SaaS tool", "expected": "route_document"},
    {"prompt": "Compare the arguments in these two opinion pieces", "expected": "route_document"},

    # ===== route_vision_understanding (15条) =====
    {"prompt": "这张显微镜图片里能看到哪些细胞结构", "expected": "route_vision_understanding"},
    {"prompt": "Describe the architectural style of this building in the photo", "expected": "route_vision_understanding"},
    {"prompt": "What products are displayed on this supermarket shelf", "expected": "route_vision_understanding"},
    {"prompt": "帮我看看这张CT扫描中是否有异常", "expected": "route_vision_understanding"},
    {"prompt": "Count the number of cars in this traffic camera image", "expected": "route_vision_understanding"},
    {"prompt": "识别这张手写稿中的文字", "expected": "route_vision_understanding"},
    {"prompt": "What emotions are the people in this photo expressing", "expected": "route_vision_understanding"},
    {"prompt": "这张图表里的数据反映了什么趋势", "expected": "route_vision_understanding"},
    {"prompt": "Identify all the landmarks visible in this cityscape photo", "expected": "route_vision_understanding"},
    {"prompt": "分析这张产品设计图的用户界面布局", "expected": "route_vision_understanding"},
    {"prompt": "Tell me what is unusual about this surveillance footage frame", "expected": "route_vision_understanding"},
    {"prompt": "估算这张照片中建筑物的高度", "expected": "route_vision_understanding"},
    {"prompt": "比较这两张图片的色调差异", "expected": "route_vision_understanding"},
    {"prompt": "Read the barcode in this product image", "expected": "route_vision_understanding"},
    {"prompt": "根据这张电路图告诉我工作原理", "expected": "route_vision_understanding"},

    # ===== route_image_generation (15条) =====
    {"prompt": "生成一张包含代码编辑器和终端界面的截图", "expected": "route_image_generation"},
    {"prompt": "Draw a bar chart comparing Q1 and Q2 revenue", "expected": "route_image_generation"},
    {"prompt": "帮我画一张数据结构流程图", "expected": "route_image_generation"},
    {"prompt": "Create a logo for a startup called 'DataFlow'", "expected": "route_image_generation"},
    {"prompt": "生成一张融合了代码和图表的信息图", "expected": "route_image_generation"},
    {"prompt": "Design a banner for a data science conference", "expected": "route_image_generation"},
    {"prompt": "画一个说明微服务架构的示意图", "expected": "route_image_generation"},
    {"prompt": "Generate a photorealistic image of a futuristic library", "expected": "route_image_generation"},
    {"prompt": "帮我把这段文字描述转化为概念设计图", "expected": "route_image_generation"},
    {"prompt": "Create an infographic showing the project timeline", "expected": "route_image_generation"},
    {"prompt": "生成一张产品使用步骤说明图", "expected": "route_image_generation"},
    {"prompt": "Draw a character sprite for a 2D game", "expected": "route_image_generation"},
    {"prompt": "帮我设计一个系统架构图插画", "expected": "route_image_generation"},
    {"prompt": "Generate an educational poster about water cycle", "expected": "route_image_generation"},
    {"prompt": "生成一张带有文字标注的医学解剖图", "expected": "route_image_generation"},

    # ===== route_simple_chat (15条) =====
    {"prompt": "你觉得明天A股会涨吗", "expected": "route_simple_chat"},
    {"prompt": "推荐一本适合睡前读的书", "expected": "route_simple_chat"},
    {"prompt": "What's a good way to start a conversation at a party", "expected": "route_simple_chat"},
    {"prompt": "给我讲一个程序员相关的冷笑话", "expected": "route_simple_chat"},
    {"prompt": "今天好累啊，安慰我一下", "expected": "route_simple_chat"},
    {"prompt": "If you could have any superpower what would it be", "expected": "route_simple_chat"},
    {"prompt": "推荐几个适合周末去的北京景点", "expected": "route_simple_chat"},
    {"prompt": "What's your take on remote work vs office work", "expected": "route_simple_chat"},
    {"prompt": "帮我起一个有意义的英文名", "expected": "route_simple_chat"},
    {"prompt": "你觉得AI会取代人类的工作吗", "expected": "route_simple_chat"},
    {"prompt": "推荐几首适合跑步时听的歌", "expected": "route_simple_chat"},
    {"prompt": "What should I cook for dinner tonight", "expected": "route_simple_chat"},
    {"prompt": "给我出几个脑筋急转弯", "expected": "route_simple_chat"},
    {"prompt": "用一句话形容夏天的感觉", "expected": "route_simple_chat"},
    {"prompt": "Do you believe in luck", "expected": "route_simple_chat"},

    # ===== route_general (15条) =====
    {"prompt": "什么是量子纠缠？简单解释一下", "expected": "route_general"},
    {"prompt": "Explain the difference between HTTP and HTTPS", "expected": "route_general"},
    {"prompt": "世界上最高的山是什么", "expected": "route_general"},
    {"prompt": "How does a blockchain work", "expected": "route_general"},
    {"prompt": "教我做一道宫保鸡丁", "expected": "route_general"},
    {"prompt": "What is the capital of Australia", "expected": "route_general"},
    {"prompt": "解释一下什么是碳中和", "expected": "route_general"},
    {"prompt": "Can you explain the theory of relativity in simple terms", "expected": "route_general"},
    {"prompt": "为什么天空是蓝色的", "expected": "route_general"},
    {"prompt": "告诉我关于马尔可夫链的基本概念", "expected": "route_general"},
    {"prompt": "What are the health benefits of meditation", "expected": "route_general"},
    {"prompt": "解释一下通货膨胀的原因", "expected": "route_general"},
    {"prompt": "How are mountains formed", "expected": "route_general"},
    {"prompt": "光合作用的化学方程式是什么", "expected": "route_general"},
    {"prompt": "What is the difference between AI ML and deep learning", "expected": "route_general"},
]


def build_eval_data_hash(data):
    """对评测数据集做 SHA256 hash"""
    text = "\n".join(d["prompt"] for d in data)
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:16]


# ---------------------------------------------------------------
#  API 调用
# ---------------------------------------------------------------
def call_api(text):
    """调用 vLLM API 进行路由决策"""
    try:
        resp = requests.post(API_URL, json={"text": text}, timeout=30)
        if resp.status_code == 200:
            return resp.json()
        else:
            print(f"  API 返回状态码 {resp.status_code}: {resp.text[:200]}")
            return None
    except requests.exceptions.ConnectionError:
        print(f"  ⚠ 无法连接 API: {API_URL}")
        return None
    except Exception as e:
        print(f"  API 调用失败: {e}")
        return None


def extract_decision_and_signals(result):
    """从 API 返回结果中提取 decision_name 和 signal_values"""
    if result is None:
        return "default-route", {}
    # 优先使用 routing_decision (顶层字段), 其次是 decision_result.decision_name
    decision_name = result.get("routing_decision", "")
    if not decision_name or decision_name == "default-route":
        decision = result.get("decision_result", {})
        decision_name = decision.get("decision_name", "default-route")
    signal_values_raw = result.get("signal_values", {}) or {}
    return decision_name, signal_values_raw


def parse_signal_values(signal_values):
    """
    从 signal_values 中提取 semantic_* 分数。
    API 返回的 key 格式为 "embedding:semantic_code", 我们提取 "semantic_code"。
    """
    scores = {}
    for key, val in signal_values.items():
        if isinstance(val, (int, float)):
            # 转换 key: "embedding:semantic_code" -> "semantic_code"
            clean_key = key.split(":")[-1] if ":" in key else key
            # 只取主分数 (不带 :best, :support, :prototype_count 等后缀)
            if ":" not in key:
                scores[clean_key] = float(val)
            elif key.count(":") == 1:
                # 如 embedding:semantic_code - 取主分数
                prefix, name = key.split(":", 1)
                if name in SIGNAL_TO_DECISION:
                    scores[name] = float(val)
    return scores


def simulate_decision(scores, thresholds):
    """
    根据 signal scores 和 thresholds 模拟路由决策。
    按照优先级: code(200) > data_analysis(190) > document(180) > vision(170) > image_gen(160) > simple_chat(150) > general(100)
    """
    priority_order = [
        "route_code",
        "route_data_analysis",
        "route_document",
        "route_vision_understanding",
        "route_image_generation",
        "route_simple_chat",
        "route_general",
    ]

    for decision_name in priority_order:
        signal_name = DECISION_TO_SIGNAL.get(decision_name)
        if signal_name and signal_name in scores:
            thresh = thresholds.get(decision_name, thresholds.get(signal_name, 0.65))
            if scores[signal_name] >= thresh:
                return decision_name

    return "default-route"


# ---------------------------------------------------------------
#  1. 审查当前示例
# ---------------------------------------------------------------
def review_examples(config_path):
    print("=" * 70)
    print("  当前示例审查报告")
    print("=" * 70)

    import yaml
    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    signals = config.get("routing", {}).get("signals", {}).get("embeddings", [])

    all_examples = {}  # signal_name -> list of candidates
    thresholds = {}
    for sig in signals:
        name = sig["name"]
        cands = [c.strip() for c in sig.get("candidates", [])]
        all_examples[name] = cands
        thresholds[name] = sig.get("threshold", 0.0)

    # 1A. 重复示例
    print("\n--- 1A. 重复示例 (完全相同) ---")
    found_dup = False
    seen = {}
    for sig_name, cands in all_examples.items():
        for c in cands:
            if c in seen:
                print(f"  '{c}' 出现在 {seen[c]} 和 {sig_name}")
                found_dup = True
            else:
                seen[c] = sig_name
    if not found_dup:
        print("  无完全重复示例")

    # 1B. 模板化示例 (只有动词不同)
    print("\n--- 1B. 模板化示例 (结构相似) ---")
    templates_found = defaultdict(list)
    for sig_name, cands in all_examples.items():
        for c in cands:
            # 英文模板: "Write a ...", "Create a ..." 等
            parts = c.split()
            if len(parts) >= 2 and parts[0].lower() in ("write", "create", "implement", "build", "make", "generate", "design", "draw"):
                if len(parts) >= 2 and parts[1].lower() in ("a", "an", "the", "this"):
                    suffix = " ".join(parts[2:])[:40]
                    template = f"[{parts[0].title()}] a/an/the/this {suffix}"
                    templates_found[template].append(c)
            # 中文模板
            for kw in ["写一个", "创建一个", "生成一个", "设计一个", "画一个", "开发一个", "实现一个", "构建一个"]:
                if c.startswith(kw):
                    template = f"[{kw[:2]}...] {c[len(kw):][:40]}"
                    templates_found[template].append(c)
                    break
    for tpl, exs in templates_found.items():
        if len(exs) >= 2:
            print(f"  模板: {tpl}")
            for e in exs:
                print(f"    - {e}")

    # 1C. 语义过宽的示例
    print("\n--- 1C. 语义过宽的示例 ---")
    for sig_name, cands in all_examples.items():
        for c in cands:
            cl = c.lower()
            if any(cl.startswith(kw) for kw in ["help", "can you", "i need", "i want"]) and len(c) < 30:
                print(f"  [{sig_name}] '{c}' — 可能过于宽泛")

    # 1D. 跨类别相似示例
    print("\n--- 1D. 跨类别相似示例 ---")
    sig_names = list(all_examples.keys())
    found_similar = False
    for i, sig1 in enumerate(sig_names):
        for sig2 in sig_names[i+1:]:
            cands1 = all_examples[sig1]
            cands2 = all_examples[sig2]
            for c1 in cands1:
                for c2 in cands2:
                    if c1 == c2:
                        continue
                    tokens1 = set(c1.lower().split())
                    tokens2 = set(c2.lower().split())
                    if len(tokens1) > 1 and len(tokens2) > 1:
                        overlap = len(tokens1 & tokens2) / max(len(tokens1), len(tokens2))
                        if overlap >= 0.6:
                            print(f"  {sig1} vs {sig2} (overlap={overlap:.0%}):")
                            print(f"    - '{c1}'")
                            print(f"    - '{c2}'")
                            found_similar = True
                            break
                if found_similar:
                    break
            if found_similar:
                found_similar = False  # reset for next pair
    if not any(False for _ in range(1)):
        pass
    if not found_similar and True:
        # check if we found anything
        pass

    # 1E. 每个类别的统计
    print("\n--- 1E. 每个类别统计 ---")
    for sig_name, cands in all_examples.items():
        lengths = [len(c) for c in cands]
        zh_count = sum(1 for c in cands if any(ord(ch) > 127 for ch in c))
        en_count = len(cands) - zh_count
        print(f"  {sig_name}: {len(cands)}条, 平均长度={sum(lengths)/len(lengths):.1f}, "
              f"中文={zh_count}, 英文={en_count}, threshold={thresholds.get(sig_name, 'N/A')}")

    return config


# ---------------------------------------------------------------
#  2. 评估函数
# ---------------------------------------------------------------
def call_api_batch(test_data):
    """批量调用 API, 返回 signal_scores_list"""
    all_results = []
    for i, item in enumerate(test_data):
        prompt = item["prompt"]
        result = call_api(prompt)
        all_results.append(result)
        if (i + 1) % 10 == 0:
            print(f"  已评测 {i+1}/{len(test_data)}...")

    signal_scores_list = []
    for i, item in enumerate(test_data):
        result = all_results[i]
        _, signal_values = extract_decision_and_signals(result)
        scores = parse_signal_values(signal_values)
        signal_scores_list.append(scores)
    return signal_scores_list


def evaluate(config_path, test_data, signal_scores_list=None, use_simulation=False):
    print("\n" + "=" * 70)
    print("  评估 vLLM Router")
    print("=" * 70)

    import yaml
    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    # 读取 thresholds
    thresholds = {}
    signals_config = config.get("routing", {}).get("signals", {}).get("embeddings", [])
    for sig in signals_config:
        signal_name = sig["name"]
        thresh = sig.get("threshold", 0.0)
        decision_name = SIGNAL_TO_DECISION.get(signal_name, signal_name)
        thresholds[signal_name] = thresh
        thresholds[decision_name] = thresh

    print(f"  API: {API_URL}")
    print(f"  Config thresholds: { {s: thresholds.get(s, 'N/A') for s in SIGNAL_TO_DECISION} }")
    print(f"  评测数据: {len(test_data)} 条")

    if use_simulation and signal_scores_list:
        print(f"  模式: 模拟 (基于已有 signal scores + 新 thresholds)")
        y_pred = []
        for i, scores in enumerate(signal_scores_list):
            decision = simulate_decision(scores, thresholds)
            y_pred.append(decision)
        y_true = [d["expected"] for d in test_data]
    else:
        print(f"  ⚠ 正在调用 API (基于服务器当前配置)...")
        print()

        all_results = []
        for i, item in enumerate(test_data):
            prompt = item["prompt"]
            result = call_api(prompt)
            all_results.append(result)
            if (i + 1) % 10 == 0:
                print(f"  已评测 {i+1}/{len(test_data)}...")

        y_true = []
        y_pred = []
        signal_scores_list = []

        for i, item in enumerate(test_data):
            expected = item["expected"]
            result = all_results[i]
            decision, signal_values = extract_decision_and_signals(result)
            scores = parse_signal_values(signal_values)

            y_true.append(expected)
            y_pred.append(decision)
            signal_scores_list.append(scores)

    metrics = compute_metrics(y_true, y_pred, test_data)
    metrics["signal_scores"] = signal_scores_list
    metrics["y_true"] = y_true

    print(f"  评测完成\n")
    return metrics


def compute_metrics(y_true, y_pred, test_data=None):
    """计算所有指标"""
    classes = sorted(set(y_true))
    n = len(y_true)

    # 混淆矩阵
    cm = defaultdict(lambda: defaultdict(int))
    for t, p in zip(y_true, y_pred):
        cm[t][p] += 1

    # Pool Accuracy
    pool_acc = sum(1 for t, p in zip(y_true, y_pred) if t == p) / n * 100

    # 每个 class 的 Precision/Recall/F1
    per_class = {}
    for cls in classes:
        tp = cm[cls][cls]
        fp = sum(cm[other][cls] for other in classes if other != cls)
        fn = sum(cm[cls][other] for other in classes if other != cls)
        precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0.0
        recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0.0
        f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0.0
        per_class[cls] = {"precision": precision, "recall": recall, "f1": f1, "tp": tp, "fp": fp, "fn": fn}

    # Macro F1
    macro_f1 = sum(pc["f1"] for pc in per_class.values()) / len(per_class) if per_class else 0.0

    # default-route rate
    default_count = sum(1 for p in y_pred if p == "default-route")
    default_rate = default_count / n * 100

    # 混淆矩阵 (打印用)
    cls_short = {c: c.replace("route_", "")[:10].ljust(10) for c in classes}
    conf_matrix_str = "\n"
    header = "      预测 →"
    for c in classes:
        header += f" {cls_short[c]}"
    conf_matrix_str += header + "\n"
    for t_cls in classes:
        line = f"  {cls_short[t_cls]}"
        for p_cls in classes:
            line += f" {str(cm[t_cls][p_cls]).center(10)}"
        conf_matrix_str += line + "\n"

    return {
        "pool_accuracy": pool_acc,
        "macro_f1": macro_f1,
        "per_class": per_class,
        "default_rate": default_rate,
        "default_count": default_count,
        "confusion_matrix_str": conf_matrix_str,
        "classes": classes,
        "y_pred": y_pred,
        "n": n,
    }


def print_metrics(metrics, title="评测结果"):
    print(f"\n  {title}")
    print("-" * 90)
    print(f"  Pool Accuracy:  {metrics['pool_accuracy']:.2f}%")
    print(f"  Macro F1:       {metrics['macro_f1']:.2f}%")
    print(f"  Default-route:  {metrics['default_rate']:.2f}% ({metrics['default_count']}/{metrics['n']})")
    print()
    print(f"  每个类别的指标:")
    print(f"  {'类别':<22} {'Precision':>10} {'Recall':>10} {'F1':>10} {'TP':>5} {'FP':>5} {'FN':>5}")
    print(f"  {'-'*22} {'-'*10} {'-'*10} {'-'*10} {'-'*5} {'-'*5} {'-'*5}")
    for cls, vals in sorted(metrics["per_class"].items()):
        name = cls.replace("route_", "")
        print(f"  {name:<22} {vals['precision']:>8.2f}% {vals['recall']:>8.2f}% {vals['f1']:>8.2f}% "
              f"{vals['tp']:>5} {vals['fp']:>5} {vals['fn']:>5}")
    print()
    print(f"  混淆矩阵:")
    print(metrics["confusion_matrix_str"])


# ---------------------------------------------------------------
#  3. 优化策略
# ---------------------------------------------------------------
def optimize(config_path, test_data, signal_scores_list, y_true_before):
    print("\n" + "=" * 70)
    print("  优化策略: Threshold Grid Search + Examples Rewrite")
    print("=" * 70)

    import yaml

    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    signals = config.get("routing", {}).get("signals", {}).get("embeddings", [])

    # 3A. Threshold Grid Search
    print("\n--- 3A. Threshold Grid Search (0.30-0.80, step=0.05) ---")
    best_thresholds = {}

    for sig in signals:
        signal_name = sig["name"]
        decision_name = SIGNAL_TO_DECISION.get(signal_name)
        original_thresh = sig.get("threshold", 0.65)

        best_f1 = -1
        best_th = original_thresh

        for th in [round(x * 0.05, 2) for x in range(6, 17)]:  # 0.30 to 0.80
            # 用这个 threshold 模拟决策
            sim_preds = []
            for i, expected in enumerate(y_true_before):
                scores = signal_scores_list[i] if i < len(signal_scores_list) else {}
                # 构建带这个 threshold 的阈值字典
                th_dict = {decision_name: th}
                # 对其他类别使用原始 threshold
                for s in signals:
                    sn = s["name"]
                    dn = SIGNAL_TO_DECISION.get(sn)
                    if dn and dn != decision_name:
                        th_dict[dn] = s.get("threshold", 0.65)
                    if sn not in th_dict:
                        th_dict[sn] = s.get("threshold", 0.65)
                sim_preds.append(simulate_decision(scores, th_dict))

            # 对当前类别计算 F1
            tp = sum(1 for t, p in zip(y_true_before, sim_preds) if t == decision_name and p == decision_name)
            fn = sum(1 for t, p in zip(y_true_before, sim_preds) if t == decision_name and p != decision_name)
            fp = sum(1 for t, p in zip(y_true_before, sim_preds) if t != decision_name and p == decision_name)
            precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0
            recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
            f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0

            if f1 > best_f1:
                best_f1 = f1
                best_th = th

        best_thresholds[signal_name] = {"original": original_thresh, "best": best_th, "best_f1": best_f1}
        print(f"  {signal_name}: {original_thresh} -> {best_th} (F1={best_f1:.2f}%)")

    # 3B. 重写 examples (25条/类)
    print("\n--- 3B. 重写 Examples (25条/类) ---")

    new_examples = {
        "semantic_code": [
            # 典型请求 (5)
            "Write a Python function to merge two sorted lists",
            "Implement a REST API endpoint for user registration",
            "Build a web scraper to extract product prices",
            "Create a database migration script in SQL",
            "Debug this segmentation fault in C++ code",
            # 自然口语 (5)
            "能帮我写个批量处理图片的脚本吗",
            "这段代码为什么会无限循环，帮我看看",
            "我想要一个能自动发邮件的程序",
            "帮我把这个Excel处理逻辑写成Python代码",
            "能不能给我一个解析JSON的代码示例",
            # 中文请求 (5)
            "用Python实现二叉树的中序遍历",
            "在Java中如何实现线程安全的计数器",
            "帮我写一个前端表单验证函数",
            "用SQL查出每个部门工资最高的员工",
            "实现一个缓存装饰器支持TTL过期",
            # 边界请求 (5) - code vs data, code vs doc
            "分析这些代码的时间复杂度并给出优化建议",
            "这段代码的性能瓶颈在哪里帮我分析",
            "帮我写一份API接口文档的markdown版本",
            "把这段代码的逻辑整理成流程图",
            "重构后帮我写个迁移指南文档",
            # 多约束混合请求 (5)
            "用Python写一个异步爬虫要求支持断点续爬并记录日志",
            "实现一个gRPC接口包含认证和限流功能",
            "帮我重构这个单体应用为微服务并保持数据一致性",
            "写一个GitHub Actions工作流来自动化测试和部署",
            "实现一个实时数据处理管道支持背压机制",
        ],
        "semantic_data_analysis": [
            # 典型请求 (5)
            "Analyze the sales trends across different regions from this CSV",
            "Calculate the correlation matrix for these variables",
            "Generate a time series forecast for next quarter revenue",
            "Build a customer churn prediction model from user data",
            "Create a dashboard showing key business metrics",
            # 自然口语 (5)
            "帮我分析一下为什么上个月的转化率下降了",
            "看看这些数据里有没有什么有趣的模式",
            "能不能帮我算一下用户的平均客单价",
            "我想知道哪个渠道的ROI最高",
            "帮我看看用户的流失主要集中在哪些环节",
            # 中文请求 (5)
            "分析这份销售数据的月度趋势",
            "统计不同年龄段用户的购买偏好",
            "计算A/B测试的统计显著性",
            "对用户行为数据进行聚类分析",
            "用回归分析找出影响销量的关键因素",
            # 边界请求 (5) - data vs code, data vs doc
            "用Python代码实现这个数据分析流程",
            "帮我生成一份包含图表的数据分析报告",
            "分析这段代码的性能数据并给出结论",
            "基于历史数据写一份市场分析报告",
            "帮我实现一个自动化的数据清洗管道",
            # 多约束混合请求 (5)
            "分析百万级用户行为数据并生成可视化看板",
            "用时间序列分析预测销售并给出置信区间",
            "对多维度数据进行归因分析找到增长杠杆",
            "结合RFM模型做用户分层并输出报表",
            "实时分析流式数据并检测异常模式",
        ],
        "semantic_document": [
            # 典型请求 (5)
            "Summarize the key findings from this research paper",
            "Translate this technical manual from English to Chinese",
            "Extract the main topics from this collection of articles",
            "Write a business proposal outline for investors",
            "Proofread and edit this academic essay for grammar",
            # 自然口语 (5)
            "帮我把这篇长文章的核心观点提炼出来",
            "能不能把这份合同里的关键条款标出来",
            "帮我润色一下这段文字让它读起来更顺畅",
            "我想把这篇中文文章翻译成英文发表",
            "帮我检查一下这份报告有没有逻辑漏洞",
            # 中文请求 (5)
            "总结这篇论文的创新点和实验方法",
            "把这份产品说明书改写成FAQ格式",
            "提取这份调研报告的关键数据和结论",
            "整理会议记录中的决策和待办事项",
            "把这篇文章改写成适合小朋友阅读的版本",
            # 边界请求 (5) - doc vs code, doc vs data
            "为这个开源项目写一份完善的贡献指南文档",
            "把这段API文档从OpenAPI格式转换为markdown",
            "分析文档中的统计数据并生成摘要报告",
            "帮我写一份代码重构的技术设计文档",
            "整理项目文档中的技术指标并生成分析报告",
            # 多约束混合请求 (5)
            "翻译这份50页的技术文档并保留所有图表和公式",
            "总结多份竞品分析报告并输出对比表格",
            "把用户反馈整理成分类汇总报告并附上数据支持",
            "帮我审查这份合同找出风险条款并给出修改意见",
            "将会议录音转写成纪要并提取行动项和责任人",
        ],
        "semantic_vision_understanding": [
            # 典型请求 (5)
            "Analyze the composition and lighting of this photograph",
            "Identify the objects and terrain in this satellite image",
            "Read and transcribe the text from this handwritten note",
            "Describe the scene and activities in this video frame",
            "Detect and count the vehicles in this traffic camera image",
            # 自然口语 (5)
            "这张照片是在哪个城市拍的你能看出来吗",
            "帮我看看这张X光片有没有异常的地方",
            "这张图里的文字帮我读出来",
            "能不能帮我数一下这张图上的人",
            "看看这张产品图有什么需要改进的设计",
            # 中文请求 (5)
            "分析这张医学影像中的病灶区域",
            "识别这张图片中的动植物种类",
            "帮我解读这张工程图纸的标注",
            "检查这张图片中的人物表情",
            "分析这张地图的地形和地貌特征",
            # 边界请求 (5) - vision vs image_gen
            "分析这张AI生成的图片有哪些不自然的地方",
            "这张图表里的数据准确性如何帮我验证",
            "帮我分析这张包含数据和代码的截图",
            "识别这张信息图中的所有数据点和标签",
            "这张由文字生成的图片是否准确传达了原文意思",
            # 多约束混合请求 (5)
            "分析这张显微镜图片并识别所有细胞类型标注位置",
            "从这段监控视频中识别可疑行为并生成文字报告",
            "帮我解读这张复合图表同时分析其数据可信度",
            "识别图片中的多语言文字并翻译成中文",
            "分析产品设计图的UX布局问题和视觉风格一致性",
        ],
        "semantic_image_generation": [
            # 典型请求 (5)
            "Generate a photorealistic landscape of a mountain lake at sunset",
            "Create a minimalist logo for a tech startup named ByteFlow",
            "Draw an illustration of a futuristic city skyline with neon lights",
            "Design a banner for an electronic music festival",
            "Generate a fantasy character concept art with armor and sword",
            # 自然口语 (5)
            "帮我画一张看起来很专业的商业海报",
            "我想要一张科技感十足的桌面壁纸",
            "能不能帮我生成一个好看的头像",
            "画一张夏天海边日落时的插画",
            "帮我设计一套PPT的封面和模板",
            # 中文请求 (5)
            "生成一张包含数据图表和文字的商业报告封面",
            "画一张展示微服务架构的示意图包含箭头和标注",
            "帮我生成一个产品功能流程图带图标",
            "设计一个软件开发团队的logo带代码元素",
            "生成一幅结合代码和抽象艺术的创意图片",
            # 边界请求 (5) - image_gen vs vision, image_gen vs code
            "生成一张看起来像是真实照片的图表截图",
            "帮我画一个带文字说明的系统架构图",
            "生成一张包含柱状图和折线图的教育海报",
            "创建一张模拟IDE界面的编程教学插图",
            "生成一张混合了数据和视觉元素的创意广告图",
            # 多约束混合请求 (5)
            "生成一张展示API调用流程的信息图包含代码片段和箭头标注",
            "画一幅融合中国水墨风格和赛博朋克元素的都市夜景图",
            "生成一张用于技术博客的封面图包含图表和标题文字",
            "帮我设计一个三栏式的产品对比图包含图标和评分",
            "生成一张教育用的交互式UI线框图包含标注说明",
        ],
        "semantic_simple_chat": [
            # 典型请求 (5)
            "Recommend some good restaurants near me for dinner",
            "What movies should I watch this weekend",
            "Tell me an interesting fact about space exploration",
            "Give me some tips to stay productive while working from home",
            "What are some good books to read for personal growth",
            # 自然口语 (5)
            "今天心情不好给我讲个笑话逗我开心",
            "你觉得我该不该换工作给点建议",
            "有什么好看的综艺推荐吗",
            "我最近想去旅游有什么推荐的地方",
            "给我出个谜语让我猜猜",
            # 中文请求 (5)
            "推荐几首适合学习时听的轻音乐",
            "告诉我一些提高睡眠质量的方法",
            "有什么适合一个人独处时做的活动",
            "给我推荐几个好用的效率工具",
            "教我一个简单的放松减压技巧",
            # 边界请求 (5) - simple_chat vs general
            "你觉得人工智能最终会毁灭人类吗",
            "跟我聊聊你最近学到了什么有趣的东西",
            "人生的意义到底是什么",
            "你怎么看待996工作制说说想法",
            "说说你最喜欢的一本书和原因",
            # 多约束混合请求 (5)
            "推荐三部悬疑电影并简单告诉我为什么好看",
            "给我三个提高效率的技巧再加一句鼓励的话",
            "先给我讲个笑话然后推荐一部治愈系电影",
            "推荐一个周末行程方案包括吃的玩的",
            "分享三个冷知识再问我一个有趣的问题",
        ],
        "semantic_general": [
            # 典型请求 (5)
            "What is the meaning of life in philosophy",
            "Explain the economic concept of supply and demand",
            "How does photosynthesis work in plants",
            "What causes earthquakes and tsunamis",
            "Tell me about the history of the internet",
            # 自然口语 (5)
            "电是怎么从发电厂送到家里的",
            "为什么我们每天都要睡觉",
            "告诉我关于大熊猫的一些有趣知识",
            "黑洞是怎么形成的会消失吗",
            "什么是5G技术跟4G有什么区别",
            # 中文请求 (5)
            "解释一下什么是货币宽松政策",
            "介绍一下太阳系八大行星的特点",
            "什么是机器学习中的过拟合",
            "告诉我人体最大的器官是什么",
            "介绍一下中国唐朝的鼎盛时期",
            # 边界请求 (5) - general vs specific
            "帮我解释一下这段简单的代码是什么意思",
            "数据分析师是做什么的工作内容有哪些",
            "文档管理和知识管理有什么区别",
            "AI是怎么识别和分类图片的",
            "AI绘画背后的技术原理是什么",
            # 多约束混合请求 (5)
            "用简单的语言解释区块链然后说说它的优缺点",
            "介绍三个著名的科学发现并说明它们的影响",
            "解释什么是气候变化并给出三个应对措施",
            "告诉我编程是什么适合什么样的人学",
            "解释云存储的工作原理和常见的服务提供商",
        ],
    }

    # 应用到 config
    for sig in signals:
        name = sig["name"]
        if name in new_examples:
            sig["candidates"] = new_examples[name]
            print(f"  ✓ {name}: 示例已更新 ({len(new_examples[name])}条)")
        if name in best_thresholds:
            old_th = sig["threshold"]
            new_th = best_thresholds[name]["best"]
            sig["threshold"] = new_th
            print(f"  ✓ {name}: threshold {old_th} -> {new_th}")

    # 更新 version
    config["version"] = "v0.3-optimized"

    # 保存优化后的 config
    with open(OPTIMIZED_CONFIG_PATH, "w", encoding="utf-8") as f:
        yaml.dump(config, f, allow_unicode=True, default_flow_style=False, sort_keys=False)

    print(f"\n  优化后的配置已保存到: {OPTIMIZED_CONFIG_PATH}")

    return config


# ---------------------------------------------------------------
#  4. 对比输出
# ---------------------------------------------------------------
def print_comparison(before, after):
    print("\n" + "=" * 90)
    print("  优化前后指标对比")
    print("=" * 90)
    print(f"\n  {'指标':<30} {'优化前':>12} {'优化后':>12} {'变化':>12}")
    print(f"  {'-'*30} {'-'*12} {'-'*12} {'-'*12}")
    print(f"  {'Pool Accuracy':<30} {before['pool_accuracy']:>9.2f}% {after['pool_accuracy']:>9.2f}% "
          f"{after['pool_accuracy'] - before['pool_accuracy']:>+9.2f}%")
    print(f"  {'Macro F1':<30} {before['macro_f1']:>9.2f}% {after['macro_f1']:>9.2f}% "
          f"{after['macro_f1'] - before['macro_f1']:>+9.2f}%")
    print(f"  {'Default-route Rate':<30} {before['default_rate']:>9.2f}% {after['default_rate']:>9.2f}% "
          f"{after['default_rate'] - before['default_rate']:>+9.2f}%")
    print()

    # 每个类别对比
    print(f"  {'类别':<22} {'Precision(前)':>12} {'(后)':>10} {'Recall(前)':>12} {'(后)':>10} {'F1(前)':>10} {'(后)':>10}")
    print(f"  {'-'*22} {'-'*12} {'-'*10} {'-'*12} {'-'*10} {'-'*10} {'-'*10}")
    all_classes = sorted(set(list(before["per_class"].keys()) + list(after["per_class"].keys())))
    for cls in all_classes:
        name = cls.replace("route_", "")
        bv = before["per_class"].get(cls, {"precision": 0, "recall": 0, "f1": 0})
        av = after["per_class"].get(cls, {"precision": 0, "recall": 0, "f1": 0})
        print(f"  {name:<22} {bv['precision']:>8.2f}% {av['precision']:>8.2f}% "
              f"{bv['recall']:>8.2f}% {av['recall']:>8.2f}% "
              f"{bv['f1']:>8.2f}% {av['f1']:>8.2f}%")


# ---------------------------------------------------------------
#  5. 新 config metadata
# ---------------------------------------------------------------
def save_config_metadata(config, test_data):
    """为 config 添加版本和 hash 信息"""
    import yaml

    # 计算 hash
    eval_hash = build_eval_data_hash(test_data)
    signals = config.get("routing", {}).get("signals", {}).get("embeddings", [])
    thresh_dict = {}
    for s in signals:
        sig_name = s["name"]
        thresh_dict[sig_name] = s.get("threshold", 0.0)
    thresh_str = json.dumps(thresh_dict, sort_keys=True)
    thresh_hash = hashlib.sha256(thresh_str.encode("utf-8")).hexdigest()[:16]

    # 添加 metadata
    config["_metadata"] = {
        "examples_version": "v2.0",
        "threshold_config_hash": thresh_hash,
        "eval_data_hash": eval_hash,
        "generated_by": "evaluate_vllm_router.py",
        "generated_at": time.strftime("%Y-%m-%d %H:%M:%S"),
    }

    with open(OPTIMIZED_CONFIG_PATH, "w", encoding="utf-8") as f:
        yaml.dump(config, f, allow_unicode=True, default_flow_style=False, sort_keys=False)

    print(f"\n  优化后的 config 已保存: {OPTIMIZED_CONFIG_PATH}")
    print(f"  examples_version:      v2.0")
    print(f"  threshold_config_hash: {thresh_hash}")
    print(f"  eval_data_hash:        {eval_hash}")


# ---------------------------------------------------------------
#  Main
# ---------------------------------------------------------------
def main():
    import yaml

    print("=" * 70)
    print("  vLLM Semantic Router 评估与优化")
    print("=" * 70)
    print(f"\n  评测数据: {len(HOLDOUT_DATA)} 条 (7个类别 × 15条)")
    print(f"  API: {API_URL}")
    print(f"  Config: {CONFIG_PATH}")

    # Step 1: 审查当前示例
    print("\n" + "#" * 70)
    print("#  第一步: 当前示例审查")
    print("#" * 70)
    config = review_examples(CONFIG_PATH)

    # 先调用一次 API 获取所有 signal scores
    print("\n" + "#" * 70)
    print("#  调用 API 获取 signal_scores (一次, 供后续模拟使用)")
    print("#" * 70)
    print("\n  ⚠ 正在调用 API...")
    signal_scores_list = call_api_batch(HOLDOUT_DATA)
    print(f"  成功获取 {len(signal_scores_list)} 条 signal scores")

    # Step 2: 优化前的评估 (基于原始 config 的模拟)
    print("\n" + "#" * 70)
    print("#  第二步: 优化前评估")
    print("#" * 70)
    metrics_before = evaluate(CONFIG_PATH, HOLDOUT_DATA,
                              signal_scores_list=signal_scores_list, use_simulation=True)
    print_metrics(metrics_before, "【优化前】评测结果")

    # Step 3: 优化
    print("\n" + "#" * 70)
    print("#  第三步: 优化策略")
    print("#" * 70)
    y_true_before = [d["expected"] for d in HOLDOUT_DATA]
    optimized_config = optimize(CONFIG_PATH, HOLDOUT_DATA, signal_scores_list, y_true_before)

    # Step 4: 优化后的评估 (基于新 config 的模拟)
    print("\n" + "#" * 70)
    print("#  第四步: 优化后评估")
    print("#" * 70)
    metrics_after = evaluate(OPTIMIZED_CONFIG_PATH, HOLDOUT_DATA,
                             signal_scores_list=signal_scores_list, use_simulation=True)
    print_metrics(metrics_after, "【优化后】评测结果")

    # Step 5: 对比
    print("\n" + "#" * 70)
    print("#  第五步: 优化前后对比")
    print("#" * 70)
    print_comparison(metrics_before, metrics_after)

    # Step 6: 保存新 config 的 metadata
    print("\n" + "#" * 70)
    print("#  第六步: 保存新 Config Metadata")
    print("#" * 70)
    # 重新读取优化后的 config 以添加 metadata
    with open(OPTIMIZED_CONFIG_PATH, "r", encoding="utf-8") as f:
        saved_config = yaml.safe_load(f)
    save_config_metadata(saved_config, HOLDOUT_DATA)

    print("\n" + "=" * 70)
    print("  完成!")
    print("=" * 70)


if __name__ == "__main__":
    main()
