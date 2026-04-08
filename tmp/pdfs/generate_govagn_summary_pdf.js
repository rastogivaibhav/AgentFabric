const fs = require('fs');
const path = require('path');

const outputDir = path.join('output', 'pdf');
fs.mkdirSync(outputDir, { recursive: true });

const outputPath = path.join(outputDir, 'govagn_app_summary_one_page.pdf');

const left = 44;
const right = 568;
let y = 758;
const items = [];

function wrap(text, maxChars) {
  const words = text.split(' ');
  const lines = [];
  let current = '';
  for (const w of words) {
    const cand = current ? `${current} ${w}` : w;
    if (cand.length <= maxChars) {
      current = cand;
    } else {
      if (current) lines.push(current);
      current = w;
    }
  }
  if (current) lines.push(current);
  return lines;
}

function addTitle(text) {
  items.push({ font: 'F2', size: 17, x: left, y, text });
  y -= 22;
}

function addRule() {
  items.push({ rule: true, y: y + 6 });
  y -= 8;
}

function addHeading(text) {
  items.push({ font: 'F2', size: 11.5, x: left, y, text });
  y -= 13;
}

function addText(text) {
  for (const line of wrap(text, 102)) {
    items.push({ font: 'F1', size: 9.5, x: left, y, text: line });
    y -= 11;
  }
}

function addBullet(text) {
  const wrapped = wrap(text, 98);
  wrapped.forEach((line, idx) => {
    items.push({ font: 'F1', size: 9.5, x: left, y, text: idx === 0 ? `- ${line}` : `  ${line}` });
    y -= 11;
  });
}

function addGap(lines = 1) {
  y -= 5 * lines;
}

addTitle('Govagn - One-Page App Summary');
addRule();

addHeading('What It Is');
addText('Govagn is a self-hosted control plane for enterprise AI runtime governance, observability, and control.');
addText('It unifies policy enforcement, cost attribution, prompt lifecycle, and runtime visibility for LLM and agent traffic.');
addGap();

addHeading('Who It Is For');
addText('Primary persona: enterprise AI platform engineer or governance owner operating internal LLM workloads in controlled environments.');
addGap();

addHeading('What It Does');
addBullet('Collects OTLP telemetry and runtime metadata for LLM and agent executions.');
addBullet('Provides trace, span, lineage, redaction, and live runtime visibility in the portal.');
addBullet('Enforces policy and guardrails, including preview and simulation workflows.');
addBullet('Calculates per-request and per-span costs with pricing rules and budget workflows.');
addBullet('Manages prompt versioning, promotion, and trace-to-release linkage.');
addBullet('Supports provider adapters or routing for OpenAI, Anthropic, Google, Vertex AI, and Bedrock.');
addBullet('Ships self-hosted deployment paths via Docker Compose and Kubernetes or Helm.');
addGap();

addHeading('How It Works (Repo-Evidenced Architecture)');
addBullet('Components: agent-sdk (Python instrumentation), collector (OTLP ingest and enrichment), api-gateway (policy, pricing, prompt, eval, budget, audit control plane), portal (React operations UI), PostgreSQL (system of record), Redis (cache and coordination).');
addBullet('Data flow: app or agent to SDK or OTLP to collector to api-gateway to PostgreSQL and Redis; portal queries api-gateway for operations and admin workflows.');
addBullet('Local stack also includes Prometheus, Alertmanager, Grafana, and Jaeger for monitoring and diagnostics.');
addGap();

addHeading('How To Run (Minimal Getting Started)');
addBullet('Install Docker and Docker Compose.');
addBullet('From repo root, run: make dev');
addBullet('Open portal at http://localhost:3000 (API gateway at http://localhost:8080).');
addBullet('Register at least one provider key, then send instrumented or proxied traffic.');
addBullet('Initial admin credentials: Not found in repo.');

items.push({ font: 'F1I', size: 7.6, x: left, y: 18, text: 'Repo evidence: README.md, docker-compose.yml, Makefile, docs/SETUP_AND_ONBOARDING.md' });

if (y < 38) {
  throw new Error(`Content overflow on page (y=${y})`);
}

function escapePdfText(s) {
  return s.replace(/\\/g, '\\\\').replace(/\(/g, '\\(').replace(/\)/g, '\\)');
}

let stream = '0.2 w\n';
for (const item of items) {
  if (item.rule) {
    stream += `${left} ${item.y} m ${right} ${item.y} l S\n`;
    continue;
  }
  let fontRef = '/F1';
  if (item.font === 'F2') fontRef = '/F2';
  if (item.font === 'F1I') fontRef = '/F3';
  stream += `BT\n${fontRef} ${item.size} Tf\n1 0 0 1 ${item.x} ${item.y} Tm\n(${escapePdfText(item.text)}) Tj\nET\n`;
}

const streamLength = Buffer.byteLength(stream, 'ascii');

const objects = [];
objects.push('1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n');
objects.push('2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n');
objects.push('3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R /F2 6 0 R /F3 7 0 R >> >> /Contents 4 0 R >>\nendobj\n');
objects.push(`4 0 obj\n<< /Length ${streamLength} >>\nstream\n${stream}endstream\nendobj\n`);
objects.push('5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n');
objects.push('6 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>\nendobj\n');
objects.push('7 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Oblique >>\nendobj\n');

let pdf = '%PDF-1.4\n';
const offsets = [0];
for (const obj of objects) {
  offsets.push(Buffer.byteLength(pdf, 'ascii'));
  pdf += obj;
}

const xrefStart = Buffer.byteLength(pdf, 'ascii');
pdf += `xref\n0 ${objects.length + 1}\n`;
pdf += '0000000000 65535 f \n';
for (let i = 1; i <= objects.length; i += 1) {
  pdf += `${String(offsets[i]).padStart(10, '0')} 00000 n \n`;
}
pdf += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xrefStart}\n%%EOF\n`;

fs.writeFileSync(outputPath, pdf, { encoding: 'ascii' });
console.log(outputPath);
