# AgentFabric Execution Flow Charts

---

## Flow 1: Inbound Telemetry (Agent SDK → Storage)

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. AGENT SDK INSTRUMENTATION                                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Agent code (CrewAI/LangGraph/OpenAI/etc)                               │
│         ↓                                                                 │
│  govagn.instrument() called at startup                                  │
│         ↓                                                                 │
│  Patch 5 frameworks: CrewAI.Task, LangGraph, OpenAI.beta.threads, etc  │
│         ↓                                                                 │
│  Create OTLPSpanExporter(endpoint="localhost:4317")                     │
│         ↓                                                                 │
│  Monkey-patch framework to emit spans on model calls                    │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 2. COLLECTOR RECEIVER (gRPC :4317 or HTTP :4318)                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  gRPC Server receives ExportSpansRequest                                │
│         ↓                                                                 │
│  GRPCRateLimiter (token-bucket)                                         │
│         ├─ If rate exceeded: return RESOURCE_EXHAUSTED                  │
│         └─ Else: proceed                                                │
│         ↓                                                                 │
│  GRPCTokenValidator (JWT Bearer check)                                  │
│         ├─ If invalid: return UNAUTHENTICATED                           │
│         └─ Else: proceed                                                │
│         ↓                                                                 │
│  Accept ResourceSpans and queue to AgentProcessor buffer                │
│  (100k slot buffer, tail-drop on backpressure)                          │
│         ↓                                                                 │
│  Return OK to caller                                                     │
│                                                                           │
│ [PARALLEL] HTTP :4318 /api/v1/telemetry/*                              │
│  ├─ /vscode [STUB: no-op, return 202]                                  │
│  ├─ /webhook [STUB: no-op, return 202]                                 │
│  ├─ /events  [STUB: no-op, return 202]                                 │
│  └─ Process discovery [STUB: no-op scan]                               │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 3. AGENT PROCESSOR (async workers)                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Worker goroutines dequeue from buffer (32 spans/batch, 1s timeout)     │
│         ↓                                                                 │
│  For each ResourceSpans:                                                 │
│    enrichSpan() per span:                                               │
│         ├─ extractAttributes() → map[string]string                      │
│         ├─ detectFramework()                                            │
│         │   ├─ Check 14 SDK-specific keys (instrumentation.name, etc)  │
│         │   ├─ Check model prefix heuristics (gpt-, claude-, etc)      │
│         │   └─ Return framework enum or "unknown"                      │
│         ├─ scrubPII()                                                   │
│         │   ├─ Apply 7 regex patterns:                                 │
│         │   │   ├─ UK postcode                                          │
│         │   │   ├─ Credit card                                          │
│         │   │   ├─ Email                                                │
│         │   │   ├─ API credentials                                      │
│         │   │   ├─ SSN                                                  │
│         │   │   ├─ Name (heuristic)                                     │
│         │   │   └─ Phone                                                │
│         │   └─ Replace matched values with [REDACTED]                  │
│         ├─ Truncate attributes to 4096 chars                           │
│         ├─ Extract token counts (input/output/cache/reasoning)         │
│         ├─ Strip security: remove ai.policy.* attributes               │
│         └─ Inject server-side attrs:                                   │
│             ├─ af.agent.framework = detected                           │
│             ├─ af.policy.trusted = "false" [ALWAYS]                    │
│             ├─ af.span.step_type = extracted                           │
│             ├─ af.error.class = if error                               │
│             └─ Prompt/response previews (220 char truncation)          │
│         ↓                                                                 │
│  Batch spans into EnrichedSpan objects                                  │
│         ↓                                                                 │
│  Export to API Gateway                                                   │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 4. HTTP EXPORTER (to api-gateway:/internal/ingest)                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Serialize batch to JSON: {"spans": [...]}                              │
│         ↓                                                                 │
│  POST with:                                                              │
│    Authorization: Bearer <GV_GATEWAY_AUTH_TOKEN>                        │
│    X-AF-Source: collector                                               │
│         ↓                                                                 │
│  Attempt 1 (immediate) ─┬─ Success (200/202) → Done                   │
│                         └─ Failure → Wait 250ms                         │
│         ↓                                                                 │
│  Attempt 2 (250ms) ──────┬─ Success (200/202) → Done                   │
│                         └─ Failure → Wait 750ms                         │
│         ↓                                                                 │
│  Attempt 3 (1000ms) ─────┬─ Success (200/202) → Done                   │
│                         └─ Failure → Drop batch + log error            │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 5. API GATEWAY INGEST HANDLER (/internal/ingest)                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Validate:                                                               │
│    ├─ CollectorAuth middleware (Bearer token match)                     │
│    ├─ Body size ≤ 32 MiB                                               │
│    ├─ Batch size ≤ 10,000 spans                                        │
│    └─ JSON parse                                                        │
│         ↓                                                                 │
│  Extract X-AF-Tenant from header (or use default)                       │
│         ↓                                                                 │
│  For each span:                                                          │
│    repriceSpan(span)                                                    │
│      ├─ Read pricing_rules from DB                                      │
│      ├─ Match model pattern                                             │
│      └─ Recalculate cost = input_tokens * input_price + ...           │
│         ↓                                                                 │
│  BudgetEnforcer.CheckAndRecord()                                        │
│    ├─ Read budgets table for tenant                                     │
│    ├─ Sum new span costs                                                │
│    ├─ Compute monthly_usage to date                                     │
│    ├─ If hard_limit && exceeded:                                        │
│    │   ├─ Write decision_record (type: budget, result: deny)           │
│    │   └─ Return 429 Too Many Requests                                 │
│    ├─ Else if soft_limit && crossed:                                   │
│    │   ├─ Write decision_record (type: budget_threshold)              │
│    │   └─ Fire webhook alert                                            │
│    └─ Else: proceed                                                     │
│         ↓                                                                 │
│  pg.BulkInsertSpans() → PostgreSQL COPY FROM                           │
│    ├─ Batch insert into spans table                                     │
│    └─ Return span IDs with generated UUIDs                              │
│         ↓                                                                 │
│  upsertRunsFromSpans()                                                  │
│    ├─ Project span trace_id, parent_span_id into runs table            │
│    └─ ON CONFLICT DO UPDATE (upsert logic)                             │
│         ↓                                                                 │
│  For each span:                                                          │
│    Broadcast to WebSocket Hub:                                          │
│      {type: "span", trace_id, span_id, framework, cost_usd, ...}       │
│         ↓                                                                 │
│  Return 202 Accepted {spans_ingested: N}                                │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 6. DATA PERSISTENCE                                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  PostgreSQL spans table:                                                │
│    ├─ (span_id, tenant_id) PRIMARY KEY                                 │
│    ├─ trace_id (for trace grouping)                                     │
│    ├─ attributes JSONB (enriched span attrs)                           │
│    ├─ events JSONB (span events)                                        │
│    ├─ token columns: input_tokens, output_tokens, cache_read, etc     │
│    ├─ cost columns: cost_usd, input_cost_usd, etc (from repricing)   │
│    ├─ framework (detected enum: crewai, langgraph, etc)               │
│    └─ received_at (server timestamp)                                    │
│         ↓                                                                 │
│  Index maintenance:                                                      │
│    ├─ (trace_id) for trace lookups                                      │
│    ├─ (tenant_id, received_at) for pagination                          │
│    └─ (source_vendor) for vendor filtering                             │
│         ↓                                                                 │
│  Redis cache [OPTIONAL]:                                                │
│    └─ May cache processed traces (trace:{tenantID}:{traceId})          │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘

END-TO-END LATENCY: ~50-200ms (Collector enrich + export, Gateway repricing/budget + DB insert)
```

---

## Flow 2: Frontend Trace Request (Portal → API → DB)

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. PORTAL USER ACTION                                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  User clicks on trace in TracesPage or TraceDetail tab                  │
│         ↓                                                                 │
│  TraceDetail.tsx useEffect calls apiFetch("/traces/{traceId}")          │
│         ↓                                                                 │
│  apiFetch():                                                             │
│    ├─ credentials: 'include' (HttpOnly cookie af_token sent auto)      │
│    ├─ Headers: Authorization: Bearer <stored in Cookie>                │
│    └─ GET /api/v1/traces/{traceId}                                     │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 2. API GATEWAY MIDDLEWARE                                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Chi router dispatcher                                                   │
│         ↓                                                                 │
│  RequestID: generate UUID for request                                    │
│         ↓                                                                 │
│  RealIP: extract from X-Forwarded-For                                   │
│         ↓                                                                 │
│  Prometheus: record request start time                                   │
│         ↓                                                                 │
│  JWTAuth:                                                                │
│    ├─ Extract from cookie or Authorization header                       │
│    ├─ Verify signature (multi-secret rotation)                         │
│    ├─ Validate exp, aud claims                                         │
│    └─ Set claims in context                                             │
│         ↓                                                                 │
│  TenantInjector:                                                         │
│    ├─ Extract tenant_id from JWT claims                                 │
│    ├─ Set tenant_id in request context                                 │
│    └─ [DEFAULT] If missing: use default tenant "000...001"            │
│         ↓                                                                 │
│  RateLimit:                                                              │
│    ├─ Read sliding-window counter from Redis: rl:{tenant_id}:{endpoint}│
│    ├─ If count > limit:                                                 │
│    │   └─ Return 429 Too Many Requests                                 │
│    └─ Else: increment counter, proceed                                  │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 3. HANDLER: GetTrace (/api/v1/traces/{traceId})                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Check Redis cache:                                                      │
│    ├─ Key: trace:{tenantID}:{traceId}                                   │
│    ├─ If HIT: deserialize and return (skip DB)                         │
│    └─ If MISS: proceed to DB                                            │
│         ↓                                                                 │
│  pg.LoadTraceViewInputs(traceId):                                       │
│    ├─ SELECT * from spans WHERE trace_id = ? AND tenant_id = ?        │
│    ├─ SELECT * from policy_audit_log WHERE trace_id = ?                │
│    ├─ SELECT * from decision_records WHERE trace_id = ?                │
│    └─ Return (spans, auditEntries, decisions)                           │
│         ↓                                                                 │
│  observability.EnrichSpans():                                           │
│    ├─ For each span:                                                    │
│    │   ├─ Compute depth (count ancestors)                              │
│    │   ├─ Build parent-child lineage                                   │
│    │   ├─ Set outcome_status (success/error)                           │
│    │   ├─ Summarize policy decisions affecting span                    │
│    │   └─ Compute derived metrics                                      │
│    └─ Return enriched spans                                             │
│         ↓                                                                 │
│  auditEntriesToPolicyEvents():                                          │
│    ├─ Convert audit_log rows to structured PolicyEvent objects        │
│    └─ Enrich with evidence JSONB fields                                 │
│         ↓                                                                 │
│  observability.BuildTrace():                                            │
│    ├─ Assemble TraceView:                                               │
│    │   ├─ metadata (user, session, framework, status, cost)            │
│    │   ├─ Root span ID                                                  │
│    │   ├─ Span list (enriched)                                          │
│    │   ├─ Policy events                                                 │
│    │   └─ Topology (DAG from spans)                                     │
│    └─ Return TraceView                                                  │
│         ↓                                                                 │
│  observability.BuildTimeline():                                         │
│    ├─ Sort spans by start_time_ns                                       │
│    ├─ Compute waterfall: span-by-span timeline with nesting           │
│    └─ Compute duration, latency between spans                          │
│         ↓                                                                 │
│  JSON Marshal TraceView                                                  │
│         ↓                                                                 │
│  Redis SETEX trace:{tenantID}:{traceId} 300s <traceView>              │
│  (Cache for 5 minutes)                                                   │
│         ↓                                                                 │
│  Return 200 OK + JSON TraceView                                         │
│                                                                           │
│ [PARALLEL] Second request: useTraceDecisions(traceId)                   │
│   GET /api/v1/traces/{traceId}/decisions                               │
│   Returns: []PolicyDecision for the trace                               │
│   [May be separate API call or bundled]                                 │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 4. PORTAL RENDERING                                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  TraceDetail.tsx receives TraceView data                                │
│         ↓                                                                 │
│  Conditional render based on active tab:                                │
│    ├─ Waterfall tab:                                                    │
│    │   ├─ <SpanTimeline spans={traceView.spans} />                    │
│    │   └─ <SpanDetailPanel span={selected} />                          │
│    ├─ Spans tab:                                                        │
│    │   └─ Flat list with search/filter                                 │
│    ├─ Graph tab:                                                        │
│    │   └─ <TopologyGraph spans={traceView.spans} />                   │
│    └─ Policy tab:                                                       │
│        └─ <PolicyEventPanel events={traceView.policyEvents} />         │
│         ↓                                                                 │
│  Render MetricsPanel:                                                    │
│    ├─ Total duration                                                     │
│    ├─ Input/output token counts                                         │
│    ├─ Estimated cost                                                     │
│    └─ Error status (if any)                                             │
│         ↓                                                                 │
│  Browser displays interactive trace visualization                        │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘

LATENCY: ~100-300ms (DB query + JSON marshaling + React render)
CACHE HIT: ~10-20ms (Redis deserialize + React render)
```

---

## Flow 3: LLM Proxy Request (Developer SDK → OpenAI/Anthropic via Gateway)

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. DEVELOPER SDK SETUP                                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Developer app configures:                                              │
│    ├─ OPENAI_BASE_URL=http://localhost:8080/proxy/openai/v1           │
│    ├─ OPENAI_API_KEY=af-vk-<32hex>                                     │
│    └─ SDK sends requests to proxy instead of api.openai.com            │
│         ↓                                                                 │
│  Or uses HTTPS MITM proxy:                                              │
│    ├─ HTTP_PROXY=http://localhost:8443                                 │
│    ├─ HTTPS_PROXY=http://localhost:8443                                │
│    ├─ Install CA cert: wget http://localhost:8080/api/v1/netproxy/ca  │
│    └─ SDK routes all HTTPS through proxy                               │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 2. REQUEST ARRIVES AT PROXY                                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  POST http://localhost:8080/proxy/openai/v1/chat/completions          │
│  {                                                                        │
│    "model": "gpt-4",                                                     │
│    "messages": [...],                                                    │
│    "headers": {"Authorization": "Bearer af-vk-abc123def456..."}        │
│  }                                                                        │
│         ↓                                                                 │
│  LLMProxy.ServeHTTP():                                                   │
│    ├─ Parse request                                                      │
│    ├─ Buffer body (max 8 MiB)                                           │
│    ├─ Extract virtual key: af-vk-*                                      │
│    │   ├─ From Authorization header (Bearer prefix)                     │
│    │   └─ Or from x-api-key header                                      │
│    └─ Parse provider: /proxy/{provider}/v1/* → provider enum           │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 3. VAULT RESOLUTION (Virtual Key → Real Key)                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  vault.Resolve("af-vk-abc123..."):                                      │
│    ├─ Query virtual_keys table: SELECT real_key, tenant_id             │
│    │   WHERE virtual_key = "af-vk-abc123..."                           │
│    ├─ Decrypt real_key:                                                 │
│    │   ├─ Read nonce (first 12 bytes)                                   │
│    │   ├─ AES-256-GCM.Decrypt(ciphertext, master_key, nonce)          │
│    │   └─ Return plaintext real_key                                     │
│    └─ Return {realKey: "sk-...", tenantID: "123..."}                  │
│                                                                           │
│  If virtual key not found:                                              │
│    └─ Return 401 Unauthorized                                           │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 4. REQUEST PARSING (Provider-Specific)                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  parser.ParseRequest(body, provider):                                   │
│    ├─ [OpenAI] Unmarshal ChatCompletionRequest                         │
│    │   ├─ Extract model, messages, temperature, max_tokens             │
│    │   ├─ Detect if vision (vision/image messages)                     │
│    │   └─ Count estimated input tokens (rough heuristic)               │
│    ├─ [Anthropic] Unmarshal MessageRequest                             │
│    │   ├─ Extract model, system, messages                              │
│    │   ├─ Read usage metadata                                          │
│    │   └─ Count input + cache_read tokens                              │
│    ├─ [Other providers] Similar parsing with provider-specific fields  │
│    └─ Return ParsedRequest{model, messages, tokens, ...}               │
│                                                                           │
│  Estimated cost calculation:                                             │
│    ├─ Look up pricing_rules for {provider, model}                      │
│    ├─ Multiply input_tokens × input_rate                               │
│    └─ Estimate output at 2x input (heuristic)                          │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 5. POLICY EVALUATION (2-pass)                                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│ PASS 1: TRAFFIC RULES                                                    │
│  policyEngine.EvaluateTraffic(provider, model, tenantID, estimatedCost):│
│    ├─ Read all policy_rules WHERE rule_type = 'traffic'               │
│    ├─ For each rule:                                                    │
│    │   ├─ Match provider/model/env pattern (regex or exact)             │
│    │   ├─ Check token count threshold                                   │
│    │   ├─ Check time-of-day restrictions                               │
│    │   └─ Check per-user rate limits                                    │
│    ├─ On ALLOW: record decision, proceed                               │
│    ├─ On WARN: record decision, log warning, proceed                   │
│    └─ On DENY: record decision, return 403 Forbidden                   │
│         ↓                                                                 │
│ PASS 2: DLP SCANNING                                                     │
│  policyEngine.EvaluateDLP(requestBody, tenantID, policyRules):        │
│    ├─ Read all policy_rules WHERE rule_type = 'dlp'                   │
│    ├─ For each rule:                                                    │
│    │   ├─ Apply regex patterns to body (secrets, email, SSN, etc)      │
│    │   └─ Check pattern matches against rule categories                │
│    ├─ On ALLOW: record decision, proceed with original body            │
│    ├─ On WARN: record decision, proceed (no body modification)         │
│    ├─ On REDACT: record decision, strip matched values from body       │
│    └─ On DENY: record decision, return 403 Forbidden                   │
│         ↓                                                                 │
│  Write decision_record to DB:                                           │
│    {                                                                      │
│      decision_type: "policy",                                           │
│      result: "allow|warn|redact|deny",                                  │
│      inputs: {provider, model, tenantID, ...},                         │
│      evidence: {matched_rules: [...], actions: [...]}                  │
│    }                                                                      │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 6. BUDGET PRE-CHECK                                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  BudgetEnforcer.CheckAndRecord(estimatedCost, tenantID):               │
│    ├─ Read budgets table WHERE tenant_id = tenantID                    │
│    ├─ Compute monthly_usage_to_date:                                    │
│    │   SELECT SUM(cost_usd) FROM spans                                 │
│    │   WHERE tenant_id = tenantID                                       │
│    │   AND EXTRACT(MONTH FROM ts) = CURRENT_MONTH                      │
│    ├─ New total = monthly_usage_to_date + estimatedCost                │
│    ├─ If hard_limit && new_total > monthly_budget:                     │
│    │   ├─ Write decision_record (type: budget, result: deny)          │
│    │   └─ Return 429 Too Many Requests                                 │
│    ├─ Else if soft_limit && new_total > threshold:                     │
│    │   ├─ Write decision_record (type: budget_threshold)              │
│    │   ├─ Fire webhook alert (if configured)                          │
│    │   └─ Proceed (don't block)                                        │
│    └─ Else: proceed normally                                            │
│         ↓                                                                 │
│  Write to decision_records table with evidence                          │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 7. ROLLOUT ROUTING (A/B Testing / Canary)                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ProviderRouter.Route(model, tenantID):                                │
│    ├─ Read rollout_rules WHERE active = true                           │
│    ├─ For each rule:                                                    │
│    │   ├─ Match target_model pattern                                    │
│    │   ├─ Check traffic_pct (e.g., 10% → route 10% of traffic)       │
│    │   ├─ Deterministic routing: hash(tenantID) % 100 < traffic_pct    │
│    │   └─ If matched: use alternate_model/alternate_provider           │
│    └─ Write decision_record (type: routing)                             │
│         ↓                                                                 │
│  Return {targetModel, targetProvider} (original or redirected)          │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 8. UPSTREAM FORWARDING                                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Modify request headers:                                                 │
│    ├─ Replace Authorization with real key                               │
│    │   ├─ [OpenAI] Authorization: Bearer <real_key>                    │
│    │   └─ [Anthropic] x-api-key: <real_key>                            │
│    ├─ Remove af-vk-* virtual key                                        │
│    └─ Add X-AF-TenantID: <tenantID> (for internal tracking)           │
│         ↓                                                                 │
│  HTTP POST to upstream:                                                  │
│    ├─ [OpenAI] https://api.openai.com/v1/chat/completions             │
│    ├─ [Anthropic] https://api.anthropic.com/v1/messages                │
│    └─ [Others] provider-specific endpoint                               │
│         ↓                                                                 │
│  Stream response back to developer app                                   │
│    ├─ Copy response headers (except sensitive ones)                     │
│    ├─ Stream response body (may be chunked)                             │
│    └─ Do NOT modify response body                                       │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│ 9. RESPONSE PROCESSING & RECORDING                                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  [Asynchronously, after response sent to client]                        │
│         ↓                                                                 │
│  parser.ParseUsage(response, provider):                                 │
│    ├─ [OpenAI] Extract usage: input_tokens, output_tokens             │
│    ├─ [Anthropic] Extract usage from response headers/body             │
│    └─ Return actual {input_tokens, output_tokens, cache_tokens}       │
│         ↓                                                                 │
│  Re-price with actual counts:                                           │
│    ├─ actual_cost = input_tokens * input_rate + output_tokens * output_rate
│    ├─ variance = estimated_cost - actual_cost                          │
│    └─ Update database with actual cost                                  │
│         ↓                                                                 │
│  Create span record:                                                     │
│    {                                                                      │
│      trace_id: <from request header or generate>,                      │
│      span_id: <generate>,                                               │
│      attributes: {model, provider, messages, ...},                     │
│      input_tokens, output_tokens, (actual),                            │
│      cost_usd: actual_cost,                                            │
│      latency_ms: response_time,                                        │
│      error: null (if success)                                           │
│    }                                                                      │
│         ↓                                                                 │
│  BulkInsertSpans() → PostgreSQL                                         │
│         ↓                                                                 │
│  Broadcast to WebSocket Hub:                                            │
│    {type: "proxy_response", trace_id, model, cost_usd, tokens, ...}    │
│         ↓                                                                 │
│  Update monthly tenant spending in budgets table                         │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘

END-TO-END LATENCY: ~50-200ms (policy check + budget check + forward + response stream)
UPSTREAM LATENCY: Depends on OpenAI/Anthropic (add ~500ms-10s)
```

---

## Key Decision Points

| Decision | Impact | Priority |
|----------|--------|----------|
| Virtual key encryption strength (AES-256-GCM) | Security: 128-bit auth tag | Critical |
| Policy evaluation order (traffic before DLP) | Semantics: DLP runs even if traffic denied | Important |
| Budget pre-check vs. post-check | UX: blocks before forwarding vs. charges anyway | High |
| Span batching (32 spans, 1s timeout) | Latency/throughput tradeoff | Medium |
| Cache TTL (5 min for traces) | Freshness: may miss very recent updates | Medium |
| Single-process WebSocket Hub | Scalability: breaks with multiple replicas | Critical |

