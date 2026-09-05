import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {EventEmitter} from 'node:events';
import {BridgeError, BASE, promptFor} from './protocol.mjs';

export class Codex extends EventEmitter {
  constructor({binary, env, cwd}) { super(); Object.assign(this, {binary, env, cwd}); this.pending = new Map(); this.seq = 0; }
  async init() {
    this.child = spawn(this.binary, ['app-server', '--listen', 'stdio://'], {env: this.env, cwd: this.cwd, stdio: ['pipe', 'pipe', 'pipe']});
    // Never forward raw harness logs: they may include transcript fragments or credentials.
    this.child.stderr.resume();
    this.child.stdin.on('error', () => {});
    createInterface({input: this.child.stdout}).on('line', line => {
      let m; try { m = JSON.parse(line); } catch { return; }
      if (m.id !== undefined && m.method) {
        // No model-initiated approvals, tool calls, or interactions are allowed.
        this.send({id: m.id, error: {code: -32601, message: 'Interactive tools are disabled by this client.'}});
      } else if (m.id !== undefined) {
        const p = this.pending.get(m.id); if (!p) return;
        clearTimeout(p.timer); this.pending.delete(m.id);
        m.error ? p.reject(new BridgeError(`Codex RPC failed (${m.error.code ?? 'unknown'}).`, 502, 'rpc_error')) : p.resolve(m.result);
      } else if (m.method) this.emit('notification', m);
    });
    const fail = () => {
      this.alive = false;
      for (const p of this.pending.values()) { clearTimeout(p.timer); p.reject(new BridgeError('Codex stopped. Restart the bridge.', 503, 'harness_stopped')); }
      this.pending.clear(); this.emit('stopped');
    };
    this.child.once('error', fail); this.child.once('exit', fail); this.alive = true;
    await this.rpc('initialize', {clientInfo: {name: 'risu_subscription_bridge', title: 'Risu Subscription Bridge', version: '0.1.0'}, capabilities: {experimentalApi: true}});
    this.send({method: 'initialized', params: {}});
  }
  send(m) { if (this.child?.stdin.writable) this.child.stdin.write(JSON.stringify(m) + '\n'); }
  rpc(method, params = {}, timeout = 30000) {
    if (!this.alive) return Promise.reject(new BridgeError('Codex is unavailable.', 503, 'harness_stopped'));
    const id = ++this.seq;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new BridgeError('Codex request timed out.', 504, 'rpc_timeout')); }, timeout);
      this.pending.set(id, {resolve, reject, timer}); this.send({id, method, params});
    });
  }
  async account() {
    const {account} = await this.rpc('account/read', {refreshToken: false});
    return {connected: account?.type === 'chatgpt', type: account?.type ?? null, plan: account?.planType ?? null};
  }
  login() { return this.rpc('account/login/start', {type: 'chatgpt'}); }
  async models() {
    let cursor; const models = [];
    do { const r = await this.rpc('model/list', {limit: 100, ...(cursor ? {cursor} : {})}); models.push(...r.data); cursor = r.nextCursor; } while (cursor);
    return models;
  }
  async generate(request, {signal, delta}) {
    if (signal.aborted) throw new BridgeError('Cancelled.', 499, 'cancelled');
    const account = await this.account();
    if (!account.connected) throw new BridgeError('Sign in with ChatGPT on the local setup page. API-key accounts are not accepted.', 401, 'login_required');
    const available = await this.models();
    const chosen = request.model === 'subscription-default' ? available.find(m => m.isDefault) ?? available[0] : available.find(m => m.model === request.model || m.id === request.model);
    if (!chosen) throw new BridgeError('Choose a model returned by /v1/models.', 400, 'unknown_model');
    const model = chosen.model ?? chosen.id;
    const {thread} = await this.rpc('thread/start', {model, cwd: this.cwd, ephemeral: true, sandbox: 'read-only', approvalPolicy: 'never', baseInstructions: BASE});
    let turnId, timer, usage, settled = false, startedAt = performance.now(), firstTokenMs = null;
    let resolveDone, rejectDone;
    const done = new Promise((resolve, reject) => { resolveDone = resolve; rejectDone = reject; });
    // Attach a rejection handler immediately while turn/start is still in flight.
    done.catch(() => {});
    const settle = error => { if (settled) return; settled = true; error ? rejectDone(error) : resolveDone(); };
    const interrupt = () => { if (turnId) this.rpc('turn/interrupt', {threadId: thread.id, turnId}, 5000).catch(() => {}); };
    const abort = () => { interrupt(); settle(new BridgeError('Cancelled.', 499, 'cancelled')); };
    const stopped = () => settle(new BridgeError('Codex stopped during generation.', 503, 'harness_stopped'));
    const listen = ({method, params: p}) => {
      if (p?.threadId !== thread.id || settled) return;
      if (method === 'turn/started') { turnId = p.turn.id; if (signal.aborted) interrupt(); }
      if (method === 'item/agentMessage/delta') { firstTokenMs ??= Math.round(performance.now() - startedAt); delta(p.delta); }
      if (method === 'thread/tokenUsage/updated') {
        const u = p.tokenUsage?.last;
        if (u) usage = {prompt_tokens: u.inputTokens, completion_tokens: u.outputTokens, total_tokens: u.totalTokens};
      }
      if (method === 'turn/completed') {
        turnId ??= p.turn.id;
        if (p.turn.status === 'completed') settle();
        else {
          const limit = /limit|quota|usage/i.test(JSON.stringify(p.turn.error?.codexErrorInfo ?? ''));
          settle(new BridgeError(limit ? 'Subscription limit reached. Wait for reset.' : `Generation ${p.turn.status}.`, limit ? 429 : 502, limit ? 'rate_limit' : 'generation_failed'));
        }
      }
    };
    this.on('notification', listen); this.once('stopped', stopped); signal.addEventListener('abort', abort, {once: true});
    timer = setTimeout(() => { interrupt(); settle(new BridgeError('Generation exceeded 180 seconds.', 504, 'generation_timeout')); }, 180000);
    try {
      if (signal.aborted) abort();
      else {
        const r = await this.rpc('turn/start', {threadId: thread.id, input: [{type: 'text', text: promptFor(request.messages), text_elements: []}]});
        turnId = r.turn.id; if (signal.aborted || settled && r.turn.status === 'inProgress') interrupt();
      }
      await done;
      return {model, usage, firstTokenMs, elapsedMs: Math.round(performance.now() - startedAt)};
    } finally {
      clearTimeout(timer); this.off('notification', listen); this.off('stopped', stopped); signal.removeEventListener('abort', abort);
      await this.rpc('thread/unload', {threadId: thread.id}, 5000).catch(() => {});
    }
  }
  shutdown() { this.child?.stdin.end(); const child = this.child; const t = setTimeout(() => child?.kill('SIGTERM'), 1500); t.unref(); }
}
