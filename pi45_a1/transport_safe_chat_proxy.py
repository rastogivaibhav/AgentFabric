#!/usr/bin/env python3
import argparse
import json
import threading
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
    marker = "\n...[A1.3 deterministic context bound]...\n"
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
    system_chars = sum(
        _text_len(m.get("content")) for m in msgs
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

    bounded_total = sum(_text_len(m.get("content")) for m in msgs if isinstance(m, dict))
    if bounded_total > budget:
        excess = bounded_total - budget
        for m in msgs:
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
    upstream_timeout = 360.0
    log_path = None
    post_lock = threading.Lock()
    state_lock = threading.Lock()
    active_posts = 0
    max_active_posts = 0
    request_seq = 0

    def log_message(self, fmt, *args):
        return

    @classmethod
    def _next_request_id(cls):
        with cls.state_lock:
            cls.request_seq += 1
            return cls.request_seq

    def _record(self, rec):
        if self.log_path:
            with self.state_lock:
                with open(self.log_path, "a", encoding="utf-8") as f:
                    f.write(json.dumps(rec, sort_keys=True) + "\n")

    def _send(self, status, data, content_type="application/json", extra_headers=None):
        client_disconnected = False
        try:
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            if extra_headers:
                for k, v in extra_headers.items():
                    if k.lower() not in {"transfer-encoding", "connection", "content-length", "content-type"}:
                        self.send_header(k, v)
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            client_disconnected = True
        return client_disconnected

    def _forward(self, body=None):
        url = self.target + self.path
        headers = {
            "Content-Type": self.headers.get("Content-Type", "application/json"),
            "Connection": "close",
        }
        req = urllib.request.Request(url, data=body, headers=headers, method=self.command)
        started = time.monotonic()
        try:
            with urllib.request.urlopen(req, timeout=self.upstream_timeout) as r:
                data = r.read()
                elapsed = time.monotonic() - started
                disconnected = self._send(r.status, data, r.headers.get("Content-Type", "application/json"), dict(r.headers.items()))
                return r.status, elapsed, None, disconnected
        except urllib.error.HTTPError as e:
            data = e.read()
            elapsed = time.monotonic() - started
            disconnected = self._send(e.code, data, e.headers.get("Content-Type", "application/json"))
            return e.code, elapsed, f"HTTPError({e.code})", disconnected
        except Exception as exc:
            elapsed = time.monotonic() - started
            payload = json.dumps({"error": {"message": repr(exc), "type": "a13_upstream_transport_error"}}).encode("utf-8")
            disconnected = self._send(504, payload)
            return 504, elapsed, repr(exc), disconnected

    def do_GET(self):
        status, elapsed, error, disconnected = self._forward()
        self._record({
            "method": "GET", "path": self.path, "status": status,
            "upstream_elapsed_s": round(elapsed, 3), "error": error,
            "client_disconnected": disconnected,
        })

    def do_POST(self):
        request_id = self._next_request_id()
        received_wall = time.time()
        received_mono = time.monotonic()
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

        lock_wait_started = time.monotonic()
        with self.post_lock:
            lock_acquired = time.monotonic()
            queue_s = lock_acquired - lock_wait_started
            with self.state_lock:
                type(self).active_posts += 1
                if type(self).active_posts > type(self).max_active_posts:
                    type(self).max_active_posts = type(self).active_posts
                active_now = type(self).active_posts
                max_active = type(self).max_active_posts
            try:
                status, upstream_elapsed, error, disconnected = self._forward(outgoing)
            finally:
                with self.state_lock:
                    type(self).active_posts -= 1
            total_elapsed = time.monotonic() - received_mono

        self._record({
            "request_id": request_id,
            "received_unix": round(received_wall, 6),
            "method": "POST",
            "path": self.path,
            "status": status,
            "queue_s": round(queue_s, 3),
            "upstream_elapsed_s": round(upstream_elapsed, 3),
            "total_elapsed_s": round(total_elapsed, 3),
            "active_at_start": active_now,
            "max_active_observed": max_active,
            "error": error,
            "client_disconnected": disconnected,
            **meta,
        })


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--listen", type=int, default=9090)
    ap.add_argument("--target", default="http://127.0.0.1:9091")
    ap.add_argument("--budget-chars", type=int, default=8000)
    ap.add_argument("--upstream-timeout", type=float, default=360.0)
    ap.add_argument("--log", required=True)
    args = ap.parse_args()
    Handler.target = args.target.rstrip("/")
    Handler.budget = args.budget_chars
    Handler.upstream_timeout = args.upstream_timeout
    Handler.log_path = args.log
    Path(args.log).parent.mkdir(parents=True, exist_ok=True)
    server = ThreadingHTTPServer(("127.0.0.1", args.listen), Handler)
    server.daemon_threads = True
    server.serve_forever()


if __name__ == "__main__":
    main()
