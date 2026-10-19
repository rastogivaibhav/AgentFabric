#!/usr/bin/env python3
"""Persistent enterprise agents backed by real SPACE Firecracker computers.

This is a local vertical slice: agent identity, conversation, bounded model loop,
persistent computer, AGS authorization, PUL identity/receipts and GovAgn tracing.
It is deliberately bound to loopback and does not claim production authentication.
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import sys
import threading
import time
import uuid
from typing import Any
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import ProxyHandler, Request, build_opener

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse, HTMLResponse
from pydantic import BaseModel, Field

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor


ROOT = Path(__file__).resolve().parent
REPO_ROOT = ROOT.parent
STATE_DIR = ROOT / "state"
DB_PATH = STATE_DIR / "enterprise-agents.sqlite3"
JOURNAL_PATH = STATE_DIR / "space-missions.sqlite3"
SECRET_DIR = STATE_DIR / "secrets"
PROVIDER_DIR = Path(os.environ.get(
    "AGENTFABRIC_PROVIDER_DIR",
    str(REPO_ROOT / "integrations" / "space_provider"),
))

SPACE_API = os.environ.get("SPACE_API", "http://127.0.0.1:18081")
SPACE_DATA = os.environ.get("SPACE_DATA", "http://127.0.0.1:18091")
AGS_ORIGIN = os.environ.get("AGS_ORIGIN", "http://127.0.0.1:8000")
PUL_ORIGIN = os.environ.get("PUL_ORIGIN", "http://127.0.0.1:8082")
OLLAMA_ORIGIN = os.environ.get("OLLAMA_ORIGIN", "http://127.0.0.1:11434")
GOVAGN_ORIGIN = os.environ.get("GOVAGN_ORIGIN", "http://127.0.0.1:28081")
MODEL = os.environ.get("ENTERPRISE_AGENT_MODEL", "qwen2.5-coder:1.5b")
TENANT = "enterprise-local"
MAX_STEPS = 8
MAX_CONTENT = 12_000
BROWSER_IMAGE = os.environ.get("ENTERPRISE_AGENT_BROWSER_IMAGE", "mcr.microsoft.com/playwright:v1.55.0-noble")
BROWSER_PORT = 9222
ARTIFACT_DIR = STATE_DIR / "artifacts"

sys.path.insert(0, str(PROVIDER_DIR))
from ags_authorizer import AGSPermitSource  # noqa: E402
from mission_broker import MissionBroker, MissionJournal  # noqa: E402
from space_client import SpaceComputerProvider, SpaceError  # noqa: E402


STATE_DIR.mkdir(parents=True, exist_ok=True)
SECRET_DIR.mkdir(parents=True, exist_ok=True)
ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
try:
    SECRET_DIR.chmod(0o700)
except OSError:
    pass

DB_LOCK = threading.RLock()
SESSION_LOCKS: dict[str, threading.Lock] = {}


def connect() -> sqlite3.Connection:
    db = sqlite3.connect(DB_PATH, timeout=30, check_same_thread=False)
    db.row_factory = sqlite3.Row
    db.execute("PRAGMA journal_mode=WAL")
    db.execute("PRAGMA foreign_keys=ON")
    return db


DB = connect()
with DB:
    DB.executescript(
        """
        CREATE TABLE IF NOT EXISTS agents (
          id TEXT PRIMARY KEY,
          name TEXT NOT NULL,
          instructions TEXT NOT NULL,
          model TEXT NOT NULL,
          ags_agent_id TEXT NOT NULL UNIQUE,
          pul_app_id TEXT NOT NULL UNIQUE,
          created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS sessions (
          id TEXT PRIMARY KEY,
          agent_id TEXT NOT NULL REFERENCES agents(id),
          run_id TEXT NOT NULL UNIQUE,
          sandbox_id TEXT NOT NULL UNIQUE,
          create_operation_id TEXT NOT NULL,
          state TEXT NOT NULL,
          created_at TEXT NOT NULL,
          last_activity_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS messages (
          id INTEGER PRIMARY KEY AUTOINCREMENT,
          session_id TEXT NOT NULL REFERENCES sessions(id),
          role TEXT NOT NULL,
          content TEXT NOT NULL,
          created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS turns (
          id TEXT PRIMARY KEY,
          session_id TEXT NOT NULL REFERENCES sessions(id),
          state TEXT NOT NULL,
          trace_id TEXT,
          reply TEXT,
          created_at TEXT NOT NULL,
          completed_at TEXT
        );
        CREATE TABLE IF NOT EXISTS steps (
          id INTEGER PRIMARY KEY AUTOINCREMENT,
          turn_id TEXT NOT NULL REFERENCES turns(id),
          seq INTEGER NOT NULL,
          kind TEXT NOT NULL,
          tool TEXT,
          rationale TEXT NOT NULL,
          status TEXT NOT NULL,
          output TEXT,
          ags_decision TEXT,
          pul_receipt TEXT,
          space_process_id TEXT,
          created_at TEXT NOT NULL,
          UNIQUE(turn_id, seq)
        );
        """
    )
    columns = {row[1] for row in DB.execute("PRAGMA table_info(sessions)")}
    if "browser_container" not in columns:
        DB.execute("ALTER TABLE sessions ADD COLUMN browser_container TEXT")
    if "browser_port" not in columns:
        DB.execute("ALTER TABLE sessions ADD COLUMN browser_port INTEGER")
    if "browser_state" not in columns:
        DB.execute("ALTER TABLE sessions ADD COLUMN browser_state TEXT NOT NULL DEFAULT 'stopped'")


def utcnow() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def required_env(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def json_request(origin: str, path: str, *, method: str = "GET", body=None, headers=None,
                 timeout: float = 120) -> tuple[int, Any]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = Request(
        origin + path,
        data=data,
        method=method,
        headers={"Accept": "application/json", "Content-Type": "application/json", **(headers or {})},
    )
    try:
        with build_opener(ProxyHandler({})).open(req, timeout=timeout) as response:
            return response.status, json.loads(response.read(8_388_609) or b"null")
    except HTTPError as error:
        raw = error.read(1_048_577)
        try:
            result = json.loads(raw or b"null")
        except (ValueError, UnicodeDecodeError):
            result = {"error": f"HTTP {error.code}"}
        return error.code, result


def docker(*args: str, check: bool = True) -> str:
    result = subprocess.run(
        ["docker", *args], text=True, capture_output=True, timeout=120, check=False,
    )
    if check and result.returncode != 0:
        raise RuntimeError((result.stderr or result.stdout or "Docker command failed").strip())
    return result.stdout.strip()


def browser_container_name(session_id: str) -> str:
    if not re.fullmatch(r"ses_[a-f0-9]{12}", session_id):
        raise ValueError("invalid session id")
    return "agentkraft-browser-" + session_id[4:]


def browser_status(session: sqlite3.Row) -> dict[str, Any]:
    name = session["browser_container"] or browser_container_name(session["id"])
    running = docker("inspect", "-f", "{{.State.Running}}", name, check=False) == "true"
    return {
        "state": "running" if running else "stopped",
        "container": name if running else None,
        "port": session["browser_port"] if running else None,
        "execution_plane": "dedicated Docker browser sidecar",
        "computer_plane": "SPACE Firecracker microVM",
    }


def ensure_browser(session: sqlite3.Row) -> dict[str, Any]:
    current = browser_status(session)
    if current["state"] == "running" and current["port"]:
        return current
    name = browser_container_name(session["id"])
    docker("rm", "-f", name, check=False)
    launch = (
        "chrome=$(find /ms-playwright -type f -path '*/chrome-linux/chrome' -print -quit); "
        "test -n \"$chrome\"; exec \"$chrome\" --headless=new --no-sandbox "
        "--disable-dev-shm-usage --disable-gpu --remote-allow-origins=* "
        "--remote-debugging-address=0.0.0.0 --remote-debugging-port=9222 "
        "--user-data-dir=/agent-profile about:blank"
    )
    docker(
        "run", "-d", "--rm", "--name", name, "--shm-size=1gb",
        "-p", f"127.0.0.1::{BROWSER_PORT}",
        "--label", f"agentkraft.session={session['id']}",
        BROWSER_IMAGE, "bash", "-lc", launch,
    )
    mapping = docker("port", name, f"{BROWSER_PORT}/tcp")
    match = re.search(r":([0-9]{2,5})$", mapping)
    if not match:
        docker("rm", "-f", name, check=False)
        raise RuntimeError("browser sidecar did not publish a loopback port")
    port = int(match.group(1))
    ready = False
    for _ in range(80):
        try:
            code, info = json_request(f"http://127.0.0.1:{port}", "/json/version", timeout=1)
            if code == 200 and isinstance(info, dict) and info.get("webSocketDebuggerUrl"):
                ready = True
                break
        except Exception:
            pass
        time.sleep(0.25)
    if not ready:
        logs = docker("logs", "--tail", "50", name, check=False)
        docker("rm", "-f", name, check=False)
        raise RuntimeError("browser sidecar did not become ready: " + logs[-1000:])
    with DB_LOCK, DB:
        DB.execute(
            "UPDATE sessions SET browser_container=?,browser_port=?,browser_state='running' WHERE id=?",
            (name, port, session["id"]),
        )
    return {"state": "running", "container": name, "port": port,
            "execution_plane": "dedicated Docker browser sidecar",
            "computer_plane": "SPACE Firecracker microVM"}


def safe_browser_url(value: Any) -> str:
    if not isinstance(value, str) or len(value) > 2000:
        raise ValueError("url must be text no longer than 2000 characters")
    parsed = urlsplit(value)
    if parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username or parsed.password:
        raise ValueError("only credential-free http(s) URLs are allowed")
    return value


def browser_observation(page, session_id: str, artifact_id: str) -> dict[str, Any]:
    folder = ARTIFACT_DIR / session_id
    folder.mkdir(parents=True, exist_ok=True)
    filename = artifact_id + ".png"
    page.screenshot(path=str(folder / filename), full_page=False)
    data = page.evaluate("""() => {
      const clean = s => (s || '').replace(/\\s+/g, ' ').trim().slice(0, 180);
      const items = [...document.querySelectorAll('a,button,input,textarea,select,[role=button]')]
        .filter(e => { const r=e.getBoundingClientRect(); return r.width>0 && r.height>0; })
        .slice(0, 60).map((e,i) => ({
          index:i+1, tag:e.tagName.toLowerCase(), text:clean(e.innerText || e.value || e.placeholder || e.getAttribute('aria-label')),
          id:e.id || null, name:e.getAttribute('name'), type:e.getAttribute('type'), href:e.href || null
        }));
      return {text: clean(document.body?.innerText).slice(0, 6000), elements: items};
    }""")
    return {
        "url": page.url,
        "title": page.title(),
        "text": data.get("text", ""),
        "elements": data.get("elements", []),
        "screenshot": f"/api/artifacts/{session_id}/{filename}",
    }


def run_browser_tool(session: sqlite3.Row, tool: str, args: dict[str, Any], artifact_id: str) -> str:
    state = ensure_browser(session)
    from playwright.sync_api import sync_playwright

    with sync_playwright() as runtime:
        browser = runtime.chromium.connect_over_cdp(f"http://127.0.0.1:{state['port']}")
        context = browser.contexts[0] if browser.contexts else browser.new_context()
        page = context.pages[0] if context.pages else context.new_page()
        page.set_default_timeout(20_000)
        if tool == "browser.open":
            page.goto(safe_browser_url(args.get("url")), wait_until="domcontentloaded", timeout=45_000)
        elif tool == "browser.click":
            selector = args.get("selector")
            if not isinstance(selector, str) or not selector or len(selector) > 300:
                raise ValueError("selector must be 1-300 characters")
            page.locator(selector).first.click()
            page.wait_for_timeout(500)
        elif tool == "browser.type":
            selector, value = args.get("selector"), args.get("text")
            if not isinstance(selector, str) or not selector or len(selector) > 300:
                raise ValueError("selector must be 1-300 characters")
            if not isinstance(value, str) or len(value) > 4000:
                raise ValueError("text must be no longer than 4000 characters")
            page.locator(selector).first.fill(value)
        elif tool not in ("browser.observe", "browser.screenshot"):
            raise ValueError("unknown browser tool")
        observation = browser_observation(page, session["id"], artifact_id)
    return json.dumps(observation, ensure_ascii=False)


def authenticate_space() -> str:
    code, result = json_request(
        SPACE_API,
        "/auth/v1/sessions",
        method="POST",
        body={"email": "root@local", "password": required_env("SPACE_LOCAL_PASSWORD")},
        timeout=15,
    )
    token = result.get("token") if isinstance(result, dict) else None
    if code not in (200, 201) or not isinstance(token, str) or not token:
        raise RuntimeError("SPACE authentication failed")
    return token


def provider() -> SpaceComputerProvider:
    return SpaceComputerProvider(SPACE_API, SPACE_DATA, authenticate_space(), request_timeout=60)


def latest_decision(journal: MissionJournal, run_id: str, event_name: str) -> str | None:
    values = [
        event["metadata"].get("decision_ref")
        for event in journal.events(TENANT, run_id)
        if event["event"] == event_name
    ]
    return values[-1] if values and isinstance(values[-1], str) else None


def broker_for(agent: sqlite3.Row) -> tuple[MissionJournal, MissionBroker]:
    journal = MissionJournal(JOURNAL_PATH)
    permits = AGSPermitSource(
        AGS_ORIGIN,
        required_env("AGS_OPERATOR_KEY"),
        agent["ags_agent_id"],
        request_timeout=5,
    )
    return journal, MissionBroker(
        journal,
        provider(),
        authorize_launch=permits.authorize_launch,
        authorize_command=permits.authorize_command,
    )


def register_ags(name: str, agent_id: str) -> None:
    code, result = json_request(
        AGS_ORIGIN,
        "/agents",
        method="POST",
        body={
            "id": agent_id,
            "name": name,
            "agent_type": "enterprise_computer_agent",
            "framework": "agentkraft-runtime",
            "scope": ["space_sandbox_launch", "space_command_execute"],
            "skill_ids": ["computer.inspect", "computer.files", "pul.search_code"],
            "jurisdiction_ids": ["local-development"],
            "token_budget": 50_000,
            "api_budget": 1_000,
        },
        headers={"X-AGS-KEY": required_env("AGS_OPERATOR_KEY")},
        timeout=10,
    )
    if code not in (200, 201, 409):
        raise RuntimeError(f"AGS agent registration failed: HTTP {code}: {result}")


def register_pul(name: str, agent_id: str) -> str:
    admin = Path(required_env("PUL_ADMIN_TOKEN_FILE")).read_text(encoding="utf-8").strip()
    headers = {"X-PUL-Admin-Token": admin}
    code, result = json_request(PUL_ORIGIN, "/v1/apps", method="POST", body={"name": name}, headers=headers)
    if code not in (200, 201) or not isinstance(result, dict):
        raise RuntimeError(f"PUL app registration failed: HTTP {code}")
    app = result.get("app")
    token = result.get("token")
    if not isinstance(app, dict) or not app.get("id") or not isinstance(token, str) or not token:
        raise RuntimeError("PUL returned an invalid application identity")
    token_path = SECRET_DIR / f"{agent_id}.pul-token"
    token_path.write_text(token + "\n", encoding="utf-8")
    try:
        token_path.chmod(0o600)
    except OSError:
        pass
    app_headers = {**headers, "Authorization": "Bearer " + token}
    code, _ = json_request(
        PUL_ORIGIN,
        f"/v1/apps/{app['id']}/allow/repo.github.search_code",
        method="POST",
        body={},
        headers=app_headers,
    )
    if code not in (200, 201, 204):
        raise RuntimeError(f"PUL policy configuration failed: HTTP {code}")
    return str(app["id"])


def pul_search(agent: sqlite3.Row, query: str, turn_id: str, seq: int) -> tuple[str, str]:
    if not isinstance(query, str) or not query.strip() or len(query) > 300:
        raise ValueError("search query must be 1-300 characters")
    token = (SECRET_DIR / f"{agent['id']}.pul-token").read_text(encoding="utf-8").strip()
    admin = Path(required_env("PUL_ADMIN_TOKEN_FILE")).read_text(encoding="utf-8").strip()
    code, result = json_request(
        PUL_ORIGIN,
        "/v1/execute",
        method="POST",
        body={
            "operation": "repo.github.search_code",
            "input": {"query": query.strip()},
            "caller": {"app": agent["pul_app_id"], "user": agent["id"]},
            "idempotencyKey": f"{turn_id}-{seq}",
        },
        headers={"X-PUL-Admin-Token": admin, "Authorization": "Bearer " + token},
    )
    if code not in (200, 201) or not isinstance(result, dict) or not result.get("receiptId"):
        raise RuntimeError(f"PUL execution failed: HTTP {code}")
    summary = json.dumps({"status": result.get("status"), "dry_run": True}, separators=(",", ":"))
    return str(result["receiptId"]), summary


def safe_relative_path(value: Any) -> str:
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/-]{0,199}", value):
        raise ValueError("path must be a simple relative workspace path")
    if value.startswith("/") or ".." in value.split("/"):
        raise ValueError("path must remain inside the agent workspace")
    return value


def tool_command(tool: str, args: dict[str, Any]) -> list[str]:
    root = "/tmp/agent-workspace"
    if tool == "computer.inspect":
        return ["sh", "-c", f"mkdir -p {root}; printf 'computer=SPACE-Firecracker\\n'; id; uname -a; printf 'workspace={root}\\n'; find {root} -maxdepth 2 -type f -print | sort"]
    if tool == "computer.list_files":
        rel = safe_relative_path(args.get("path", ".")) if args.get("path", ".") != "." else "."
        target = root if rel == "." else f"{root}/{rel}"
        return ["sh", "-c", "mkdir -p \"$1\"; find \"$1\" -maxdepth 3 -type f -print | sort", "agent-list", target]
    if tool == "computer.write_file":
        rel = safe_relative_path(args.get("path"))
        content = args.get("content")
        if not isinstance(content, str) or len(content.encode("utf-8")) > MAX_CONTENT:
            raise ValueError(f"content must be text no larger than {MAX_CONTENT} bytes")
        encoded = base64.b64encode(content.encode("utf-8")).decode("ascii")
        target = f"{root}/{rel}"
        return [
            "sh", "-c",
            "mkdir -p \"$(dirname \"$2\")\"; printf '%s' \"$1\" | base64 -d > \"$2\"; wc -c < \"$2\"",
            "agent-write", encoded, target,
        ]
    if tool == "computer.read_file":
        rel = safe_relative_path(args.get("path"))
        target = f"{root}/{rel}"
        return ["sh", "-c", "test -f \"$1\"; wc -c < \"$1\"; sed -n '1,240p' \"$1\"", "agent-read", target]
    if tool == "computer.run_file":
        rel = safe_relative_path(args.get("path"))
        target = f"{root}/{rel}"
        if rel.endswith(".sh"):
            return ["sh", target]
        if rel.endswith(".py"):
            return ["python3", target]
        raise ValueError("only .sh and .py workspace programs can be run")
    raise ValueError("unknown computer tool")


TOOLS = [
    {"name": "computer.inspect", "args": {}, "purpose": "Inspect this agent's computer and workspace."},
    {"name": "computer.list_files", "args": {"path": "."}, "purpose": "List files below a relative workspace path."},
    {"name": "computer.write_file", "args": {"path": "relative/path.txt", "content": "text"}, "purpose": "Create or replace a workspace file."},
    {"name": "computer.read_file", "args": {"path": "relative/path.txt"}, "purpose": "Read and verify a workspace file."},
    {"name": "computer.run_file", "args": {"path": "relative/script.sh"}, "purpose": "Run a .sh or .py program stored in the workspace."},
    {"name": "browser.open", "args": {"url": "https://example.com"}, "purpose": "Open a credential-free web page in this agent's dedicated browser and capture evidence."},
    {"name": "browser.observe", "args": {}, "purpose": "Read the current page, enumerate visible controls, and capture a screenshot."},
    {"name": "browser.click", "args": {"selector": "text=More information"}, "purpose": "Click one visible control by Playwright selector, then observe and screenshot."},
    {"name": "browser.type", "args": {"selector": "input[name=q]", "text": "search"}, "purpose": "Fill a visible field, then observe and screenshot."},
    {"name": "browser.screenshot", "args": {}, "purpose": "Capture the current browser viewport as durable evidence."},
    {"name": "pul.search_code", "args": {"query": "search terms"}, "purpose": "Use the governed PUL GitHub search connector; currently dry-run without provider credentials."},
]


def model_plan(agent: sqlite3.Row, messages: list[dict[str, str]], steps: list[dict[str, Any]]) -> dict[str, Any]:
    system = (
        "You are a persistent enterprise computer agent. You have your own isolated Linux computer. "
        "Work on the user's task over multiple steps. Return exactly one JSON object and nothing else. "
        "To use a tool return {\"kind\":\"tool\",\"tool\":\"tool.name\",\"args\":{},\"rationale\":\"why\"}. "
        "When the task is complete return {\"kind\":\"respond\",\"message\":\"concise evidence-backed answer\",\"rationale\":\"why complete\"}. "
        "Use only listed tools. Inspect before acting. Use browser.open for web pages and browser.observe before clicking. "
        "After writing a file, read it to verify it before responding. "
        "Do not repeat a successful action. Do not claim a result until its tool output proves it. "
        "PUL search is a safe dry-run until a provider credential is explicitly installed."
    )
    prompt = {
        "agent": {"name": agent["name"], "instructions": agent["instructions"]},
        "tools": TOOLS,
        "conversation": messages[-12:],
        "completed_steps": steps[-12:],
    }
    code, result = json_request(
        OLLAMA_ORIGIN,
        "/api/generate",
        method="POST",
        body={
            "model": agent["model"],
            "stream": False,
            "format": "json",
            "options": {"temperature": 0},
            "prompt": system + "\n\nSTATE:\n" + json.dumps(prompt, ensure_ascii=False),
        },
        timeout=120,
    )
    if code != 200 or not isinstance(result, dict) or result.get("done") is not True:
        raise RuntimeError(f"Ollama planning failed: HTTP {code}")
    try:
        plan = json.loads(result["response"])
    except (KeyError, TypeError, json.JSONDecodeError) as error:
        raise RuntimeError("Ollama returned invalid planning JSON") from error
    if isinstance(plan, dict) and isinstance(plan.get("action"), dict):
        plan = plan["action"]
    if not isinstance(plan, dict) or plan.get("kind") not in ("tool", "respond"):
        raise RuntimeError("Ollama returned an unsupported plan")
    return plan


def unmet_task_requirements(user_message: str, history: list[dict[str, Any]]) -> list[str]:
    """Derive small, objective completion gates from the user's requested computer work.

    This is not a second planner. It prevents a model from claiming that an observable
    operation happened when no corresponding successful tool evidence exists.
    """
    lower = user_message.lower()
    completed = [item for item in history if item.get("status") == "completed"]
    tools = [item.get("tool") for item in completed]
    unmet: list[str] = []
    if "inspect" in lower and "computer.inspect" not in tools:
        unmet.append("computer inspection has not completed")
    paths = {
        match
        for match in re.findall(r"\b[A-Za-z0-9][A-Za-z0-9._/-]{0,120}\.(?:txt|md|json|sh|py)\b", user_message)
        if ".." not in match.split("/")
    }
    for path in sorted(paths):
        writes = [item for item in completed if item.get("tool") == "computer.write_file" and item.get("path") == path]
        reads = [item for item in completed if item.get("tool") == "computer.read_file" and item.get("path") == path]
        if any(word in lower for word in ("create", "write", "save")) and not writes:
            unmet.append(f"{path} has no successful write evidence")
        if any(word in lower for word in ("read", "verify", "check")) and not reads:
            unmet.append(f"{path} has no successful read-back evidence")
    if "pul" in lower and not any(tool == "pul.search_code" for tool in tools):
        unmet.append("the requested PUL action has no receipt")
    if any(word in lower for word in ("browse", "browser", "navigate", "open website", "open page")):
        if not any(str(tool).startswith("browser.") for tool in tools):
            unmet.append("the requested browser action has no screenshot evidence")
    return unmet


def evidence_repair_plan(user_message: str, unmet: list[str]) -> dict[str, Any]:
    """Select the smallest safe action that can satisfy the next objective evidence gap."""
    paths = re.findall(
        r"\b[A-Za-z0-9][A-Za-z0-9._/-]{0,120}\.(?:txt|md|json|sh|py)\b",
        user_message,
    )
    path = paths[0] if paths else None
    first = unmet[0] if unmet else ""
    if "inspection" in first:
        return {
            "kind": "tool", "tool": "computer.inspect", "args": {},
            "rationale": "evidence gate requires a real computer inspection before completion",
        }
    if "write evidence" in first and path:
        match = re.search(
            r"containing\s+(?:exactly\s+)?(?:[A-Za-z0-9]+\s+lines?\s*:\s*)?(.+?)(?:\.\s+(?:Read|Then|Verify|After)\b|$)",
            user_message,
            flags=re.IGNORECASE | re.DOTALL,
        )
        requested = match.group(1).strip() if match else user_message.strip()
        if ";" in requested and "lines" in user_message.lower():
            requested = "\n".join(part.strip().rstrip(".") for part in requested.split(";") if part.strip()) + "\n"
        return {
            "kind": "tool", "tool": "computer.write_file",
            "args": {"path": path, "content": requested},
            "rationale": "evidence gate requires the requested workspace artifact",
        }
    if "read-back evidence" in first and path:
        return {
            "kind": "tool", "tool": "computer.read_file", "args": {"path": path},
            "rationale": "evidence gate requires read-back verification before completion",
        }
    if "PUL action" in first:
        return {
            "kind": "tool", "tool": "pul.search_code",
            "args": {"query": "AgentFabric enterprise agent"},
            "rationale": "evidence gate requires a governed PUL receipt",
        }
    if "browser action" in first:
        match = re.search(r"https?://[^\s<>\"]+", user_message)
        if match:
            return {
                "kind": "tool", "tool": "browser.open",
                "args": {"url": match.group(0).rstrip(".,)")},
                "rationale": "evidence gate requires a real browser observation",
            }
        return {
            "kind": "tool", "tool": "browser.observe", "args": {},
            "rationale": "evidence gate requires a real browser observation",
        }
    raise RuntimeError("the task evidence contract could not derive a safe next action")


def bearer_token(secret: str) -> str:
    def segment(value: dict) -> str:
        raw = json.dumps(value, separators=(",", ":"), sort_keys=True).encode()
        return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()
    now = int(time.time())
    signing = segment({"alg": "HS256", "typ": "JWT"}) + "." + segment(
        {"sub": "enterprise-agent-runtime", "iat": now, "exp": now + 3600}
    )
    sig = hmac.new(secret.encode(), signing.encode(), hashlib.sha256).digest()
    return signing + "." + base64.urlsafe_b64encode(sig).rstrip(b"=").decode()


def telemetry() -> TracerProvider:
    provider_ = TracerProvider(resource=Resource.create({
        "service.name": "agentkraft-enterprise-agent",
        "deployment.environment": "local",
    }))
    provider_.add_span_processor(SimpleSpanProcessor(OTLPSpanExporter(
        endpoint="127.0.0.1:14317",
        insecure=True,
        headers=(("authorization", "Bearer " + bearer_token(required_env("GOVAGN_JWT_SECRET"))),),
    )))
    return provider_


def row_or_404(table: str, item_id: str) -> sqlite3.Row:
    if table not in ("agents", "sessions", "turns"):
        raise RuntimeError("invalid table")
    row = DB.execute(f"SELECT * FROM {table} WHERE id=?", (item_id,)).fetchone()
    if not row:
        raise HTTPException(404, f"{table[:-1]} not found")
    return row


def agent_payload(row: sqlite3.Row) -> dict[str, Any]:
    active = DB.execute(
        "SELECT id,sandbox_id,state,created_at,last_activity_at,browser_container,browser_port,browser_state FROM sessions WHERE agent_id=? AND state='active' ORDER BY created_at DESC LIMIT 1",
        (row["id"],),
    ).fetchone()
    return {
        "id": row["id"], "name": row["name"], "instructions": row["instructions"],
        "model": row["model"], "ags_agent_id": row["ags_agent_id"],
        "pul_app_id": row["pul_app_id"], "created_at": row["created_at"],
        "active_session": dict(active) if active else None,
    }


class AgentIn(BaseModel):
    name: str = Field(min_length=1, max_length=80)
    instructions: str = Field(min_length=1, max_length=4000)
    model: str = Field(default=MODEL, min_length=1, max_length=120)


class MessageIn(BaseModel):
    message: str = Field(min_length=1, max_length=12_000)


app = FastAPI(title="AgentKraft Enterprise Agents", version="0.1.0")


@app.get("/health")
def health():
    checks = {}
    for name, origin, path in (
        ("ags", AGS_ORIGIN, "/health"),
        ("pul", PUL_ORIGIN, "/health"),
        ("space", SPACE_DATA, "/health"),
        ("ollama", OLLAMA_ORIGIN, "/api/tags"),
        ("govagn", GOVAGN_ORIGIN, "/healthz"),
    ):
        try:
            code, _ = json_request(origin, path, timeout=5)
            checks[name] = code in (200, 204)
        except Exception:
            checks[name] = False
    return {"status": "ok" if all(checks.values()) else "degraded", "checks": checks}


@app.get("/api/agents")
def list_agents():
    with DB_LOCK:
        return {"agents": [agent_payload(row) for row in DB.execute("SELECT * FROM agents ORDER BY created_at DESC").fetchall()]}


@app.post("/api/agents", status_code=201)
def create_agent(body: AgentIn):
    agent_id = "agent_" + uuid.uuid4().hex[:12]
    ags_id = "AGT-ENT-" + uuid.uuid4().hex[:8].upper()
    try:
        register_ags(body.name, ags_id)
        pul_id = register_pul(body.name, agent_id)
    except Exception as error:
        raise HTTPException(503, str(error)) from error
    now = utcnow()
    with DB_LOCK, DB:
        DB.execute(
            "INSERT INTO agents VALUES (?,?,?,?,?,?,?)",
            (agent_id, body.name.strip(), body.instructions.strip(), body.model, ags_id, pul_id, now),
        )
        return agent_payload(row_or_404("agents", agent_id))


@app.post("/api/agents/{agent_id}/sessions", status_code=201)
def start_session(agent_id: str):
    with DB_LOCK:
        agent = row_or_404("agents", agent_id)
        active = DB.execute(
            "SELECT * FROM sessions WHERE agent_id=? AND state='active' ORDER BY created_at DESC LIMIT 1",
            (agent_id,),
        ).fetchone()
        if active:
            return dict(active)
    session_id = "ses_" + uuid.uuid4().hex[:12]
    run_id = "enterprise-agent-" + uuid.uuid4().hex[:12]
    journal, broker = broker_for(agent)
    try:
        journal.begin(TENANT, run_id, f"Persistent computer for enterprise agent {agent_id}")
        mission = broker.launch(TENANT, run_id, template="seed:latest", request_key=session_id + "-launch")
        sandbox_id = mission.sandbox_id
        operation_id = mission.operation_id
    except Exception as error:
        raise HTTPException(503, f"Could not start SPACE computer: {error}") from error
    finally:
        journal.close()
    now = utcnow()
    with DB_LOCK, DB:
        DB.execute(
            "INSERT INTO sessions(id,agent_id,run_id,sandbox_id,create_operation_id,state,created_at,last_activity_at) VALUES (?,?,?,?,?,?,?,?)",
            (session_id, agent_id, run_id, sandbox_id, operation_id, "active", now, now),
        )
    return dict(row_or_404("sessions", session_id))


@app.get("/api/sessions/{session_id}")
def get_session(session_id: str):
    with DB_LOCK:
        session = row_or_404("sessions", session_id)
        messages = [dict(row) for row in DB.execute(
            "SELECT role,content,created_at FROM messages WHERE session_id=? ORDER BY id", (session_id,)
        ).fetchall()]
        turns = [dict(row) for row in DB.execute(
            "SELECT * FROM turns WHERE session_id=? ORDER BY created_at DESC LIMIT 20", (session_id,)
        ).fetchall()]
        steps = [dict(row) for row in DB.execute(
            "SELECT s.* FROM steps s JOIN turns t ON t.id=s.turn_id WHERE t.session_id=? ORDER BY s.id DESC LIMIT 100",
            (session_id,),
        ).fetchall()]
    return {"session": dict(session), "browser": browser_status(session),
            "messages": messages, "turns": turns, "steps": list(reversed(steps))}


@app.delete("/api/sessions/{session_id}")
def stop_session(session_id: str):
    with DB_LOCK:
        session = row_or_404("sessions", session_id)
        if session["state"] != "active":
            return {"id": session_id, "state": session["state"]}
        agent = row_or_404("agents", session["agent_id"])
    journal, broker = broker_for(agent)
    try:
        broker.terminate(TENANT, session["run_id"], request_key=session_id + "-stop")
    except Exception as error:
        raise HTTPException(503, f"Could not stop SPACE computer: {error}") from error
    finally:
        journal.close()
    name = session["browser_container"] or browser_container_name(session_id)
    docker("rm", "-f", name, check=False)
    with DB_LOCK, DB:
        DB.execute("UPDATE sessions SET state='stopped',browser_state='stopped',last_activity_at=? WHERE id=?",
                   (utcnow(), session_id))
    return {"id": session_id, "state": "stopped"}


@app.get("/api/sessions/{session_id}/browser")
def get_browser(session_id: str):
    with DB_LOCK:
        session = row_or_404("sessions", session_id)
    return browser_status(session)


@app.post("/api/sessions/{session_id}/browser", status_code=201)
def start_browser(session_id: str):
    with DB_LOCK:
        session = row_or_404("sessions", session_id)
        if session["state"] != "active":
            raise HTTPException(409, "agent computer is not active")
    try:
        return ensure_browser(session)
    except Exception as error:
        raise HTTPException(503, f"Could not start dedicated browser: {error}") from error


@app.get("/api/artifacts/{session_id}/{filename}")
def get_artifact(session_id: str, filename: str):
    if not re.fullmatch(r"ses_[a-f0-9]{12}", session_id) or not re.fullmatch(r"[A-Za-z0-9_-]+[.]png", filename):
        raise HTTPException(404, "artifact not found")
    path = ARTIFACT_DIR / session_id / filename
    if not path.is_file():
        raise HTTPException(404, "artifact not found")
    return FileResponse(path, media_type="image/png", headers={"Cache-Control": "no-store"})


def record_step(turn_id: str, seq: int, kind: str, *, tool: str | None, rationale: str,
                status: str, output: str | None = None, ags_decision: str | None = None,
                pul_receipt: str | None = None, process_id: str | None = None) -> None:
    with DB_LOCK, DB:
        DB.execute(
            """INSERT INTO steps
               (turn_id,seq,kind,tool,rationale,status,output,ags_decision,pul_receipt,space_process_id,created_at)
               VALUES (?,?,?,?,?,?,?,?,?,?,?)""",
            (turn_id, seq, kind, tool, rationale[:2000], status,
             output[:16000] if output else None, ags_decision, pul_receipt, process_id, utcnow()),
        )


@app.post("/api/sessions/{session_id}/messages", status_code=201)
def run_agent(session_id: str, body: MessageIn):
    lock = SESSION_LOCKS.setdefault(session_id, threading.Lock())
    if not lock.acquire(blocking=False):
        raise HTTPException(409, "this agent is already working")
    try:
        with DB_LOCK:
            session = row_or_404("sessions", session_id)
            if session["state"] != "active":
                raise HTTPException(409, "agent computer is not active")
            agent = row_or_404("agents", session["agent_id"])
            messages = [dict(row) for row in DB.execute(
                "SELECT role,content FROM messages WHERE session_id=? ORDER BY id", (session_id,)
            ).fetchall()]
        turn_id = "turn_" + uuid.uuid4().hex[:12]
        with DB_LOCK, DB:
            DB.execute("INSERT INTO messages(session_id,role,content,created_at) VALUES (?,?,?,?)",
                       (session_id, "user", body.message, utcnow()))
            DB.execute("INSERT INTO turns(id,session_id,state,created_at) VALUES (?,?,?,?)",
                       (turn_id, session_id, "running", utcnow()))
        messages.append({"role": "user", "content": body.message})
        history: list[dict[str, Any]] = []
        journal, broker = broker_for(agent)
        telemetry_provider = telemetry()
        tracer = telemetry_provider.get_tracer("agentkraft.enterprise")
        reply = None
        trace_id = None
        try:
            with tracer.start_as_current_span("enterprise.agent.turn") as root:
                trace_id = format(root.get_span_context().trace_id, "032x")
                root.set_attribute("agent.id", agent["id"])
                root.set_attribute("agent.ags_id", agent["ags_agent_id"])
                root.set_attribute("session.id", session_id)
                root.set_attribute("space.sandbox_id", session["sandbox_id"])
                root.set_attribute("input.sha256", hashlib.sha256(body.message.encode()).hexdigest())
                for seq in range(1, MAX_STEPS + 1):
                    with tracer.start_as_current_span("agent.plan") as span:
                        plan = model_plan(agent, messages, history)
                        kind = plan.get("kind")
                        tool = plan.get("tool") if kind == "tool" else None
                        rationale = str(plan.get("rationale") or "model-selected next step")
                        span.set_attribute("agent.step", seq)
                        span.set_attribute("agent.action", tool or "respond")
                    if kind == "respond":
                        unmet = unmet_task_requirements(body.message, history)
                        if unmet:
                            plan = evidence_repair_plan(body.message, unmet)
                            kind = "tool"
                            tool = plan["tool"]
                            args = plan["args"]
                            rationale = (
                                "Premature model completion was replaced by a required evidence action: "
                                + plan["rationale"]
                            )
                        else:
                            reply = str(plan.get("message") or "Task completed.")[:12000]
                            record_step(turn_id, seq, "respond", tool=None, rationale=rationale,
                                        status="completed", output=reply)
                            with tracer.start_as_current_span("agent.respond") as span:
                                span.set_attribute("agent.step", seq)
                            break
                    else:
                        args = plan.get("args") if isinstance(plan.get("args"), dict) else {}
                    if isinstance(tool, str) and tool.startswith("browser."):
                        with tracer.start_as_current_span("browser.tool") as span:
                            output = run_browser_tool(session, tool, args, f"{turn_id}-{seq}")
                            span.set_attribute("browser.tool", tool)
                            span.set_attribute("browser.execution_plane", "docker-sidecar")
                        record_step(turn_id, seq, "tool", tool=tool, rationale=rationale,
                                    status="completed", output=output)
                        history.append({"step": seq, "tool": tool, "status": "completed", "output": output[-8000:]})
                        continue
                    if tool == "pul.search_code":
                        with tracer.start_as_current_span("pul.tool") as span:
                            receipt, output = pul_search(agent, str(args.get("query") or ""), turn_id, seq)
                            span.set_attribute("pul.receipt_id", receipt)
                            span.set_attribute("pul.external_write", False)
                        record_step(turn_id, seq, "tool", tool=tool, rationale=rationale,
                                    status="completed", output=output, pul_receipt=receipt)
                        history.append({"step": seq, "tool": tool, "status": "completed", "output": output})
                        continue
                    if tool not in {item["name"] for item in TOOLS if item["name"].startswith("computer.")}:
                        output = "Rejected: the model proposed an undeclared tool."
                        record_step(turn_id, seq, "tool", tool=str(tool), rationale=rationale,
                                    status="rejected", output=output)
                        history.append({"step": seq, "tool": tool, "status": "rejected", "output": output})
                        continue
                    try:
                        argv = tool_command(str(tool), args)
                    except ValueError as error:
                        output = "Rejected: " + str(error)
                        record_step(turn_id, seq, "tool", tool=str(tool), rationale=rationale,
                                    status="rejected", output=output)
                        history.append({"step": seq, "tool": tool, "status": "rejected", "output": output})
                        continue
                    fingerprint = hashlib.sha256(json.dumps(argv, separators=(",", ":")).encode()).hexdigest()
                    if history and history[-1].get("fingerprint") == fingerprint:
                        output = "Rejected: duplicate consecutive action. Choose the next step or respond."
                        record_step(turn_id, seq, "tool", tool=str(tool), rationale=rationale,
                                    status="rejected", output=output)
                        history.append({"step": seq, "tool": tool, "status": "rejected", "output": output})
                        continue
                    with tracer.start_as_current_span("ags.space.command") as span:
                        result = broker.execute(
                            TENANT,
                            session["run_id"],
                            argv=argv,
                            request_key=f"{turn_id}-{seq}",
                        )
                        decision = latest_decision(journal, session["run_id"], "command.completed")
                        output = result.stdout.decode("utf-8", "replace")
                        span.set_attribute("ags.decision_ref", decision or "")
                        span.set_attribute("space.process_id", result.process_id)
                        span.set_attribute("space.exit_code", result.exit_code)
                    record_step(turn_id, seq, "tool", tool=str(tool), rationale=rationale,
                                status="completed", output=output, ags_decision=decision,
                                process_id=result.process_id)
                    history.append({
                        "step": seq, "tool": tool, "status": "completed",
                        "output": output[-8000:], "fingerprint": fingerprint,
                        "path": args.get("path") if isinstance(args.get("path"), str) else None,
                    })
                if reply is None:
                    reply = "I reached the bounded step limit. Review the completed steps before continuing."
                    record_step(turn_id, MAX_STEPS + 1, "respond", tool=None,
                                rationale="bounded execution limit reached", status="limited", output=reply)
            telemetry_provider.force_flush(timeout_millis=10_000)
            with DB_LOCK, DB:
                DB.execute("INSERT INTO messages(session_id,role,content,created_at) VALUES (?,?,?,?)",
                           (session_id, "assistant", reply, utcnow()))
                DB.execute("UPDATE turns SET state='completed',trace_id=?,reply=?,completed_at=? WHERE id=?",
                           (trace_id, reply, utcnow(), turn_id))
                DB.execute("UPDATE sessions SET last_activity_at=? WHERE id=?", (utcnow(), session_id))
        except Exception as error:
            with DB_LOCK, DB:
                DB.execute("UPDATE turns SET state='failed',trace_id=?,reply=?,completed_at=? WHERE id=?",
                           (trace_id, str(error)[:2000], utcnow(), turn_id))
            raise HTTPException(503, f"agent turn failed: {error}") from error
        finally:
            telemetry_provider.shutdown()
            journal.close()
        return get_session(session_id)
    finally:
        lock.release()


@app.get("/", response_class=HTMLResponse)
def ui():
    return HTMLResponse(UI)


UI = r"""<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>AgentKraft Enterprise Agents</title>
<style>
:root{color-scheme:dark;--bg:#07111f;--card:#0d1b2a;--line:#233952;--text:#e8f0f8;--muted:#8fa9c2;--accent:#65d3ff;--ok:#55dfa5;--warn:#ffca6b}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 10% 0,#123152 0,transparent 34%),var(--bg);color:var(--text);font:14px/1.45 Inter,system-ui,sans-serif}
header{padding:24px 30px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;align-items:center}h1{font-size:22px;margin:0}small,.muted{color:var(--muted)}main{display:grid;grid-template-columns:320px 1fr;min-height:calc(100vh - 82px)}aside{border-right:1px solid var(--line);padding:20px}.workspace{padding:22px;max-width:1100px}.card{background:rgba(13,27,42,.92);border:1px solid var(--line);border-radius:14px;padding:16px;margin-bottom:16px}.agent{cursor:pointer}.agent:hover,.agent.active{border-color:var(--accent)}button{background:var(--accent);color:#032235;border:0;border-radius:8px;padding:9px 13px;font-weight:750;cursor:pointer}button.secondary{background:#17304a;color:var(--text)}input,textarea{width:100%;background:#081522;border:1px solid var(--line);color:var(--text);border-radius:8px;padding:10px;margin:6px 0 10px}textarea{min-height:86px;resize:vertical}.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.pill{font-size:12px;padding:4px 8px;border-radius:999px;background:#17304a;color:var(--accent)}.ok{color:var(--ok)}.warn{color:var(--warn)}.messages{max-height:360px;overflow:auto}.message{padding:10px 12px;margin:8px 0;border-left:3px solid var(--line);white-space:pre-wrap}.message.user{border-color:var(--accent)}.message.assistant{border-color:var(--ok)}.step{padding:10px;border-top:1px solid var(--line)}pre{white-space:pre-wrap;word-break:break-word;color:#b8cbe0;margin:7px 0}.hidden{display:none}@media(max-width:780px){main{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid var(--line)}}
</style></head><body>
<header><div><h1>AgentKraft Enterprise Agents</h1><small>Persistent agents with governed SPACE computers and dedicated browsers</small></div><span id="health" class="pill">checking stack…</span></header>
<main><aside><button onclick="showCreate()">+ Create agent</button><div id="agents"></div></aside>
<section class="workspace">
<div id="empty" class="card"><h2>Create or choose an agent</h2><p class="muted">Each agent gets an AGS identity, PUL application identity, persistent conversation and dedicated SPACE Firecracker computer.</p></div>
<div id="create" class="card hidden"><h2>New enterprise agent</h2><input id="name" value="Enterprise Workspace Agent" placeholder="Agent name"><textarea id="instructions">Work carefully on enterprise tasks using your persistent computer. Inspect first, preserve evidence, verify every file you create, and report only observed results.</textarea><button onclick="createAgent()">Create agent identity</button></div>
<div id="detail" class="hidden"><div class="card"><div class="row"><h2 id="agentName" style="margin-right:auto"></h2><span id="model" class="pill"></span></div><p id="instructions" class="muted"></p><div id="identities"></div><div class="row" style="margin-top:12px"><button id="start" onclick="startComputer()">Start dedicated computer</button><span id="computer"></span></div></div>
<div id="chat" class="card hidden"><div class="row"><h3 style="margin-right:auto">Talk to the agent</h3><button class="secondary" onclick="stopComputer()">Stop computer</button></div><div id="messages" class="messages"></div><textarea id="task" placeholder="Example: Inspect your computer, create hello-agent.txt explaining who you are, read it back to verify it, then report the result."></textarea><button id="run" onclick="runTask()">Run task</button></div>
<div id="browserCard" class="card hidden"><div class="row"><h3 style="margin-right:auto">Agent browser</h3><span id="browserState" class="pill">stopped</span><button id="startBrowser" class="secondary" onclick="startDedicatedBrowser()">Start browser</button></div><p class="muted">A persistent Chromium profile dedicated to this agent. Browser actions and screenshots appear in the audited activity below.</p><img id="browserShot" class="hidden" alt="Latest agent browser screenshot" style="width:100%;max-height:520px;object-fit:contain;border:1px solid var(--line);border-radius:10px;background:white"></div>
<div id="activity" class="card hidden"><h3>Agent activity</h3><div id="steps"></div></div></div>
</section></main>
<script>
let selected=null,session=null;const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
async function api(url,opt){const r=await fetch(url,{headers:{'Content-Type':'application/json'},...opt});const b=await r.json();if(!r.ok)throw Error(b.detail||r.statusText);return b}
async function refresh(){const h=await api('/health');document.querySelector('#health').textContent=h.status+' · '+Object.entries(h.checks).filter(x=>x[1]).length+'/5 services';document.querySelector('#health').className='pill '+(h.status==='ok'?'ok':'warn');const d=await api('/api/agents');document.querySelector('#agents').innerHTML=d.agents.map(a=>`<div class="card agent ${selected?.id===a.id?'active':''}" onclick="choose('${a.id}')"><b>${esc(a.name)}</b><div class="muted">${esc(a.model)}</div><div class="pill" style="display:inline-block;margin-top:8px">${a.active_session?'computer active':'no computer'}</div></div>`).join('');if(selected){const n=d.agents.find(a=>a.id===selected.id);if(n){selected=n;renderAgent()}}}
function showCreate(){document.querySelector('#empty').classList.add('hidden');document.querySelector('#detail').classList.add('hidden');document.querySelector('#create').classList.remove('hidden')}
async function createAgent(){try{selected=await api('/api/agents',{method:'POST',body:JSON.stringify({name:document.querySelector('#name').value,instructions:document.querySelector('#instructions').value})});document.querySelector('#create').classList.add('hidden');renderAgent();await refresh()}catch(e){alert(e.message)}}
async function choose(id){const d=await api('/api/agents');selected=d.agents.find(a=>a.id===id);session=selected.active_session;renderAgent();if(session)await loadSession();await refresh()}
function renderAgent(){document.querySelector('#empty').classList.add('hidden');document.querySelector('#create').classList.add('hidden');document.querySelector('#detail').classList.remove('hidden');document.querySelector('#agentName').textContent=selected.name;document.querySelector('#model').textContent=selected.model;document.querySelector('#instructions').textContent=selected.instructions;document.querySelector('#identities').innerHTML=`<span class="pill">${esc(selected.id)}</span> <span class="pill">AGS ${esc(selected.ags_agent_id)}</span> <span class="pill">PUL ${esc(selected.pul_app_id)}</span>`;session=selected.active_session;document.querySelector('#computer').innerHTML=session?`<span class="pill ok">SPACE ${esc(session.sandbox_id)}</span>`:'<span class="muted">Not started</span>';document.querySelector('#start').classList.toggle('hidden',!!session);document.querySelector('#chat').classList.toggle('hidden',!session);document.querySelector('#browserCard').classList.toggle('hidden',!session);document.querySelector('#activity').classList.toggle('hidden',!session)}
async function startComputer(){try{session=await api(`/api/agents/${selected.id}/sessions`,{method:'POST',body:'{}'});selected.active_session=session;renderAgent();await loadSession();await refresh()}catch(e){alert(e.message)}}
async function stopComputer(){if(!session||!confirm('Stop this agent computer? Its sandbox files will be removed.'))return;try{await api(`/api/sessions/${session.id}`,{method:'DELETE'});session=null;selected.active_session=null;renderAgent();await refresh()}catch(e){alert(e.message)}}
async function loadSession(){if(!session)return;const d=await api(`/api/sessions/${session.id}`);document.querySelector('#messages').innerHTML=d.messages.map(m=>`<div class="message ${esc(m.role)}"><b>${esc(m.role)}</b><br>${esc(m.content)}</div>`).join('');document.querySelector('#messages').scrollTop=999999;document.querySelector('#browserState').textContent=d.browser.state+' · '+d.browser.execution_plane;document.querySelector('#browserState').className='pill '+(d.browser.state==='running'?'ok':'warn');document.querySelector('#startBrowser').classList.toggle('hidden',d.browser.state==='running');let latest=null;for(const s of d.steps){if(s.tool?.startsWith('browser.')&&s.output){try{const o=JSON.parse(s.output);if(o.screenshot)latest=o.screenshot}catch{}}}const shot=document.querySelector('#browserShot');if(latest){shot.src=latest+'?t='+Date.now();shot.classList.remove('hidden')}document.querySelector('#steps').innerHTML=d.steps.map(s=>`<div class="step"><div class="row"><b>#${s.seq} ${esc(s.tool||s.kind)}</b><span class="pill ${s.status==='completed'?'ok':'warn'}">${esc(s.status)}</span>${s.ags_decision?`<span class="pill">${esc(s.ags_decision)}</span>`:''}${s.pul_receipt?`<span class="pill">${esc(s.pul_receipt)}</span>`:''}</div><div class="muted">${esc(s.rationale)}</div>${s.output?`<pre>${esc(s.output)}</pre>`:''}</div>`).join('')}
async function startDedicatedBrowser(){const btn=document.querySelector('#startBrowser');btn.disabled=true;btn.textContent='Starting…';try{await api(`/api/sessions/${session.id}/browser`,{method:'POST',body:'{}'});await loadSession()}catch(e){alert(e.message)}finally{btn.disabled=false;btn.textContent='Start browser'}}
async function runTask(){const box=document.querySelector('#task'),btn=document.querySelector('#run');if(!box.value.trim())return;btn.disabled=true;btn.textContent='Agent working…';try{await api(`/api/sessions/${session.id}/messages`,{method:'POST',body:JSON.stringify({message:box.value})});box.value='';await loadSession()}catch(e){alert(e.message);await loadSession()}finally{btn.disabled=false;btn.textContent='Run task';await refresh()}}
refresh();setInterval(()=>{if(session)loadSession();},5000);
</script></body></html>"""
