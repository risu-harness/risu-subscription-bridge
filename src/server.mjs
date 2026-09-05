import http from 'node:http';
import {randomUUID, timingSafeEqual} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import {BridgeError, normalize, StopFilter} from './protocol.mjs';

const page = await readFile(new URL('./setup.html', import.meta.url), 'utf8');
const errorBody = e => ({error: {message: e instanceof BridgeError ? e.message : 'Internal bridge error.', type: e.code ?? 'bridge_error', code: e.code ?? 'bridge_error'}});
function equal(a, b) { const x = Buffer.from(a ?? ''), y = Buffer.from(b); return x.length === y.length && timingSafeEqual(x, y); }
async function readJSON(req) {
  if (!req.headers['content-type']?.startsWith('application/json')) throw new BridgeError('Use application/json.', 415, 'content_type');
  let size = 0; const parts = [];
  for await (const part of req) { size += part.length; if (size > 2 * 1024 * 1024) throw new BridgeError('Request exceeds 2 MiB.', 413, 'request_too_large'); parts.push(part); }
  try { return JSON.parse(Buffer.concat(parts).toString('utf8')); } catch { throw new BridgeError('Invalid JSON.', 400, 'invalid_json'); }
}

export function createBridge({adapter, token, port = 8787, origins = ['https://risuai.xyz', 'https://risuai.net', 'tauri://localhost', 'http://tauri.localhost'], runtime = '', shutdown}) {
  const metrics = {requests: 0, completed: 0, cancelled: 0, failed: 0, last: null};
  let busy = false;
  const server = http.createServer(async (req, res) => {
    const boundPort = server.address()?.port ?? port;
    const local = [`http://127.0.0.1:${boundPort}`, `http://localhost:${boundPort}`];
    const hosts = new Set([`127.0.0.1:${boundPort}`, `localhost:${boundPort}`]);
    const allowed = new Set([...origins, ...local]);
    res.setHeader('Cache-Control', 'no-store'); res.setHeader('X-Content-Type-Options', 'nosniff');
    const json = (status, body) => { res.writeHead(status, {'Content-Type': 'application/json; charset=utf-8'}); res.end(JSON.stringify(body)); };
    try {
      if (!hosts.has(req.headers.host)) throw new BridgeError('Invalid Host.', 403, 'invalid_host');
      const origin = req.headers.origin;
      if (origin && !allowed.has(origin)) throw new BridgeError(`Origin is not allowed: ${origin.slice(0, 200)}`, 403, 'origin_denied');
      if (origin) { res.setHeader('Access-Control-Allow-Origin', origin); res.setHeader('Vary', 'Origin'); }
      if (req.method === 'OPTIONS') {
        res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
        res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type, X-Proxy-Risu');
        res.setHeader('Access-Control-Allow-Private-Network', 'true'); res.writeHead(204); return res.end();
      }
      const path = new URL(req.url, local[0]).pathname;
      if (req.method === 'GET' && path === '/') {
        res.setHeader('Content-Security-Policy', "default-src 'self'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'");
        res.writeHead(200, {'Content-Type': 'text/html; charset=utf-8'}); return res.end(page);
      }
      if (req.method === 'GET' && path === '/setup.js') { res.writeHead(200, {'Content-Type': 'text/javascript'}); return res.end(await readFile(new URL('./setup.js', import.meta.url))); }
      if (req.method === 'GET' && path === '/healthz') return json(200, {ok: Boolean(adapter.alive), version: '0.1.0'});
      if (!equal(req.headers.authorization, `Bearer ${token}`)) throw new BridgeError('Local bridge key required.', 401, 'unauthorized');
      if (path.startsWith('/internal/') && origin && !local.includes(origin)) throw new BridgeError('Setup must be opened locally.', 403, 'setup_origin');
      if (req.method === 'GET' && path === '/internal/status') return json(200, {account: await adapter.account(), metrics, busy, runtime, adapter: adapter.name ?? 'app-server', delivery: adapter.delivery ?? 'token-delta', mode: 'Risu owns history; fresh ephemeral generation per request', controlPlane: 'App Server for login/account/models only when using exec'});
      if (req.method === 'POST' && path === '/internal/stop' && shutdown) { await readJSON(req); json(200, {ok: true}); setImmediate(shutdown); return; }
      if (req.method === 'POST' && path === '/internal/login') { await readJSON(req); return json(200, await adapter.login()); }
      if (req.method === 'GET' && path === '/v1/models') {
        const models = await adapter.models();
        return json(200, {object: 'list', data: [{id: 'subscription-default', object: 'model', owned_by: 'local-bridge'}, ...models.map(m => ({id: m.model ?? m.id, object: 'model', owned_by: 'openai'}))]});
      }
      if (req.method !== 'POST' || path !== '/v1/chat/completions') throw new BridgeError('Not found.', 404, 'not_found');
      const request = normalize(await readJSON(req));
      if (busy) throw new BridgeError('One generation at a time in this spike. Retry after completion.', 429, 'bridge_busy');
      busy = true; metrics.requests++;
      const abort = new AbortController();
      const disconnected = () => { if (!res.writableEnded) abort.abort(); };
      res.once('close', disconnected);
      const id = `chatcmpl-${randomUUID()}`, created = Math.floor(Date.now() / 1000);
      let text = '', stopReached = false;
      const filter = new StopFilter(request.stop);
      const chunk = (delta, finish_reason = null, model = request.model) => ({id, object: 'chat.completion.chunk', created, model, choices: [{index: 0, delta, finish_reason}]});
      const sse = value => { if (!res.destroyed) { if (res.writableLength > 1024 * 1024) { abort.abort(); res.destroy(); return; } res.write(`data: ${typeof value === 'string' ? value : JSON.stringify(value)}\n\n`); } };
      let streaming = false;
      const begin = () => { if (!streaming && request.stream) { streaming = true; res.writeHead(200, {'Content-Type': 'text/event-stream; charset=utf-8', Connection: 'keep-alive', 'X-Bridge-Ignored-Parameters': request.ignored.join(',')}); sse(chunk({role: 'assistant', content: ''})); } };
      const push = value => { text += value; if (value && request.stream) { begin(); sse(chunk({content: value})); } };
      let result;
      try {
        result = await adapter.generate(request, {signal: abort.signal, delta: value => {
          push(filter.push(value)); if (filter.stopped) { stopReached = true; abort.abort(); }
        }});
      } catch (e) {
        if (!stopReached || res.destroyed) {
          abort.signal.aborted ? metrics.cancelled++ : metrics.failed++;
          if (!res.destroyed) { if (streaming) { sse(errorBody(e)); res.end(); } else json(e.status ?? 500, errorBody(e)); }
          return;
        }
      } finally { busy = false; res.off('close', disconnected); }
      if (res.destroyed) return;
      push(filter.push('', true)); metrics.completed++;
      metrics.last = {model: result?.model ?? request.model, firstTokenMs: result?.firstTokenMs ?? null, firstMessageMs: result?.firstMessageMs ?? null, processSpawnMs: result?.processSpawnMs ?? null, firstEventMs: result?.firstEventMs ?? null, elapsedMs: result?.elapsedMs ?? null, usage: result?.usage ?? null, ignoredParameters: request.ignored};
      if (request.stream) {
        begin(); sse(chunk({}, 'stop', result?.model ?? request.model));
        if (request.includeUsage && result?.usage) sse({id, object: 'chat.completion.chunk', created, model: result.model, choices: [], usage: result.usage});
        sse('[DONE]'); res.end();
      } else json(200, {id, object: 'chat.completion', created, model: result?.model ?? request.model, choices: [{index: 0, message: {role: 'assistant', content: text}, finish_reason: 'stop'}], ...(result?.usage ? {usage: result.usage} : {}), bridge: {ignored_parameters: request.ignored}});
    } catch (e) { if (!res.headersSent && !res.destroyed) json(e.status ?? 500, errorBody(e)); else res.end(); }
  });
  server.requestTimeout = 30000; server.headersTimeout = 10000;
  return server;
}
