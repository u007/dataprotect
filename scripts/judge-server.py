"""Minimal OpenAI-compatible chat-completions server for the piiscrub LLM judge.

Exposes POST /v1/chat/completions (what recognizers/judge calls) and GET /healthz.
Uses a GBNF grammar so Qwen3-0.6B can only answer "yes" or "no" (design decision 6).

Model path: $QWEN3_MODEL_PATH (default /tmp/models/Qwen3-0.6B-Q8_0.gguf).
Loopback only (127.0.0.1:8788).
"""
import os
from typing import Optional

from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel
import uvicorn
from llama_cpp import Llama, LlamaGrammar

MODEL_PATH = os.environ.get("QWEN3_MODEL_PATH", "/tmp/models/Qwen3-0.6B-Q8_0.gguf")
GRAMMAR = 'root ::= "yes" | "no"'

app = FastAPI(title="piiscrub judge (Qwen3-0.6B)")
llm: Optional[Llama] = None
grammar: Optional[LlamaGrammar] = None


class Message(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    model: str
    messages: list[Message]
    temperature: float = 0.0
    max_tokens: Optional[int] = None
    stream: bool = False


@app.on_event("startup")
def _load():
    global llm, grammar
    llm = Llama(model_path=MODEL_PATH, n_ctx=1024, seed=42, n_gpu_layers=0, verbose=False)
    grammar = LlamaGrammar.from_string(GRAMMAR)


@app.get("/healthz")
def healthz():
    return {"status": "ok", "model": "Qwen3-0.6B-Q8_0"}


@app.post("/v1/chat/completions")
def chat(req: ChatRequest):
    # Small non-zero temperature so the GBNF-constrained sampler can pick from
    # allowed tokens even when argmax is not in the grammar.
    temperature = req.temperature if req.temperature > 0 else 0.1
    resp = llm.create_chat_completion(
        messages=[m.model_dump() for m in req.messages],
        temperature=temperature,
        max_tokens=req.max_tokens or 4,
        grammar=grammar,
    )
    text = resp["choices"][0]["message"]["content"]
    return JSONResponse(
        {
            "id": "chatcmpl-judge",
            "object": "chat.completion",
            "model": req.model or "qwen3-0.6b",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": text},
                    "finish_reason": "stop",
                }
            ],
        }
    )


if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8788, log_level="error")
