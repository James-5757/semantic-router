#!/usr/bin/env python3
# Final evaluation with manually tuned thresholds
import json, yaml

with open("vllm_final_config.yaml", encoding="utf-8") as f:
    config = yaml.safe_load(f)

signals = {s["name"]: s["threshold"] for s in config["routing"]["signals"]["embeddings"]}
decisions = {d["name"]: d["priority"] for d in config["routing"]["decisions"]}

from evaluate_vllm_router import load_test_data, get_signal_scores_from_cache, simulate_decision, evaluate_from_decisions

with open("vllm_signal_cache.json", encoding="utf-8") as f:
    cache = json.load(f)

results = evaluate_from_decisions(cache, signals, decisions)

acc = results["accuracy"]
mf1 = results["macro_f1"]
drr = results["default_route_rate"]

print("=" * 70)
print("  Final manually tuned config - Evaluation Results")
print("=" * 70)
print("  Pool Accuracy:      %.2f%%" % (acc * 100))
print("  Macro F1:           %.2f%%" % (mf1 * 100))
print("  Default-route Rate: %.2f%%" % (drr * 100))
print()

print("  Per-class metrics:")
print("  %-25s %10s %10s %10s  %4s %4s %4s" % ("Class", "Precision", "Recall", "F1", "TP", "FP", "FN"))
print("  " + "-" * 72)
for cls_name in sorted(results["classes"].keys()):
    d = results["classes"][cls_name]
    print("  %-25s %9.2f%% %9.2f%% %9.2f%%  %4d %4d %4d" % (
        cls_name, d["precision"]*100, d["recall"]*100, d["f1"]*100,
        d["tp"], d["fp"], d["fn"]))

print()
print("  Confusion Matrix:")
cm = results["confusion_matrix"]
classes = sorted(cm.keys())
print("  %-15s" % "predict/truth", end="")
for c in classes:
    print("%15s" % c, end="")
print()
for truth in classes:
    print("  %-15s" % truth, end="")
    for pred in classes:
        print("%15d" % cm[truth].get(pred, 0), end="")
    print()

print()
print("  Thresholds used:")
for name, th in sorted(signals.items()):
    print("    %s: %.2f" % (name, th))

print()
print("  Priorities used:")
for name, pri in sorted(decisions.items(), key=lambda x: x[1], reverse=True):
    print("    %s: %d" % (name, pri))
