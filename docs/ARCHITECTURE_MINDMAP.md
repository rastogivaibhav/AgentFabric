# AgentFabric Architecture — Comprehensive Mind Map & Analysis

**Last Updated:** 2026-04-27  
**Current Branch:** simplify-architecture  
**Phase Completion:** Phase 3 (Webhook Handler) ✅ | Phase 4 (Dashboards) ⚠️ Partial

---

## MIND MAP: System Architecture

```
AgentFabric (Govagn)
├── INBOUND TELEMETRY LAYER
│   ├── Agent SDK (Python)
│   │   ├── Framework patches (5x)
│   │   │   ├── CrewAI: patch Task.execute()
│   │   │   ├── LangGraph: patch GraphNode.invoke()
│   │   │   ├── OpenAI Agents: patch client.beta.threads
│   │   │   ├── Anthropic: patch messages.create()
│   │   │   └── Google ADK: patch Agent.__call__()
│   │   ├── Auto-instrument via sitecustomize.py
│   │   └── OTLPSpanExporter → Collector :4317 (gRPC)
│   │
│   ├── Collector Service (Go) [entrypoint: cmd/collector/main.go]
│   │   ├── RECEIVERS (3x, currently 2 stub)
│   │   │   ├── OTLP gRPC :4317
│   │   │   │   ├── GRPCRateLimiter (token-bucket on SpansPerSecond)
│   │   │   │   └── GRPCTokenValidator (JWT header)
│   │   │   ├── OTLP HTTP :4318/v1/traces
│   │   │   │   ├── /api/v1/telemetry/vscode [STUB: no-op return 202]
│   │   │   │   ├── /api/v1/telemetry/webhook [STUB: no-op return 202]
│   │   │   │   └── /api/v1/telemetry/events [STUB: no-op return 202]
│   │   │   └── Process discovery (k8s/proc) [STUB: no-op scan]
│   │   │
│   │   ├── AGENT PROCESSOR [async queue + workers]
│   │   │   ├── Queue buffer: 100,000 slots (tail-drop backpressure)
│   │   │   ├── Workers: read from queue → processResourceSpans
│   │   │   │   ├── enrichSpan()
│   │   │   │   │   ├── Framework detection (14 SDK checks → model heuristic)
│   │   │   │   │   ├── PII scrubbing (7 regex patterns)
│   │   │   │   │   ├── Attribute truncation (4096 char limit)
│   │   │   │   │   ├── Token forwarding (input/output/cache/reasoning)
│   │   │   │   │   ├── Server-side attrs injection
│   │   │   │   │   │   ├── af.agent.framework (detected)
│   │   │   │   │   │   ├── af.policy.trusted = "false" [ALWAYS]
│   │   │   │   │   │   ├── af.span.step_type (from attributes)
│   │   │   │   │   │   ├── af.error.class (if error)
│   │   │   │   │   │   └── prompt/response previews (220 char)
│   │   │   │   │   └── Security strip: remove ai.policy.* attrs
│   │   │   │   └── Batching: 32 spans/batch, 1 sec timeout
│   │   │   └── Metrics at /metrics (Prometheus)
│   │   │
│   │   ├── HTTP EXPORTER
│   │   │   ├── Serialize batch as JSON
│   │   │   ├── POST to api-gateway:/internal/ingest
│   │   │   │   ├── Authorization: Bearer GV_GATEWAY_AUTH_TOKEN
│   │   │   │   └── X-AF-Source: collector
│   │   │   └── Retry: 3 attempts (0ms, 250ms, 750ms exponential backoff)
│   │   │
│   │   └── CONFIGURATION (Viper + env)
│   │       ├── GV_OTLP_GRPC_PORT (default 4317)
│   │       ├── GV_OTLP_HTTP_PORT (default 4318)
│   │       ├── GV_GATEWAY_URL
│   │       ├── GV_GATEWAY_AUTH_TOKEN
│   │       └── /etc/govagn/collector.yaml or ./collector.yaml
│
├── CORE PROCESSING LAYER
│   ├── API Gateway (Go) [entrypoint: cmd/server/main.go]
│   │   ├── ROUTER (Chi v5)
│   │   │   ├── Auth routes: /auth/*, /login, /logout, /oidc/callback
│   │   │   ├── API v1 routes: /api/v1/*
│   │   │   ├── Internal routes: /internal/ingest (collector auth only)
│   │   │   ├── Proxy routes: /proxy/{provider}/v1/* (LLM reverse proxy)
│   │   │   ├── NetProxy: :8443 HTTPS MITM
│   │   │   ├── WebSocket: /api/v1/stream/live
│   │   │   └── Admin: /admin/* (RBAC-gated)
│   │   │
│   │   ├── MIDDLEWARE STACK
│   │   │   ├── 1. RequestID (generate UUID per request)
│   │   │   ├── 2. RealIP (extract from X-Forwarded-For)
│   │   │   ├── 3. Recoverer (panic recovery)
│   │   │   ├── 4. Timeout (30s default)
│   │   │   ├── 5. SecurityHeaders (CSP, X-Frame-Options, etc.)
│   │   │   ├── 6. CORS (configurable allowed origins)
│   │   │   ├── 7. Prometheus (request/response metrics)
│   │   │   ├── 8. JWTAuth (extract token from cookie or Authorization header)
│   │   │   │   └── Multi-secret rotation support
│   │   │   ├── 9. TenantInjector (set tenant ID in context)
│   │   │   └── 10. RateLimit (Redis sliding-window per tenant)
│   │   │
│   │   ├── AUTHENTICATION & AUTHORIZATION
│   │   │   ├── Password login (bcrypt hashing)
│   │   │   ├── OIDC SSO (optional, PKCE flow)
│   │   │   ├── JWT multi-secret rotation
│   │   │   ├── HttpOnly cookie 'af_token' + secure flag
│   │   │   └── RBAC: admin | operator | viewer (middleware enforcement)
│   │   │
│   │   ├── HANDLER: Ingest (/internal/ingest)
│   │   │   ├── Validate body ≤ 32 MiB, batch ≤ 10,000 spans
│   │   │   ├── Read X-AF-Tenant header
│   │   │   ├── repriceSpan() — apply pricing rules from DB
│   │   │   ├── BudgetEnforcer.CheckAndRecord()
│   │   │   │   └── If exceeded → create decision_record (type: budget) + return 429
│   │   │   ├── BulkInsertSpans() → PostgreSQL COPY FROM
│   │   │   ├── upsertRunsFromSpans() → project into runs table
│   │   │   └── Broadcast each span to WebSocket Hub (LiveEvent)
│   │   │
│   │   ├── HANDLER: LLM Proxy (/proxy/{provider}/v1/*)
│   │   │   ├── Extract virtual key (af-vk-*) from Authorization or x-api-key
│   │   │   ├── Vault.Resolve() — AES-256-GCM decrypt → real key + tenantID
│   │   │   ├── Buffer request body (≤ 8 MiB)
│   │   │   ├── ParseRequest() — provider-specific parser (OpenAI/Anthropic/Gemini/VertexAI/Bedrock/Azure)
│   │   │   │
│   │   │   ├── POLICY EVALUATION (2-pass)
│   │   │   │   ├── Pass 1: Traffic rules (provider/model/env/token matching)
│   │   │   │   │   └── On DENY → record decision + return 403
│   │   │   │   └── Pass 2: DLP scanning (secrets + PII + compliance)
│   │   │   │       └── On DENY → record decision + return 403
│   │   │   │       └── On REDACT → strip from body before forward
│   │   │   │
│   │   │   ├── BUDGET PRE-CHECK
│   │   │   │   ├── ComputeEstimatedCostForTenant()
│   │   │   │   └── If over limit → 429 (hard) or webhook alert (soft)
│   │   │   │
│   │   │   ├── ROLLOUT ROUTING
│   │   │   │   └── ProviderRouter checks rollout rules (A/B, canary)
│   │   │   │
│   │   │   ├── UPSTREAM FORWARDING
│   │   │   │   ├── Modify Authorization header with real key
│   │   │   │   └── Stream response back verbatim
│   │   │   │
│   │   │   └── RESPONSE PROCESSING
│   │   │       ├── ParseUsage() — extract actual token counts
│   │   │       ├── Re-price with real counts
│   │   │       ├── Create span record in DB
│   │   │       └── Broadcast to WebSocket hub
│   │   │
│   │   ├── HANDLER: NetProxy (:8443 HTTPS MITM)
│   │   │   ├── Intercept HTTPS CONNECT for api.openai.com / api.anthropic.com
│   │   │   ├── TLS MITM with ephemeral/persisted CA
│   │   │   ├── Extract af-vk-* from Authorization
│   │   │   ├── Apply policy + budget checks
│   │   │   ├── Record span
│   │   │   ├── Tunnel non-LLM hosts blindly
│   │   │   └── CA cert served at GET /api/v1/netproxy/ca.crt
│   │   │
│   │   ├── POLICY ENGINE
│   │   │   ├── In-memory compiled rules (loaded from DB at startup)
│   │   │   ├── Traffic rules: pattern match on provider/model/environment/token limit
│   │   │   ├── DLP rules: regex scan request body for secrets/PII
│   │   │   │   ├── Secrets: OpenAI key, Anthropic key, GitHub token, AWS key, etc.
│   │   │   │   └── PII: email, SSN, phone (from collector PII patterns)
│   │   │   └── Actions: allow | warn (log) | redact (strip from body) | deny (403)
│   │   │
│   │   ├── BUDGET ENFORCER
│   │   │   ├── Read from budgets table (per tenant)
│   │   │   ├── Track monthly_cost_usd usage
│   │   │   ├── Fire webhook alerts at threshold
│   │   │   ├── Hard-block (429) if hard_limit = true and exceeded
│   │   │   └── Create decision_record on enforcement action
│   │   │
│   │   ├── VAULT (AES-256-GCM Virtual Key Management)
│   │   │   ├── Master key from GV_VAULT_KEY (64-char hex)
│   │   │   ├── Each virtual key encrypts one real key + tenantID
│   │   │   ├── Nonce: 12 bytes random prepended to ciphertext
│   │   │   └── Resolve(vk) → {realKey, tenantID}
│   │   │
│   │   ├── WEBSOCKET HUB (in-process, single-replica only)
│   │   │   ├── Per-tenant client maps
│   │   │   ├── 4096-slot broadcast channel
│   │   │   ├── Span ingestion → broadcast LiveEvent
│   │   │   ├── LLM proxy responses → broadcast ProxyEvent
│   │   │   ├── Reconnect logic + ping/pong keep-alive
│   │   │   └── [LIMITATION] Does not scale to multiple api-gateway replicas
│   │   │
│   │   ├── EVAL SERVICE
│   │   │   ├── YAML pack definitions (deploy/seed/eval-packs/)
│   │   │   ├── Pack runtime: load pack, score traces against weighted dimensions
│   │   │   ├── Executor: run pack against datasets or live traces
│   │   │   ├── Agent scorecards: compute multi-component health
│   │   │   └── Results stored in eval_executions table
│   │   │
│   │   ├── STORE (PostgreSQL)
│   │   │   ├── BulkInsertSpans() — COPY FROM for throughput
│   │   │   ├── GetCostReport() — GROUP BY vendor/model over time range
│   │   │   ├── GetFrameworkStats() — framework usage breakdown
│   │   │   ├── GetTrace() — load trace + audit entries + compute enrichments
│   │   │   ├── CreatePolicyAuditEntry() — log policy decision [NO HASH CHAIN]
│   │   │   ├── LoadTraceViewInputs() — spans + audit for trace
│   │   │   └── WritePolicyDecision() → decision_records table
│   │   │
│   │   └── ADMIN ENDPOINTS
│   │       ├── Policy management: load/reload policy rules
│   │       ├── Pricing management: create/update pricing rules
│   │       ├── Budget management: set/update tenant budgets
│   │       ├── Audit log: GET /admin/audit (no verification chain)
│   │       ├── Eval pack management
│   │       └── Managed runtime: create/approve/deny tasks
│
├── DATA LAYER
│   ├── PostgreSQL (21 migrations)
│   │   ├── Core tables
│   │   │   ├── tenants: multi-tenancy root (UUID PK)
│   │   │   ├── spans: hot-path span store (trace_id, attributes JSONB, token+cost)
│   │   │   ├── runs: LangSmith-style run tracking (projected from spans)
│   │   │   ├── agent_runs: legacy UUID-keyed run store
│   │   │   ├── users: for password login + OIDC
│   │   │   └── virtual_keys: encrypted real_key + virtual_key mapping
│   │   │
│   │   ├── Governance tables
│   │   │   ├── policy_rules: runtime-loadable traffic + DLP rules
│   │   │   ├── policy_audit_log: [INCOMPLETE] hash-chain schema exists, chain not computed
│   │   │   ├── decision_records: policy/budget/routing decisions with JSON evidence
│   │   │   ├── control_plane_audit: admin action history (no hash chain)
│   │   │   └── budgets: per-tenant monthly caps
│   │   │
│   │   ├── Cost tables
│   │   │   ├── spans.cost_usd, input_cost, output_cost, etc.
│   │   │   ├── pricing_rules: model_pattern → input_per_1m, output_per_1m
│   │   │   └── model_pricing: [DEPRECATED] legacy pricing data
│   │   │
│   │   ├── Eval tables
│   │   │   ├── eval_runs: eval_suite, overall_score, release_tag
│   │   │   ├── eval_executions: pack_id, mode, overall_score, policy_effectiveness JSONB
│   │   │   ├── eval_datasets: golden datasets
│   │   │   └── eval_dataset_items: individual test cases
│   │   │
│   │   ├── Managed Runtime tables
│   │   │   ├── managed_agents: runtime_provider, system_prompt, mcp_servers JSONB
│   │   │   ├── managed_sessions: FK to agents, conversation_turns, event_count
│   │   │   ├── managed_tasks: status, interruption_reason, server_tool_count
│   │   │   └── managed_artifacts: kind, uri, size_bytes
│   │   │
│   │   ├── Config tables
│   │   │   ├── prompt_versions: content_hash for versioning
│   │   │   ├── rollout_rules: A/B test routing (traffic_pct, target_model/provider)
│   │   │   ├── recommendations: AI-generated suggestions (type, status, confidence)
│   │   │   └── trace_saved_views: user filter presets
│   │   │
│   │   └── Indexing strategy
│   │       ├── spans: (trace_id), (tenant_id, ts), (source_vendor)
│   │       ├── runs: (tenant_id, created_at)
│   │       ├── decision_records: (tenant_id, type, created_at)
│   │       └── policy_audit_log: (entry_hash) UNIQUE
│   │
│   ├── Redis (cache + rate-limit)
│   │   ├── Rate-limit counters: sliding-window per tenant per endpoint
│   │   ├── Trace cache: trace:{tenantID}:{traceId} TTL 5m
│   │   ├── Policy rule cache: [IF IMPLEMENTED] policy:{tenantID}:rules
│   │   └── Session store: [IF IMPLEMENTED] for distributed auth
│   │
│   └── [ARCHIVED] ClickHouse
│       ├── govagn.spans: 500k spans/sec write design target (Rust implementation)
│       ├── Time-series partitioning: partition by event date
│       └── [NO GO IMPLEMENTATION] All analytics queries hit PostgreSQL (hot-path issue)
│
├── FRONTEND LAYER
│   ├── Portal (React 18 + TypeScript)
│   │   ├── Entry: src/main.tsx → App.tsx
│   │   ├── Router (React Router v6)
│   │   │   ├── RequireAuth HOC (redirect to /login if unauthenticated)
│   │   │   ├── RequireRole HOC (RBAC gating)
│   │   │   └── 29 routes organized in 7 nav groups
│   │   │
│   │   ├── NAV STRUCTURE (Layout.tsx)
│   │   │   ├── OVERVIEW
│   │   │   │   ├── Dashboard.tsx — high-level metrics
│   │   │   │   └── LiveStream.tsx — real-time span/proxy events
│   │   │   ├── PROTECT
│   │   │   │   ├── PoliciesPage.tsx — view/create traffic + DLP rules
│   │   │   │   ├── DecisionsPage.tsx — historical policy decisions
│   │   │   │   └── PolicySimulationPage.tsx — test rules against sample requests
│   │   │   ├── CONTROL
│   │   │   │   ├── RolloutsPage.tsx — A/B test + canary config
│   │   │   │   └── EnvironmentsPage.tsx — dev/staging/prod settings
│   │   │   ├── SPEND
│   │   │   │   ├── CostPage.tsx — token/cost trends by model/provider
│   │   │   │   ├── PricingRulesPage.tsx — override default pricing
│   │   │   │   └── RecommendationsPage.tsx — cost optimization suggestions
│   │   │   ├── OBSERVE
│   │   │   │   ├── TracesPage.tsx — list traces with filters
│   │   │   │   ├── TraceDetail.tsx — waterfall + timeline + graph tabs
│   │   │   │   ├── RunsPage.tsx — run history
│   │   │   │   ├── AgentsPage.tsx — agent metadata + framework breakdown
│   │   │   │   └── ErrorAnalyticsPage.tsx — error rate trends
│   │   │   ├── SHIP
│   │   │   │   ├── PromptsPage.tsx — prompt versioning + release
│   │   │   │   └── EvalsPage.tsx — eval run results
│   │   │   └── PROVE (admin only)
│   │   │       ├── AuditPage.tsx — admin action log
│   │   │       └── MemoryPage.tsx — long-term context store
│   │   │
│   │   ├── STATE MANAGEMENT
│   │   │   ├── React Query (server state, 15s staleTime)
│   │   │   ├── React Context [could be added, currently not used]
│   │   │   ├── Local state (component useState for UI)
│   │   │   └── WebSocket (useLiveStream hook)
│   │   │
│   │   ├── API CLIENT (hooks/api.ts)
│   │   │   ├── apiFetch() — credentials:'include' for HttpOnly cookie
│   │   │   ├── useQuery wrappers for all GET endpoints
│   │   │   ├── useMutation wrappers for POST/PUT/DELETE
│   │   │   ├── useLiveStream() — WS management + event buffer
│   │   │   └── Automatic retry on transient 5xx
│   │   │
│   │   ├── KEY COMPONENTS
│   │   │   ├── TopologyGraph.tsx — mermaid/d3 render of trace dependencies
│   │   │   ├── SpanTimeline.tsx — waterfall visualization
│   │   │   ├── SpanDetailPanel.tsx — attributes/events/logs inspection
│   │   │   ├── PolicyEventPanel.tsx — policy decision details
│   │   │   ├── DecisionRecordPanel.tsx — governance decision trace
│   │   │   ├── TraceHeader.tsx — metadata (user, session, status)
│   │   │   ├── TraceFilters.tsx — date range, vendor, model filters
│   │   │   ├── CostBreakdownTable.tsx — cost per model/provider
│   │   │   ├── RecommendationFeed.tsx — AI-generated cards
│   │   │   └── [MISSING] Analytics charts (token/cost/latency trends)
│   │   │
│   │   └── AUTHENTICATION
│   │       ├── Login page: username + password (or OIDC button)
│   │       ├── JWT auto-refresh: check token expiry before API calls
│   │       └── Logout: clear HttpOnly cookie
│   │
│   └── [STUB] Analytics pages
│       ├── No dedicated Analytics page (data exists, no chart UI)
│       ├── No dedicated Governance/Risk page
│       └── Cost page exists but no token/latency analytics
│
├── DEPLOYMENT LAYER
│   ├── Docker Compose (dev stack)
│   │   ├── api-gateway: 8080 (API), 8443 (NetProxy HTTPS)
│   │   ├── collector: 4317 (OTLP gRPC), 4318 (OTLP HTTP)
│   │   ├── portal: 3000 (built) → 80 (nginx)
│   │   ├── postgres: 5432 (with init.sql migrations)
│   │   ├── redis: 6379
│   │   ├── [REMOVED] kafka, zookeeper, clickhouse, af-core
│   │   └── prometheus: 9090 (metrics from api-gateway + collector)
│   │
│   ├── Kubernetes (production)
│   │   ├── Helm chart: deploy/helm/
│   │   ├── K8s manifests: deploy/k8s/
│   │   ├── Config: configmaps for env, secrets for keys
│   │   ├── Scaling: api-gateway can scale horizontally [with caveat: WebSocket Hub is single-process]
│   │   └── Schema migrations: run in Job before api-gateway startup
│   │
│   └── Environment Variables (Viper)
│       ├── GV_* prefix (collector + api-gateway)
│       ├── VITE_* prefix (portal React build-time)
│       └── DATABASE_URL, REDIS_URL standard formats
│
└── EXTERNAL INTEGRATIONS
    ├── AGENT SOURCES
    │   ├── Cursor (IDE): /api/v1/telemetry/cursor [STUB]
    │   ├── VSCode extensions (Copilot/Continue/Roo/Cline): /api/v1/telemetry/vscode [STUB]
    │   ├── Cowork (Anthropic): /api/v1/telemetry/cowork [STUB]
    │   ├── Direct API (webhook): /api/v1/telemetry/webhook [STUB]
    │   └── Anthropic Python SDK: OTLPSpanExporter → :4317
    │
    ├── LLM PROVIDERS
    │   ├── OpenAI: /proxy/openai/v1/* or HTTPS :8443 MITM
    │   ├── Anthropic: /proxy/anthropic/v1/* or HTTPS :8443 MITM
    │   ├── Google Gemini: /proxy/google/v1/*
    │   ├── Google Vertex AI: /proxy/vertexai/v1/*
    │   ├── AWS Bedrock: /proxy/bedrock/*
    │   └── Azure OpenAI: /proxy/azure-openai/v1/*
    │
    └── NOTIFICATIONS
        ├── Budget alert webhook (optional)
        └── [MISSING] Task approval notifications (email/Slack)

---

## KEY STATISTICS

- **21 database migrations** (001_initial_schema.up.sql → 0021_eval_execution_core.up.sql)
- **50+ API routes** across auth, api/v1, proxy, admin, internal
- **29 React pages** + 10+ shared components
- **7 telemetry receivers** (OTLP gRPC, OTLP HTTP, 3 webhook stubs, process discovery stub)
- **6 LLM provider parsers** (OpenAI, Anthropic, Gemini, VertexAI, Bedrock, Azure)
- **8 policy rule types** (traffic, DLP, cost, rate limit, approval, etc.)
- **5 framework detections** (CrewAI, LangGraph, OpenAI, Anthropic, Google ADK)
- **7 PII regex patterns** (postcode, card, email, credentials, SSN, name, phone)

---

## CRITICAL GAPS (Summary)

1. **Multi-source receivers are stubs** — VSCode/Webhook/DirectAPI return 202 but drop events
2. **af.policy.trusted never recomputed** — always `"false"` in storage
3. **ClickHouse absent from active stack** — all analytics hit PostgreSQL (hot-path issue)
4. **WebSocket Hub single-process** — does not scale to multiple replicas
5. **Hash-chain audit incomplete** — schema exists, chain not computed
6. **Process discovery is a stub** — no actual /proc or k8s API calls
7. **No Kafka integration** — synchronous HTTP only (no durable buffer)
8. **No Phase 4 dashboards** — Analytics/Governance UI pages missing

---

## PHASE 4 REQUIREMENTS vs. CURRENT STATE

See PHASE_4_COMPARISON.md for detailed analysis.
