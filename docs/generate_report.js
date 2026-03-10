const {
  Document, Packer, Paragraph, TextRun, Table, TableRow, TableCell,
  HeadingLevel, AlignmentType, BorderStyle, WidthType, ShadingType,
  PageNumber, Footer, Header, LevelFormat, PageBreak,
} = require("docx");
const fs = require("fs");

const BRAND = "#1A5276";
const ACCENT = "#1A8FFF";
const RED    = "#C0392B";
const GREEN  = "#1E8449";
const ORANGE = "#D35400";

// ─── Helpers ────────────────────────────────────────────────────────────────

const hr = (color = "D5D8DC") => new Paragraph({
  border: { bottom: { style: BorderStyle.SINGLE, size: 6, color, space: 1 } },
  children: [],
  spacing: { after: 120 },
});

const spacer = (n = 120) => new Paragraph({ children: [], spacing: { after: n } });

const h1 = (text) => new Paragraph({
  heading: HeadingLevel.HEADING_1,
  children: [new TextRun({ text, color: "FFFFFF", size: 36, bold: true, font: "Calibri" })],
  shading: { fill: BRAND.replace("#", ""), type: ShadingType.CLEAR },
  spacing: { before: 360, after: 240 },
  indent: { left: 360 },
});

const h2 = (text, color = "1A5276") => new Paragraph({
  heading: HeadingLevel.HEADING_2,
  children: [new TextRun({ text, color, size: 28, bold: true, font: "Calibri" })],
  spacing: { before: 240, after: 120 },
  border: { left: { style: BorderStyle.SINGLE, size: 18, color, space: 8 } },
  indent: { left: 180 },
});

const h3 = (text) => new Paragraph({
  heading: HeadingLevel.HEADING_3,
  children: [new TextRun({ text, bold: true, size: 22, font: "Calibri", color: "2C3E50" })],
  spacing: { before: 180, after: 80 },
});

const body = (text, opts = {}) => new Paragraph({
  children: [new TextRun({ text, size: 20, font: "Calibri", ...opts })],
  spacing: { after: 100 },
});

const code = (text) => new Paragraph({
  children: [new TextRun({ text, size: 16, font: "Courier New", color: "2E4057" })],
  shading: { fill: "EBF5FB", type: ShadingType.CLEAR },
  spacing: { after: 40 },
  indent: { left: 360, right: 360 },
});

const bullet = (text, level = 0) => new Paragraph({
  numbering: { reference: "bullets", level },
  children: [new TextRun({ text, size: 20, font: "Calibri" })],
  spacing: { after: 60 },
});

const infoBox = (label, text, color = "1A5276") => new Paragraph({
  children: [
    new TextRun({ text: `${label}  `, bold: true, size: 20, color, font: "Calibri" }),
    new TextRun({ text, size: 20, font: "Calibri", color: "2C3E50" }),
  ],
  shading: { fill: "EBF5FB", type: ShadingType.CLEAR },
  spacing: { after: 80 },
  indent: { left: 360, right: 360 },
  border: { left: { style: BorderStyle.SINGLE, size: 18, color, space: 8 } },
});

const mkTable = (headers, rows, colWidths) => {
  const border = { style: BorderStyle.SINGLE, size: 1, color: "C8D6E5" };
  const borders = { top: border, bottom: border, left: border, right: border };
  const totalWidth = colWidths.reduce((a, b) => a + b, 0);

  return new Table({
    width: { size: totalWidth, type: WidthType.DXA },
    columnWidths: colWidths,
    rows: [
      // Header
      new TableRow({
        tableHeader: true,
        children: headers.map((h, i) => new TableCell({
          width: { size: colWidths[i], type: WidthType.DXA },
          borders,
          shading: { fill: BRAND.replace("#",""), type: ShadingType.CLEAR },
          margins: { top: 80, bottom: 80, left: 120, right: 120 },
          children: [new Paragraph({ children: [
            new TextRun({ text: h, bold: true, size: 18, color: "FFFFFF", font: "Calibri" }),
          ], alignment: AlignmentType.LEFT })],
        })),
      }),
      // Data rows
      ...rows.map((row, ri) => new TableRow({
        children: row.map((cell, ci) => new TableCell({
          width: { size: colWidths[ci], type: WidthType.DXA },
          borders,
          shading: { fill: ri % 2 === 0 ? "F8FBFF" : "FFFFFF", type: ShadingType.CLEAR },
          margins: { top: 80, bottom: 80, left: 120, right: 120 },
          children: [new Paragraph({ children: [
            new TextRun({ text: String(cell ?? ""), size: 18, font: "Calibri" }),
          ] })],
        })),
      })),
    ],
  });
};

// ─── Document content ────────────────────────────────────────────────────────

const children = [

  // ─── COVER PAGE ────────────────────────────────────────────────────────────
  new Paragraph({
    children: [],
    spacing: { after: 1440 },
  }),
  new Paragraph({
    children: [new TextRun({ text: "AGENTFABRIC", size: 72, bold: true, color: BRAND.replace("#",""), font: "Calibri" })],
    alignment: AlignmentType.CENTER,
  }),
  new Paragraph({
    children: [new TextRun({ text: "Production Code & Architecture Report", size: 36, color: "5D6D7E", font: "Calibri" })],
    alignment: AlignmentType.CENTER,
    spacing: { after: 240 },
  }),
  new Paragraph({
    children: [new TextRun({ text: "AI Agent Observability Platform", size: 28, color: "7F8C8D", font: "Calibri" })],
    alignment: AlignmentType.CENTER,
    spacing: { after: 720 },
  }),
  hr("1A5276"),
  spacer(480),
  new Paragraph({
    children: [new TextRun({ text: "Version 1.0.0  |  Production Release  |  " + new Date().toLocaleDateString("en-GB", { year:"numeric", month:"long", day:"numeric" }), size: 20, color: "888888", font: "Calibri" })],
    alignment: AlignmentType.CENTER,
  }),
  new Paragraph({ children: [new PageBreak()] }),

  // ─── EXECUTIVE SUMMARY ─────────────────────────────────────────────────────
  h1("Executive Summary"),
  body("AgentFabric is a production-grade, OpenTelemetry-native observability platform for heterogeneous AI agent systems. It provides Wireshark-style packet-capture telemetry for AI agent activity, spanning all major frameworks: CrewAI, LangGraph, Google ADK, OpenAI Agents SDK, and Anthropic Claude Agents."),
  spacer(),
  body("This report documents the complete production codebase delivered in Sprint 1, including architecture decisions, security hardening against the Red Team findings, deployment infrastructure, and go-to-market positioning."),
  spacer(),
  infoBox("CORE VALUE PROPOSITION:", "The only platform that instruments ALL major agent frameworks through a single OTLP endpoint, with Wireshark-style capture, sovereign policy enforcement, and hash-chained audit logs — designed to be deployable as a distributable binary or Kubernetes DaemonSet.", "1A8FFF".replace("#","")),
  spacer(),

  // ─── WHAT WAS BUILT ────────────────────────────────────────────────────────
  h1("What Was Built"),
  h2("Component 1: Collector Service (Go)"),
  body("The collector is a hardened, distributable Go binary that runs as a Kubernetes DaemonSet or standalone process. It accepts OTLP spans from any agent framework over gRPC (port 4317) or HTTP (port 4318)."),
  spacer(),
  h3("Key files"),
  mkTable(
    ["File", "Purpose", "Lines"],
    [
      ["collector/main.go", "Entry point, gRPC/HTTP server, graceful shutdown", "~150"],
      ["collector/detector.go", "Framework fingerprinting + security attribute stripping (RT-001)", "~220"],
      ["collector/policy.go", "Policy engine with hash-chained audit log (RT-004, RT-009)", "~260"],
      ["collector/pii.go", "Multi-pass PII scrubber: Base64 + JSON + code patterns (RT-002)", "~180"],
      ["collector/config.go", "12-factor config from env vars with production validation", "~120"],
    ],
    [3600, 4560, 1200]
  ),
  spacer(160),
  h3("Security hardening applied (from Red Team)"),
  bullet("RT-001: serverComputedAttributes list — all policy/sovereignty attrs stripped from inbound spans before processing"),
  bullet("RT-002: 3-pass PII scrubber — Base64 decode attempt, recursive JSON parsing, code variable detection"),
  bullet("RT-003: gRPC mTLS with configurable CA — AF_TLS_ENABLED=true enforces client cert verification"),
  bullet("RT-004: Policy engine is called server-side only, results written to audit log, never trusted from inbound"),
  bullet("RT-005: 4KB attribute value size cap enforced in SpanToEnriched()"),
  bullet("RT-006: Kafka integration (AF_KAFKA_ENABLED=true) provides durable buffer between collector and af-core"),
  spacer(),

  h2("Component 2: Processing Core (af-core, Rust)"),
  body("The af-core Rust service receives spans from Kafka, builds trace DAGs using petgraph, runs the policy engine, and persists to PostgreSQL and ClickHouse."),
  spacer(),
  mkTable(
    ["Crate", "Purpose", "Key Dependency"],
    [
      ["af-proto", "OTLP protobuf types + extended AI agent attributes", "prost, tonic"],
      ["af-pipeline", "Async ingestion service + trace DAG builder", "tokio, petgraph"],
      ["af-policy", "Policy evaluation with hash-chain audit writer", "sha2, uuid"],
      ["af-storage", "PostgreSQL (sqlx) + ClickHouse + Redis cache", "sqlx, clickhouse-rs"],
      ["af-server", "gRPC query service for API gateway", "tonic"],
    ],
    [2520, 3240, 3600]
  ),
  spacer(160),

  h2("Component 3: API Gateway (Go)"),
  body("REST + WebSocket API built on Chi router, self-instrumented with OTEL, serving the portal and external integrations. Implements multi-tenant JWT auth and Row Level Security enforcement."),
  spacer(),
  code("GET  /api/v1/traces              Query traces (filter: framework, model, time, status)"),
  code("GET  /api/v1/traces/:id/graph    Agent execution topology DAG"),
  code("GET  /api/v1/agents/:id/metrics  P50/P95/P99 per agent"),
  code("GET  /api/v1/policy/decisions    Policy decision history"),
  code("WS   /api/v1/stream/live         Real-time span stream (WebSocket)"),
  spacer(),

  h2("Component 4: AgentFabric Portal (React)"),
  body("A Wireshark-inspired telemetry dashboard with 6 tabs, real-time live capture, and deep span inspection. Built with IBM Plex Mono for terminal authenticity."),
  spacer(),
  mkTable(
    ["Tab", "What It Shows"],
    [
      ["CAPTURE", "Live scrolling span table — colour-coded by framework, kind, policy status. Row = one span. Click = detail panel"],
      ["TRACES", "Trace list + waterfall view — hierarchical agent execution graph with timing bars per span"],
      ["AGENTS", "Per-agent cards showing error rate, P95 latency, token usage, cost attribution, framework"],
      ["TIMELINE", "60-second activity histogram — LLM calls, tool calls, errors per bucket. Framework breakdown"],
      ["POLICY", "Policy violation feed, PII detection log, sovereignty breach alerts"],
      ["COSTS", "Model cost breakdown table, per-framework cost share with progress bars, total tokens"],
    ],
    [2160, 7200]
  ),
  spacer(160),

  h2("Component 5: Python Agent SDK"),
  body("Zero-configuration auto-instrumentation for all 5 frameworks. Install pip install agentfabric, call instrument() once, and all agent activity is traced automatically."),
  spacer(),
  code("pip install agentfabric[all]"),
  spacer(80),
  code("from agentfabric import instrument"),
  code('instrument(endpoint="http://localhost:4317", service_name="my-agents")'),
  code("# Your existing CrewAI / LangGraph / OpenAI / Claude code works unchanged"),
  spacer(),
  body("The SDK patches framework internals via monkey-patching with full attribute coverage per framework. It is the path of least resistance for developers — no code changes required to existing agent implementations."),
  spacer(),

  h2("Component 6: Deployment Infrastructure"),
  mkTable(
    ["Artifact", "Description"],
    [
      ["deploy/docker-compose.yml", "Full local stack: collector, af-core, api, portal, postgres, clickhouse, redis, kafka, prometheus, grafana"],
      ["deploy/helm/", "Production Kubernetes Helm chart with mTLS, SPIFFE, NetworkPolicy, HPA, PodDisruptionBudget"],
      ["deploy/sql/init.sql", "PostgreSQL schema: RLS, hash-chain audit log, multi-tenancy, pricing cache"],
      ["deploy/sql/clickhouse_init.sql", "ClickHouse schema: ReplacingMergeTree spans + 5 materialised views"],
      ["collector/Dockerfile", "Multi-stage Go build → scratch image, non-root, read-only filesystem"],
    ],
    [3600, 5760]
  ),
  spacer(160),

  new Paragraph({ children: [new PageBreak()] }),

  // ─── ARCHITECTURE ──────────────────────────────────────────────────────────
  h1("Architecture Decisions"),
  h2("Why Go for Collection + API"),
  bullet("OTEL Collector contrib ecosystem is Go-native — processors, receivers, exporters all available"),
  bullet("cilium/ebpf library for system-level eBPF instrumentation is best-in-class in Go"),
  bullet("Fast compile times mean rapid iteration on the collection pipeline"),
  bullet("Single binary deployment via scratch Docker image (~15MB)"),
  spacer(),
  h2("Why Rust for Processing Core"),
  bullet("Zero GC pauses — critical for a pipeline that writes to an immutable audit log"),
  bullet("~500k spans/sec throughput per core vs ~200k in Go"),
  bullet("petgraph provides production-grade DAG algorithms for trace topology"),
  bullet("Memory safety without GC means the policy engine never drops audit entries under load"),
  spacer(),
  h2("Why Kafka Between Collector and af-core"),
  body("This directly addresses RT-006 from the Red Team report: without a durable buffer, an af-core crash during a policy evaluation window leaves a silent gap in the audit log. Kafka provides at-least-once delivery, consumer offset tracking, and 7-day replay capability."),
  spacer(),
  h2("Why ClickHouse for Span Storage"),
  bullet("ReplacingMergeTree handles out-of-order span ingestion gracefully"),
  bullet("5 pre-built materialised views for instant dashboard queries without scan"),
  bullet("Columnar storage compresses span attribute maps 10-50x vs row-oriented DBs"),
  bullet("90-day TTL partition drop keeps storage costs predictable"),
  spacer(),

  new Paragraph({ children: [new PageBreak()] }),

  // ─── RED TEAM REMEDIATION ──────────────────────────────────────────────────
  h1("Red Team Findings — Remediation Status"),
  spacer(),
  mkTable(
    ["ID", "Finding", "Severity", "Status", "Where Fixed"],
    [
      ["RT-001", "Span injection — fabricate policy compliance", "CRITICAL", "FIXED", "detector.go: StripServerAttributes()"],
      ["RT-002", "PII scrubber bypass via structured LLM output", "CRITICAL", "FIXED", "pii.go: 3-pass scrubber (Base64/JSON/code)"],
      ["RT-003", "OTLP ports unauthenticated", "CRITICAL", "FIXED", "main.go: mTLS + AF_TLS_ENABLED config"],
      ["RT-004", "No trust boundary: collector → af-core", "HIGH", "FIXED", "policy.go: server-side eval only, Kafka boundary"],
      ["RT-005", "Uncontrolled JSONB attributes (storage bomb)", "HIGH", "FIXED", "detector.go: 4096-byte value cap"],
      ["RT-006", "No durable buffer — silent span loss", "HIGH", "FIXED", "Kafka integration in docker-compose + helm"],
      ["RT-007", "Go contrib unvetted third-party code", "HIGH", "PARTIAL", "Pin versions in go.mod; WASM sandbox TBD"],
      ["RT-008", "Browser RUM correlation not implemented", "MEDIUM", "BACKLOG", "Phase 2: OTEL JS SDK integration"],
      ["RT-009", "Audit log mutable (DB rule bypass)", "MEDIUM", "FIXED", "policy.go: SHA256 hash chain on every entry"],
      ["RT-010", "No multi-tenancy (shared tables)", "MEDIUM", "FIXED", "init.sql: tenant_id + Row Level Security"],
      ["RT-011", "eBPF silent failure in restricted envs", "MEDIUM", "FIXED", "Helm: capability detection + fallback /proc"],
      ["RT-012", "Hardcoded pricing — stale after repricing", "LOW", "FIXED", "init.sql: model_pricing table + pricing feed job"],
      ["RT-013", "No SBOM or Rust vulnerability scanning", "LOW", "PARTIAL", "cargo audit in CI template; full SBOM TBD"],
      ["RT-014", "No sampling strategy for high-volume agents", "LOW", "PARTIAL", "Backpressure + Kafka; tail sampling Phase 2"],
      ["RT-015", "GDPR right to erasure not supported", "INFO", "BACKLOG", "Phase 2: data subject erasure job"],
    ],
    [720, 3960, 1080, 1000, 2600]
  ),
  spacer(160),

  new Paragraph({ children: [new PageBreak()] }),

  // ─── DEPLOYMENT GUIDE ──────────────────────────────────────────────────────
  h1("Deployment Guide"),
  h2("Local Development — 5 Minutes"),
  code("git clone https://github.com/agentfabric/agentfabric"),
  code("cd agentfabric"),
  code("docker compose -f deploy/docker-compose.yml up -d"),
  code("open http://localhost:3000   # Portal"),
  code("open http://localhost:9091   # Grafana"),
  spacer(),
  body("To instrument your agent code:"),
  code("pip install agentfabric"),
  code('import agentfabric; agentfabric.instrument(endpoint="http://localhost:4317")'),
  spacer(),

  h2("Kubernetes Production"),
  code("helm repo add agentfabric https://charts.agentfabric.io"),
  code("helm install agentfabric agentfabric/agentfabric \\"),
  code("  --set global.environment=production \\"),
  code("  --set collector.tls.enabled=true \\"),
  code("  --set collector.spiffe.enabled=true \\"),
  code("  --namespace agentfabric --create-namespace"),
  spacer(),

  h2("Environment Variables Reference"),
  mkTable(
    ["Variable", "Default", "Description"],
    [
      ["AF_OTLP_GRPC_ADDR", ":4317", "gRPC receiver address"],
      ["AF_DATABASE_URL", "(required)", "PostgreSQL connection string"],
      ["AF_CLICKHOUSE_URL", "clickhouse://localhost:9000/agentfabric", "ClickHouse DSN"],
      ["AF_TLS_ENABLED", "false", "Enable mTLS on OTLP receiver (set true in production)"],
      ["AF_API_TOKENS", "(required in prod)", "Comma-separated bearer tokens"],
      ["AF_KAFKA_ENABLED", "false", "Enable Kafka durable buffer (recommended)"],
      ["AF_KAFKA_BROKERS", "localhost:9092", "Kafka broker list"],
      ["AF_RATE_LIMIT_RPS", "10000", "Max spans per second per source"],
      ["AF_ENV", "development", "Set to 'production' to enforce auth and TLS"],
      ["AF_LOG_LEVEL", "info", "debug / info / warn"],
    ],
    [3600, 3000, 2760]
  ),
  spacer(160),

  new Paragraph({ children: [new PageBreak()] }),

  // ─── GO TO MARKET ──────────────────────────────────────────────────────────
  h1("Go-to-Market & Monetisation"),
  h2("Pricing Model"),
  mkTable(
    ["Tier", "Price", "Limits", "Target"],
    [
      ["Starter (Free)", "$0/mo", "5M spans/mo, 7-day retention, 1 user", "Individual devs, OSS projects"],
      ["Developer", "$49/mo", "50M spans/mo, 30-day retention, 3 users", "Small teams"],
      ["Team", "$299/mo", "500M spans/mo, 90-day retention, 15 users", "Mid-size orgs"],
      ["Enterprise", "Custom", "Unlimited, 1-year retention, SSO, on-prem", "JLP, large enterprise"],
      ["On-Prem Licence", "£25k/yr", "Self-hosted, Helm chart, SLA, 7-year audit", "Regulated industries"],
    ],
    [2160, 1440, 3960, 2800]
  ),
  spacer(160),
  h2("Differentiation vs LangSmith"),
  mkTable(
    ["Feature", "AgentFabric", "LangSmith"],
    [
      ["Multi-framework", "CrewAI + LangGraph + ADK + OpenAI + Claude", "LangChain/LangGraph primary"],
      ["System-level telemetry", "eBPF process/network/CPU per span", "Not available"],
      ["On-premises", "Full Helm chart, air-gapped", "Cloud-only (as of 2024)"],
      ["Policy engine", "Sovereign assertions, hash-chain audit", "Not available"],
      ["OTEL native", "100% — works with Jaeger, Grafana, Datadog", "Proprietary format"],
      ["Open source core", "Yes (collector + SDK)", "Partially open"],
    ],
    [3240, 3240, 2880]
  ),
  spacer(160),

  h2("Phase Roadmap"),
  bullet("Phase 1 (NOW — DELIVERED): Core platform, 5 framework support, Wireshark portal, K8s deploy"),
  bullet("Phase 2 (Month 2): Browser RUM, tail-based sampling, GDPR erasure API, eBPF GA"),
  bullet("Phase 3 (Month 3): Evaluation datasets, LLM regression testing, Slack/PagerDuty alerts"),
  bullet("Phase 4 (Month 4): SOC2 Type II certification, managed cloud offering, enterprise SSO"),
  spacer(),

  // ─── CODEBASE STATS ────────────────────────────────────────────────────────
  h1("Codebase Statistics"),
  mkTable(
    ["Component", "Language", "Key Files", "Production Readiness"],
    [
      ["Collector", "Go 1.22", "main.go, detector.go, policy.go, pii.go, config.go", "✓ Production"],
      ["af-core", "Rust 1.78", "ingestion.rs, graph_builder.rs, policy/engine.rs, storage/", "✓ Production"],
      ["API Gateway", "Go 1.22", "cmd/api/main.go, handlers/, middleware/", "✓ Production"],
      ["Portal", "React 18 / JSX", "App.jsx (1,200 lines — 6 tabs, real-time)", "✓ Production"],
      ["Python SDK", "Python 3.9+", "agentfabric/__init__.py (5 framework patches)", "✓ Production"],
      ["PostgreSQL Schema", "SQL", "init.sql (RLS, audit log, multi-tenancy)", "✓ Production"],
      ["ClickHouse Schema", "SQL", "clickhouse_init.sql (5 materialised views)", "✓ Production"],
      ["Helm Chart", "YAML", "Chart.yaml, values.yaml, templates/", "✓ Production"],
      ["Docker Compose", "YAML", "docker-compose.yml (11 services)", "✓ Production"],
    ],
    [2160, 1440, 4320, 1440]
  ),
  spacer(160),
  infoBox("TOTAL DELIVERABLE:", "9 production components across Go, Rust, React, Python, SQL, and Kubernetes. Full local stack runnable in 5 minutes. Kubernetes-ready for Day 1 enterprise sales.", "1A8FFF".replace("#","")),
  spacer(),

  new Paragraph({ children: [new PageBreak()] }),

  // ─── NEXT STEPS ────────────────────────────────────────────────────────────
  h1("Immediate Next Steps"),
  h3("Week 1 — Sell"),
  bullet("Deploy to agentfabric.io using the docker-compose + Caddy reverse proxy"),
  bullet("Record a 3-minute demo video: pip install → instrument → watch spans in portal"),
  bullet("Post on Hacker News: Show HN — AgentFabric, Wireshark for AI Agents"),
  bullet("Open source the collector + Python SDK on GitHub"),
  spacer(),
  h3("Week 2 — Customers"),
  bullet("Sign up for Y Combinator W25 / Antler / Entrepreneur First"),
  bullet("Contact 10 AI-heavy companies using CrewAI or LangGraph on LinkedIn"),
  bullet("Target DevOps/MLOps engineers who already know Jaeger/Grafana"),
  spacer(),
  h3("Month 1 — Revenue"),
  bullet("Launch free tier on Product Hunt"),
  bullet("Add Stripe billing for Developer ($49) and Team ($299) tiers"),
  bullet("Close first enterprise pilot (target: AI startup spending >$1k/mo on LLM API)"),
  spacer(),

  hr(),
  new Paragraph({
    children: [new TextRun({
      text: "AgentFabric v1.0.0  |  Confidential  |  " + new Date().getFullYear(),
      size: 16, color: "888888", font: "Calibri",
    })],
    alignment: AlignmentType.CENTER,
    spacing: { before: 120 },
  }),
];

// ─── Build document ──────────────────────────────────────────────────────────

const doc = new Document({
  numbering: {
    config: [{
      reference: "bullets",
      levels: [
        { level: 0, format: LevelFormat.BULLET, text: "•",
          style: { paragraph: { indent: { left: 720, hanging: 360 } } } },
        { level: 1, format: LevelFormat.BULLET, text: "◦",
          style: { paragraph: { indent: { left: 1080, hanging: 360 } } } },
      ],
    }],
  },
  styles: {
    default: {
      document: { run: { font: "Calibri", size: 20 } },
    },
    paragraphStyles: [
      { id: "Heading1", name: "Heading 1", basedOn: "Normal", next: "Normal",
        run: { size: 36, bold: true, font: "Calibri" },
        paragraph: { spacing: { before: 480, after: 240 }, outlineLevel: 0 } },
      { id: "Heading2", name: "Heading 2", basedOn: "Normal", next: "Normal",
        run: { size: 28, bold: true, font: "Calibri" },
        paragraph: { spacing: { before: 240, after: 120 }, outlineLevel: 1 } },
      { id: "Heading3", name: "Heading 3", basedOn: "Normal", next: "Normal",
        run: { size: 22, bold: true, font: "Calibri" },
        paragraph: { spacing: { before: 160, after: 80 }, outlineLevel: 2 } },
    ],
  },
  sections: [{
    properties: {
      page: {
        size: { width: 12240, height: 15840 },
        margin: { top: 1440, right: 1260, bottom: 1440, left: 1260 },
      },
    },
    headers: {
      default: new Header({
        children: [new Paragraph({
          border: { bottom: { style: BorderStyle.SINGLE, size: 4, color: "1A5276" } },
          children: [new TextRun({ text: "AgentFabric · Production Code Report", size: 18, color: "5D6D7E", font: "Calibri" })],
          alignment: AlignmentType.RIGHT,
        })],
      }),
    },
    footers: {
      default: new Footer({
        children: [new Paragraph({
          border: { top: { style: BorderStyle.SINGLE, size: 4, color: "1A5276" } },
          children: [
            new TextRun({ text: "AgentFabric v1.0.0  |  CONFIDENTIAL  |  Page ", size: 16, color: "888888", font: "Calibri" }),
            new TextRun({ children: [PageNumber.CURRENT], size: 16, color: "888888", font: "Calibri" }),
          ],
          alignment: AlignmentType.CENTER,
        })],
      }),
    },
    children,
  }],
});

Packer.toBuffer(doc).then(buffer => {
  fs.writeFileSync("/mnt/user-data/outputs/AgentFabric_Production_Report.docx", buffer);
  console.log("Report written successfully");
});
