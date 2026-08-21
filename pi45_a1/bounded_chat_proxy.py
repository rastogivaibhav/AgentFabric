#!/usr/bin/env python3
import argparse
import json
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def _text_len(content):
    if isinstance(content, str):
        return len(content)
    if isinstance(content, list):
        total = 0
        for part in content:
            if isinstance(part, dict) and isinstance(part.get("text"), str):
                total += len(part["text"])
        return total
    return 0


def _bound_text(text, limit):
    if len(text) <= limit:
        return text, False
    marker = "\n...[A1.2 deterministic context bound]...\n"
    if limit <= len(marker) + 2:
        return text[:limit], True
    remain = limit - len(marker)
    head = remain // 2
    tail = remain - head
    return text[:head] + marker + text[-tail:], True


def _bound_content(content, limit):
    if isinstance(content, str):
        return _bound_text(content, limit)
    if isinstance(content, list):
        out = []
        remaining = limit
        changed = False
        for part in content:
            if not isinstance(part, dict) or not isinstance(part.get("text"), str):
                out.append(part)
                continue
            new_part = dict(part)
            bounded, did = _bound_text(part["text"], max(0, remaining))
            new_part["text"] = bounded
            remaining = max(0, remaining - len(bounded))
            changed = changed or did
            out.append(new_part)
        return out, changed
    return content, False


def bound_messages(payload, budget):
    messages = payload.get("messages")
    if not isinstance(messages, list):
        return payload, {"original_chars": 0, "bounded_chars": 0, "truncated": False}

    original = sum(_text_len(m.get("content")) for m in messages if isinstance(m, dict))
    if original <= budget:
        return payload, {"original_chars": original, "bounded_chars": original, "truncated": False}

    bounded = json.loads(json.dumps(payload))
    msgs = bounded["messages"]
    # Preserve system instructions. Bound non-system messages from newest to oldest,
    # giving the newest message the remaining budget first. Within an oversized
    # message preserve both head (task/instructions) and tail (latest state/memory).
    system_chars = sum(
        _text_len(m.get("content"))
        for m in msgs
        if isinstance(m, dict) and m.get("role") == "system"
    )
    available = max(512, budget - system_chars)
    non_system = [i for i, m in enumerate(msgs) if isinstance(m, dict) and m.get("role") != "system"]
    remaining = available
    changed = False
    for i in reversed(non_system):
        c = msgs[i].get("content")
        size = _text_len(c)
        allowance = min(size, remaining)
        if allowance < size:
            msgs[i]["content"], did = _bound_content(c, allowance)
            changed = changed or did
        remaining = max(0, remaining - _text_len(msgs[i].get("content")))

    # If system content itself exceeds the configured total budget, bound it last.
    bounded_total = sum(_text_len(m.get("content")) for m in msgs if isinstance(m, dict))
    if bounded_total > budget:
        excess = bounded_total - budget
        for i, m in enumerate(msgs):
            if not isinstance(m, dict) or m.get("role") != "system":
                continue
            size = _text_len(m.get("content"))
            target = max(256, size - excess)
            m["content"], did = _bound_content(m.get("content"), target)
            changed = changed or did
            bounded_total = sum(_text_len(x.get("content")) for x in msgs if isinstance(x, dict))
            if bounded_total <= budget:
                break

    bounded_total = sum(_text_len(m.get("content")) for m in msgs if isinstance(m, dict))
    return bounded, {"original_chars": original, "bounded_chars": bounded_total, "truncated": changed}


class Handler(BaseHTTPRequestHandler):
    target = "http://127.0.0.1:9091"
    budget = 8000
    log_path = None

    def log_message(self, fmt, *args):
        return

    def _record(self, rec):
        if self.log_path:
            with open(self.log_path, "a", encoding="utf-8") as f:
                f.write(json.dumps(rec, sort_keys=True) + "\n")

    def _forward(self, body=None):
        url = self.target + self.path
        headers = {"Content-Type": self.headers.get("Content-Type", "application/json")}
        req = urllib.request.Request(url, data=body, headers=headers, method=self.command)
        started = time.time()
        try:
            with urllib.request.urlopen(req, timeout=180) as r:
                data = r.read()
                self.send_response(r.status)
                for k, v in r.headers.items():
                    if k.lower() not in {"transfer-encoding", "connection", "content-length"}:
                        self.send_header(k, v)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)
                return r.status, time.time() - started
        except urllib.error.HTTPError as e:
            data = e.read()
            self.send_response(e.code)
            self.send_header("Content-Type", e.headers.get("Content-Type", "application/json"))
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return e.code, time.time() - started

    def do_GET(self):
        status, elapsed = self._forward()
        self._record({"method": "GET", "path": self.path, "status": status, "elapsed_s": round(elapsed, 3)})

    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(n)
        meta = {"original_chars": 0, "bounded_chars": 0, "truncated": False}
        outgoing = raw
        if "application/json" in self.headers.get("Content-Type", "application/json"):
            try:
                payload = json.loads(raw.decode("utf-8"))
                payload, meta = bound_messages(payload, self.budget)
                outgoing = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
            except Exception as exc:
                meta["proxy_parse_error"] = repr(exc)
        status, elapsed = self._forward(outgoing)
        self._record({
            "method": "POST",
            "path": self.path,
            "status": status,
            "elapsed_s": round(elapsed, 3),
            **meta,
        })


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--listen", type=int, default=9090)
    ap.add_argument("--target", default="http://127.0.0.1:9091")
    ap.add_argument("--budget-chars", type=int, default=8000)
    ap.add_argument("--log", required=True)
    args = ap.parse_args()
    Handler.target = args.target.rstrip("/")
    Handler.budget = args.budget_chars
    Handler.log_path = args.log
    Path(args.log).parent.mkdir(parents=True, exist_ok=True)
    server = ThreadingHTTPServer(("127.0.0.1", args.listen), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
