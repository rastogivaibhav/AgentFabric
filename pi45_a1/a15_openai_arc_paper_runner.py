#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from typing import Any

import a15_arc_diagnostic_runner as base
import a15_arc_paper_runner as paper

OPENAI_MODEL = os.getenv("OPENAI_MODEL", "gpt-5.6-sol")
OPENAI_ENDPOINT = os.getenv("OPENAI_RESPONSES_ENDPOINT", "https://api.openai.com/v1/responses")
PROVIDER = "openai"


def _extract_output_text(data: dict[str, Any]) -> str:
    # Responses API may expose output_text in SDKs, but raw HTTP returns output
    # items. Keep extraction deliberately narrow and fail closed.
    direct = data.get("output_text")
    if isinstance(direct, str) and direct:
        return direct
    chunks: list[str] = []
    for item in data.get("output") or []:
        if not isinstance(item, dict) or item.get("type") != "message":
            continue
        for content in item.get("content") or []:
            if not isinstance(content, dict):
                continue
            if content.get("type") in {"output_text", "text"} and isinstance(content.get("text"), str):
                chunks.append(content["text"])
    if not chunks:
        raise RuntimeError("OpenAI Responses API returned no output text")
    return "".join(chunks)


def openai_call_llm(endpoint: str, prompt: str, *, max_tokens: int, timeout: int = 360) -> tuple[str, dict[str, Any]]:
    # endpoint is intentionally ignored: the frozen ARC runner still passes its
    # local endpoint argument, while this capacity-control adapter owns provider routing.
    del endpoint
    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        raise RuntimeError("OPENAI_API_KEY is required for OpenAI capacity-control run")
    body: dict[str, Any] = {
        "model": OPENAI_MODEL,
        "instructions": "Return exactly the requested JSON object and no additional text.",
        "input": prompt,
        "max_output_tokens": int(max_tokens),
        "store": False,
    }
    req = urllib.request.Request(
        OPENAI_ENDPOINT,
        data=json.dumps(body, separators=(",", ":")).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as response:
            data = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        # Never include request headers / key in errors or evidence.
        body_text = exc.read().decode("utf-8", errors="replace")[:2000]
        raise RuntimeError(f"OpenAI HTTP {exc.code}: {body_text}") from None
    usage = data.get("usage") if isinstance(data.get("usage"), dict) else {}
    usage = {**usage, "provider": PROVIDER, "model": OPENAI_MODEL}
    return _extract_output_text(data), usage


def main() -> None:
    # Freeze all DMW behavior. Replace only the proposal/outcome model provider.
    paper._ORIGINAL_CALL_LLM = openai_call_llm
    base.MODEL_ID = OPENAI_MODEL
    base.MODEL_SHA256 = f"api:{PROVIDER}:{OPENAI_MODEL}"
    base.SEED = 0  # API model does not expose/use our local llama.cpp seed.
    paper.main()


if __name__ == "__main__":
    main()
