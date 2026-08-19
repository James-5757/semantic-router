#!/usr/bin/env python3
"""
Real Embedding Service using multilingual-e5-small
Optimized version: Cache pool embeddings at startup, warmup, selective rerank
"""

import os
import time
import argparse
from typing import Dict, List, Optional, Any
from pathlib import Path

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import numpy as np
from sentence_transformers import SentenceTransformer
from sklearn.metrics.pairwise import cosine_similarity

# ============== Pool Samples Data (Improved) ==============
POOL_SAMPLES = {
    "code_pool": [
        # English - 明确是编程任务
        "write a quick sort algorithm in Python",
        "implement a binary search tree in Java",
        "debug this code and fix the error",
        "optimize this SQL query performance",
        "write a login function in JavaScript",
        "create a REST API endpoint in Go",
        "write a unit test for this function",
        "refactor this code for better readability",
        "fix the memory leak in this program",
        "implement a distributed lock in Python",
        "write Python code to process data",
        "build a microservice in Go",
        "create a Docker configuration",
        "write a Python script to automate tasks",
        # Chinese - 明确是编程任务
        "帮我写一个排序算法",
        "用 Java 实现一个二分查找",
        "帮我调试这段代码",
        "优化这个 SQL 查询速度",
        "写一个用户登录功能",
        "创建一个 REST API 接口",
        "写单元测试",
        "重构这段代码",
        "开发一个爬虫程序",
        "写一个算法实现",
    ],
    "data_pool": [
        # English - 明确是数据分析任务，不是编程
        "analyze this sales data and find trends",
        "generate a chart from this CSV",
        "calculate the conversion rate by channel",
        "build a prediction model for sales",
        "clean this messy dataset",
        "create a dashboard for metrics",
        "perform clustering analysis on users",
        "visualize this data as a line chart",
        "extract key insights from this report",
        "forecast next quarter revenue",
        "process Excel file and calculate metrics",
        "analyze customer behavior patterns",
        "create data visualization from CSV",
        # Chinese - 明确是数据分析任务
        "分析这份销售数据",
        "根据数据生成图表",
        "计算每个渠道的转化率",
        "建立一个预测模型",
        "清洗这份数据",
        "创建一个仪表盘",
        "对用户进行聚类分析",
        "可视化这些数据",
        "从报告中提取关键数据",
        "预测下季度收入",
    ],
    "document_pool": [
        # English
        "summarize this PDF document",
        "extract key points from this Word file",
        "translate this contract to English",
        "review this agreement for risks",
        "write a technical proposal",
        "create a monthly report",
        "analyze this research paper",
        "extract tables from this PDF",
        "proofread this article",
        "generate a summary of this doc",
        "read and summarize this document",
        "answer questions about this PDF",
        # Chinese
        "总结这个 PDF",
        "提取 Word 文档要点",
        "翻译这份合同",
        "审查协议风险",
        "写一份技术方案",
        "创建月度报告",
        "分析这篇论文",
        "从 PDF 提取表格",
        "校对这篇文章",
        "生成文档摘要",
    ],
    "vision_pool": [
        # English - 理解/分析已有图片，不是生成
        "describe what's in this image",
        "read text from this screenshot",
        "analyze this photo for errors",
        "extract information from this chart",
        "what does this diagram show",
        "identify objects in this picture",
        "describe the scene in this photo",
        "analyze this graph",
        "what text is in this image",
        "explain what's shown in this screenshot",
        "understand this diagram",
        "analyze this picture content",
        # Chinese
        "描述这张图片的内容",
        "读取截图中的文字",
        "分析这张照片",
        "提取图表中的信息",
        "这张图显示了什么",
        "识别图片中的物体",
        "描述这个场景",
        "分析这个图表",
        "读取手写文字",
        "这张截图显示什么",
    ],
    "image_generation_pool": [
        # English - 生成新图片，不是分析
        "generate a poster for the event",
        "create an illustration for the article",
        "draw a landscape painting",
        "generate a logo design",
        "create a digital art piece",
        "make a product photo background",
        "generate a comic strip",
        "create a banner image",
        "design a social media post",
        "generate an AI artwork",
        "create a new image of a robot",
        "draw a futuristic city",
        "make an illustration for story",
        # Chinese
        "生成一张海报",
        "创建一张插画",
        "画一幅风景画",
        "生成一个标志",
        "创建数字艺术作品",
        "制作产品图背景",
        "生成漫画",
        "创建横幅图片",
        "设计社交媒体帖子",
        "生成 AI 艺术",
        "帮我画一个机器人",
        "生成一张科技感图片",
    ],
    "cheap_chat_pool": [
        # English - 简单对话
        "hello world",
        "how are you today",
        "what is the weather",
        "recommend a movie",
        "translate this to Chinese",
        "what time is it",
        "simple math calculation",
        "basic explanation of AI",
        "quick greeting",
        "short fact query",
        # Chinese
        "你好",
        "今天天气怎么样",
        "推荐一部电影",
        "翻译成中文",
        "现在几点了",
        "简单计算",
        "什么是人工智能",
        "日常问候",
        "简短问答",
        "基础解释",
    ],
    "general_pool": [
        # English - 模糊/低置信度 fallback
        "general conversation",
        "open ended question",
        "casual chat",
        "uncertain intent",
        "low confidence routing",
        # Chinese
        "随便聊聊",
        "开放式问题",
        "日常对话",
        "不确定的意图",
        "低置信度路由",
    ],
}

app = FastAPI(title="Embedding Pool Router", version="2.0.0")

# Global state
model: Optional[SentenceTransformer] = None
model_path: str = ""
model_loaded: bool = False

# Cached embeddings
pool_embeddings: Dict[str, np.ndarray] = {}  # Pool name -> centroid embedding
pool_sample_embeddings: Dict[str, List[np.ndarray]] = {}  # Pool name -> list of sample embeddings

# Metrics
metrics = {
    "total_requests": 0,
    "warmup_requests": 0,
    "inference_latencies": [],
    "errors": 0,
}


class PoolRouteRequest(BaseModel):
    prompt: str
    candidate_pools: Optional[List[str]] = None  # For selective rerank: only re-rank these pools
    top_k: Optional[int] = 2  # Return top k pools


class PoolRouteResponse(BaseModel):
    best_pool: str
    best_score: float
    second_best_pool: Optional[str] = None
    second_best_score: float = 0.0
    score_margin: float = 0.0
    scores: Dict[str, float] = {}
    latency_ms: float = 0.0
    model_name: str = "multilingual-e5-small"
    is_reranked: bool = False  # True if using selective rerank


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
    model_path: str
    embedding_dimension: int = 384
    pools_cached: int = 0
    service_ready: bool = False


def load_model(path: str) -> bool:
    """Load the sentence transformer model only once at startup"""
    global model, model_path, model_loaded

    try:
        print(f"Loading model from: {path}")
        model = SentenceTransformer(
            path,
            local_files_only=True,  # MUST use local only
            device="cpu"
        )
        model_path = path
        model_loaded = True
        dim = model.get_embedding_dimension()
        print(f"Model loaded successfully! Embedding dimension: {dim}")
        return True
    except Exception as e:
        print(f"Failed to load model: {e}")
        model_loaded = False
        return False


def cache_pool_embeddings() -> bool:
    """Pre-compute and cache all pool embeddings at startup"""
    global pool_embeddings, pool_sample_embeddings

    if model is None:
        return False

    try:
        print("Caching pool embeddings...")

        for pool_name, samples in POOL_SAMPLES.items():
            # Encode all samples with passage prefix
            texts = [f"passage: {s}" for s in samples]
            embeddings = model.encode(texts, normalize_embeddings=True)

            # Store individual sample embeddings
            pool_sample_embeddings[pool_name] = [emb for emb in embeddings]

            # Compute centroid (mean)
            centroid = np.mean(embeddings, axis=0)
            centroid = centroid / np.linalg.norm(centroid)  # Re-normalize
            pool_embeddings[pool_name] = centroid

        print(f"Cached {len(pool_embeddings)} pool embeddings")
        return True
    except Exception as e:
        print(f"Failed to cache pool embeddings: {e}")
        return False


def warmup_requests(num_warmup: int = 20):
    """Warmup the service with dummy requests"""
    global metrics

    print(f"Running {num_warmup} warmup requests...")

    test_prompts = [
        "hello world",
        "帮我写代码",
        "分析数据",
        "描述这张图片",
        "生成一张海报",
    ]

    for i in range(num_warmup):
        prompt = test_prompts[i % len(test_prompts)]
        try:
            _ = model.encode(f"query: {prompt}", normalize_embeddings=True)
            metrics["warmup_requests"] += 1
        except Exception as e:
            print(f"Warmup request {i} failed: {e}")

    print(f"Warmup complete: {metrics['warmup_requests']} successful")


def encode_prompt(prompt: str) -> np.ndarray:
    """Encode user prompt with query prefix"""
    if model is None:
        raise RuntimeError("Model not loaded")
    return model.encode(f"query: {prompt}", normalize_embeddings=True)


def compute_similarities(prompt_embedding: np.ndarray, pool_names: List[str]) -> Dict[str, float]:
    """Compute cosine similarity between prompt and specified pools"""
    scores = {}
    for pool_name in pool_names:
        if pool_name in pool_embeddings:
            # Use pre-computed centroid
            similarity = cosine_similarity(
                prompt_embedding.reshape(1, -1),
                pool_embeddings[pool_name].reshape(1, -1)
            )[0][0]
            scores[pool_name] = float(similarity)
        elif pool_name in pool_sample_embeddings:
            # Fallback: compute on the fly (for dynamic pools)
            embeddings = pool_sample_embeddings[pool_name]
            similarities = cosine_similarity(
                prompt_embedding.reshape(1, -1),
                embeddings
            )[0]
            scores[pool_name] = float(np.max(similarities))
    return scores


@app.on_event("startup")
async def startup_event():
    """Load model and cache embeddings on startup"""
    pass  # Handled in main()


@app.get("/health", response_model=HealthResponse)
async def health():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy" if model_loaded else "model_not_loaded",
        model_loaded=model_loaded,
        model_path=model_path,
        embedding_dimension=384,
        pools_cached=len(pool_embeddings),
        service_ready=model_loaded and len(pool_embeddings) > 0
    )


@app.post("/route/pool", response_model=PoolRouteResponse)
async def route_pool(request: PoolRouteRequest):
    """Route prompt to best matching pool"""
    global metrics

    if not model_loaded or model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    start_time = time.time()

    try:
        # Encode user prompt (only this is done per request)
        prompt_embedding = encode_prompt(request.prompt)

        # Determine which pools to score
        if request.candidate_pools and len(request.candidate_pools) > 0:
            # Selective rerank: only score specified pools
            pools_to_score = request.candidate_pools
            is_reranked = True
        else:
            # Full classification: score all pools
            pools_to_score = list(POOL_SAMPLES.keys())
            is_reranked = False

        # Compute similarities (uses cached embeddings)
        scores = compute_similarities(prompt_embedding, pools_to_score)

        # Sort by score descending
        sorted_pools = sorted(scores.items(), key=lambda x: x[1], reverse=True)

        # Get top results
        best_pool = sorted_pools[0][0]
        best_score = sorted_pools[0][1]

        if len(sorted_pools) > 1:
            second_best_pool = sorted_pools[1][0]
            second_best_score = sorted_pools[1][1]
            score_margin = best_score - second_best_score
        else:
            second_best_pool = None
            second_best_score = 0.0
            score_margin = 0.0

        inference_latency = (time.time() - start_time) * 1000
        metrics["total_requests"] += 1
        metrics["inference_latencies"].append(inference_latency)

        return PoolRouteResponse(
            best_pool=best_pool,
            best_score=best_score,
            second_best_pool=second_best_pool,
            second_best_score=second_best_score,
            score_margin=score_margin,
            scores=scores,
            latency_ms=inference_latency,
            model_name="multilingual-e5-small",
            is_reranked=is_reranked
        )

    except Exception as e:
        metrics["errors"] += 1
        raise HTTPException(status_code=500, detail=f"Routing error: {str(e)}")


@app.get("/metrics")
async def get_metrics():
    """Get service metrics"""
    latencies = metrics["inference_latencies"]
    if len(latencies) > 0:
        sorted_latencies = sorted(latencies)
        p50 = sorted_latencies[int(len(sorted_latencies) * 0.50)]
        p95 = sorted_latencies[int(len(sorted_latencies) * 0.95)]
        p99 = sorted_latencies[int(len(sorted_latencies) * 0.99)]
    else:
        p50 = p95 = p99 = 0

    return {
        "total_requests": metrics["total_requests"],
        "warmup_requests": metrics["warmup_requests"],
        "errors": metrics["errors"],
        "latency_avg_ms": sum(latencies) / len(latencies) if latencies else 0,
        "latency_p50_ms": p50,
        "latency_p95_ms": p95,
        "latency_p99_ms": p99,
    }


@app.get("/")
async def root():
    """Root endpoint"""
    return {
        "service": "Embedding Pool Router v2.0",
        "model": "multilingual-e5-small",
        "status": "running" if model_loaded else "model_not_loaded",
        "optimizations": [
            "cached_pool_embeddings",
            "selective_rerank",
            "warmup_requests"
        ],
        "endpoints": ["/health", "/route/pool", "/metrics"]
    }


if __name__ == "__main__":
    import uvicorn

    parser = argparse.ArgumentParser(description="Embedding Pool Router Service")
    parser.add_argument(
        "--model-path",
        type=str,
        default="./models/multilingual-e5-small",
        help="Path to the model directory"
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8001,
        help="Port to listen on"
    )
    parser.add_argument(
        "--warmup",
        type=int,
        default=20,
        help="Number of warmup requests"
    )
    args = parser.parse_args()

    # Load model
    if not load_model(args.model_path):
        print("Failed to load model, exiting...")
        exit(1)

    # Cache pool embeddings
    if not cache_pool_embeddings():
        print("Failed to cache pool embeddings, exiting...")
        exit(1)

    # Warmup
    warmup_requests(args.warmup)

    print(f"Starting server on port {args.port}")
    uvicorn.run(app, host="0.0.0.0", port=args.port)