#!/usr/bin/env python3
"""手动微调 vLLM Router threshold 和优先级"""
import json
import yaml
import hashlib

with open('vllm_optimized_config.yaml', encoding='utf-8') as f:
    config = yaml.safe_load(f)

# =============================================
# 手动微调 Threshold
# =============================================
# 基于 signal scores 分布分析：
# - code 平均分 ~0.48，general 平均分 ~0.43
# - general 阈值太低(0.3)导致太多误匹配
# - document 和 image_generation 阈值可以略降

new_thresholds = {
    'semantic_code': 0.55,                # 提高一点，减少误配
    'semantic_data_analysis': 0.55,       # 保持
    'semantic_document': 0.60,            # 保持，document 分数较高
    'semantic_vision_understanding': 0.55, # 保持
    'semantic_image_generation': 0.55,    # 保持
    'semantic_simple_chat': 0.55,         # 提高，减少误配
    'semantic_general': 0.45,             # 降低区分度，但不至于太低
}

signals = config['routing']['signals']['embeddings']
for s in signals:
    name = s['name']
    old_th = s['threshold']
    new_th = new_thresholds.get(name, old_th)
    s['threshold'] = new_th
    print(f"  {name}: {old_th} -> {new_th}")

# =============================================
# 重新排序 decisions 优先级
# =============================================
# 原则：不要因为优先级高就覆盖明确匹配
# 1. default-route (兜底, 优先级最低)
# 2. general/simple_chat (通用类, 低优先级)
# 3. image_generation/vision (明确意图, 高优先级)
# 4. code/data/document (专业类, 最高优先级)
#
# 但注意：decision 是用 embedding name match 的
# 所以优先级只影响当多个 embedding 都过 threshold 时
# 应该让明确意图的类别优先

new_priorities = {
    'default-route': 1,
    'route_smoke': 999,
    'route_general': 100,
    'route_simple_chat': 110,
    'route_image_generation': 120,
    'route_vision_understanding': 130,
    'route_document': 140,
    'route_data_analysis': 150,
    'route_code': 160,
}

for d in config['routing']['decisions']:
    name = d['name']
    if name in new_priorities:
        old_pri = d['priority']
        new_pri = new_priorities[name]
        d['priority'] = new_pri
        print(f"  {name}: priority {old_pri} -> {new_pri}")
    else:
        pass

# =============================================
# 更新 metadata
# =============================================
th_str = json.dumps(new_thresholds, sort_keys=True)
th_hash = hashlib.md5(th_str.encode()).hexdigest()[:16]

# 保存最终版本
output_path = 'vllm_final_config.yaml'
with open(output_path, 'w', encoding='utf-8') as f:
    yaml.dump(config, f, default_flow_style=False, allow_unicode=True, sort_keys=False)

print(f"\n最终配置已保存: {output_path}")
print(f"threshold_config_hash: {th_hash}")
