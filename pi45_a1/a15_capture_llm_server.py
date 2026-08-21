#!/usr/bin/env python3
import argparse, json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--listen', type=int, default=9090)
    ap.add_argument('--out', required=True)
    args = ap.parse_args()
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            return
        def _send(self, code, obj):
            body = json.dumps(obj).encode()
            self.send_response(code)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        def do_GET(self):
            if self.path in ('/health', '/v1/health'):
                self._send(200, {'status':'ok'})
            else:
                self._send(404, {'error':'not_found'})
        def do_POST(self):
            n = int(self.headers.get('Content-Length','0'))
            raw = self.rfile.read(n)
            try:
                req = json.loads(raw)
            except Exception:
                req = {'_raw': raw.decode('utf-8','replace')}
            out.write_text(json.dumps(req, indent=2, sort_keys=True), encoding='utf-8')
            content = json.dumps({
                'action':'ACTION6','x':32,'y':32,
                'hypothesis':'Audit probe only.',
                'goal':'Determine what the interface exposes.',
                'rationale':'Audit-only deterministic response.',
                'confidence':0.1,
            })
            self._send(200, {
                'id':'a15-perception-audit','object':'chat.completion','created':0,
                'model':'a15-audit-stub',
                'choices':[{'index':0,'message':{'role':'assistant','content':content},'finish_reason':'stop'}],
                'usage':{'prompt_tokens':1,'completion_tokens':1,'total_tokens':2},
            })

    ThreadingHTTPServer(('127.0.0.1', args.listen), Handler).serve_forever()

if __name__ == '__main__':
    main()
