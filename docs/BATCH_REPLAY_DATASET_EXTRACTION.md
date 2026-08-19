# Batch Replay Dataset Extraction

## Overview

Extract real user prompts from Hugging Face conversation datasets for use with the Semantic Router's Batch Replay system.

The extraction is **deterministic**: no LLM API calls, no model inference, no network calls to the router services. Only Hugging Face dataset downloads.

## Installation

```bash
pip install datasets
```

## Hugging Face Authentication

Some datasets (e.g., `lmsys/lmsys-chat-1m`) require authentication:

```bash
# Option 1: Login via CLI
huggingface-cli login
# Enter your token when prompted

# Option 2: Set environment variable
export HF_TOKEN=hf_your_token_here
```

The tool detects tokens from:
1. `HF_TOKEN` environment variable
2. `HUGGINGFACE_TOKEN` environment variable
3. Hugging Face CLI cache at `~/.cache/huggingface/token`

**The token is never printed to logs. The tool validates token presence but never outputs it.**

## Usage

### Basic Usage

Default: extract 1000 first-user prompts from lmsys/lmsys-chat-1m:

```bash
python scripts/build_replay_dataset.py
```

### PowerShell

```powershell
python scripts/build_replay_dataset.py `
    --dataset lmsys/lmsys-chat-1m `
    --max-rows 5000 `
    --languages en zh `
    --message-mode first_user `
    --seed 42 `
    --run-id my_extraction_1
```

### Bash

```bash
python scripts/build_replay_dataset.py \
    --dataset lmsys/lmsys-chat-1m \
    --max-rows 5000 \
    --languages en zh \
    --message-mode first_user \
    --seed 42 \
    --run-id my_extraction_1
```

### Extract All User Messages (not just first)

```bash
python scripts/build_replay_dataset.py \
    --dataset HuggingFaceH4/ultrachat_200k \
    --max-rows 2000 \
    --message-mode all_user \
    --seed 123
```

## Arguments

| Argument | Default | Description |
|----------|---------|-------------|
| `--dataset` | `lmsys/lmsys-chat-1m` | Hugging Face dataset name |
| `--max-rows` | `1000` | Maximum number of valid prompts |
| `--languages` | (all) | Language codes to filter (e.g., `en zh`) |
| `--min-length` | `10` | Minimum prompt length in characters |
| `--max-length` | `4096` | Maximum prompt length in characters |
| `--message-mode` | `first_user` | `first_user`, `all_user`, or `last_user` |
| `--seed` | `42` | Seed for reproducible dataset split |
| `--run-id` | auto | Custom output directory name |
| `--output-dir` | auto | Custom output directory path |

## Output Format

### Directory Structure

```
datasets/generated/<run_id>/
   natural_replay.jsonl         # 70% of prompts — for batch replay
   annotation_candidates.jsonl  # 20% — for human review/annotation
   holdout_candidates.jsonl     # 10% — for final holdout evaluation
   extraction_manifest.json     # Extraction metadata and statistics
   extraction_errors.jsonl       # Records that failed validation
```

### JSONL Record Format

```json
{
  "id": "nat_000001",
  "prompt": "Write a Python function...",
  "prompt_hash": "a1b2c3d4e5f6...",
  "language": "en",
  "source": "lmsys_lmsys-chat-1m",
  "has_image": false,
  "has_document": false,
  "has_data": false,
  "metadata_source": "dataset_text_only"
}
```

### Privacy

- Prompts are always included in the output (the tool extracts user queries, not model responses)
- No full conversation context is saved
- No metadata about individual users is preserved
- SHA-256 hash is used for dedup (not as an identifier for individuals)
- Token is never written to output files or logs

### Data Split

The split is **hash-based and reproducible**:
- `natural_replay` (70%): Prompts for automated batch replay testing
- `annotation_candidates` (20%): Prompts for human review and expected label annotation
- `holdout_candidates` (10%): Prompts held back from development for final evaluation

No prompt appears in more than one split.

### Supported Datasets

| Dataset | Format | Notes |
|---------|--------|-------|
| `lmsys/lmsys-chat-1m` | `conversation` | May require HF login |
| `lmsys/lmsys-chat-1m-clean` | `conversation` | Filtered version |
| `anon8231489123/ShareGPT_Vicuna_unfiltered` | `conversations` | ShareGPT format |
| `Aeala/ShareGPT_Vicuna_unfiltered` | `conversations` | ShareGPT format |
| `OpenAssistant/oasst1` | `role`/`text` | Per-message records |
| `OpenAssistant/oasst2` | `role`/`text` | Per-message records |
| `HuggingFaceH4/ultrachat_200k` | `messages` | Multi-turn chat |

## Testing

```bash
python scripts/test_build_replay_dataset.py
```

Runs 50 tests covering:
- Three conversation field formats
- Three message extraction modes
- Language detection (en, zh, ko, mixed)
- SHA-256 dedup consistency
- Hash-based split reproducibility
- JSONL I/O with internal field cleanup
- Edge cases (empty, non-dict, invalid content)

## Integration with Batch Replay

This tool only **extracts** prompts. The Batch Replay step (routing prompts through the Semantic Router) is a separate, subsequent process.

```bash
# Step 1: Extract prompts
python scripts/build_replay_dataset.py --max-rows 1000

# Step 2: Import into Playground for batch replay
# (Upload datasets/generated/<run_id>/natural_replay.jsonl 
#  via the Playground Import interface at /v1/debug/events/import)
```

## Safety Constraints

The extraction tool:
- **Never calls 8080, 8082, or 8899**
- **Never calls any LLM API or model inference**
- **Never modifies the Router, Scheduler, RouteLLM, or Holdout V2**
- **Never saves full conversation context**
- **Never adds automatic expected_intent/pool/tier labels** (requires human review)
