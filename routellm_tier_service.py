"""Local, shadow-only RouteLLM BERT tier scorer.

This service never calls ``Controller.chat.completions`` and never contacts a
strong or weak upstream model.  It only evaluates the local BERT router's
``calculate_strong_win_rate(prompt)`` method.

Set ROUTELLM_TIER_ENABLE_LOCAL_MODEL=1 only after the official checkpoint is
already available locally.  The safe default exposes an unavailable health
state rather than downloading a model during service startup.
"""

import os
import time
from typing import Any, Optional

# IMPORTANT: Must set BEFORE importing routellm
# RouteLLM 0.2 imports a LiteLLM/OpenAI client eagerly, even though this service
# never makes a completion request. A non-secret placeholder only satisfies that
# constructor; it cannot enable an upstream call because no completion API is used.
os.environ.setdefault("OPENAI_API_KEY", "routellm-local-router-placeholder")

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

ROUTER_NAME = "bert"
CHECKPOINT = os.getenv("ROUTELLM_TIER_CHECKPOINT", "routellm/bert_gpt4_augmented")
WEAK_THRESHOLD = float(os.getenv("ROUTELLM_TIER_WEAK_THRESHOLD", "0.35"))
STRONG_THRESHOLD = float(os.getenv("ROUTELLM_TIER_STRONG_THRESHOLD", "0.65"))
ENABLE_LOCAL_MODEL = os.getenv("ROUTELLM_TIER_ENABLE_LOCAL_MODEL", "0") == "1"

app = FastAPI(title="RouteLLM Tier Service", version="0.1.0")
_router: Optional[Any] = None
_load_error: Optional[str] = None


class ScoreRequest(BaseModel):
    prompt: str = Field(min_length=1, max_length=32768)


def suggested_tier(score: float) -> str:
    if score < WEAK_THRESHOLD:
        return "weak"
    if score < STRONG_THRESHOLD:
        return "medium"
    return "strong"


def load_local_router() -> None:
    """Load the official BERT router only; no chat/completion client is used."""
    global _router, _load_error
    if _router is not None or _load_error is not None:
        return
    if not ENABLE_LOCAL_MODEL:
        _load_error = "local RouteLLM model loading is disabled"
        return
    try:
        from routellm.controller import Controller

        # RouteLLM 0.2 requires model names to describe the pair, but loading
        # the BERT router does not call either of them. `config` is the official
        # Controller argument used to pass the checkpoint to BERTRouter.
        controller = Controller(
            routers=[ROUTER_NAME],
            strong_model="gpt-4",
            weak_model="gpt-3.5-turbo",
            config={ROUTER_NAME: {"checkpoint_path": CHECKPOINT}},
            api_key=os.environ["OPENAI_API_KEY"],
        )

        routers = getattr(controller, "routers", None)
        if isinstance(routers, dict):
            _router = routers.get(ROUTER_NAME)
        elif isinstance(routers, (list, tuple)) and routers:
            _router = routers[0]
        else:
            _router = getattr(controller, "router", None)
        if _router is None or not hasattr(_router, "calculate_strong_win_rate"):
            raise RuntimeError("RouteLLM BERT router does not expose calculate_strong_win_rate")
    except Exception as exc:  # health endpoint reports a local dependency/configuration error
        _load_error = f"RouteLLM unavailable: {exc}"


@app.on_event("startup")
def startup() -> None:
    load_local_router()


@app.get("/health")
def health() -> dict[str, Any]:
    load_local_router()
    return {
        "status": "ok" if _router is not None else "unavailable",
        "router": ROUTER_NAME,
        "checkpoint": CHECKPOINT,
        "shadow_only": True,
        "upstream_called": False,
        "weak_threshold": WEAK_THRESHOLD,
        "strong_threshold": STRONG_THRESHOLD,
        "error": _load_error,
    }


@app.post("/score")
def score(request: ScoreRequest) -> dict[str, Any]:
    load_local_router()
    if _router is None:
        raise HTTPException(status_code=503, detail=_load_error or "RouteLLM BERT router unavailable")
    started = time.perf_counter()
    try:
        probability = float(_router.calculate_strong_win_rate(request.prompt))
    except Exception as exc:
        raise HTTPException(status_code=503, detail=f"RouteLLM scoring failed: {exc}") from exc
    latency_ms = round((time.perf_counter() - started) * 1000, 3)
    return {
        "router": ROUTER_NAME,
        "strong_win_probability": probability,
        "suggested_tier": suggested_tier(probability),
        "weak_threshold": WEAK_THRESHOLD,
        "strong_threshold": STRONG_THRESHOLD,
        "latency_ms": latency_ms,
        "shadow_only": True,
        "upstream_called": False,
    }
