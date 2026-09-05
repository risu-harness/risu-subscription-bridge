import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {writeFile} from 'node:fs/promises';
import {join} from 'node:path';
import {validateEffort} from './settings.mjs';
import {BASE, BridgeError, promptFor} from './protocol.mjs';

// Generation is exclusively codex exec. The existing control object only handles
// login/account/model discovery; no character transcript is sent to App Server.
export class CodexExec {
  constructor({binary, env, cwd, control, spawnProcess = spawn, timeoutMs = 180000, writeInstructions = writeFile}) {
    Object.assign(this, {binary, env, cwd, control, spawnProcess, timeoutMs, writeInstructions});
    this.name = 'codex-exec'; this.delivery = 'completed-message'; this.children = new Set();
    this.instructionsPath = join(env.CODEX_HOME, 'bridge-instructions.txt');
  }
  get alive() { return this.control.alive; }
  async init() { await this.writeInstructions(this.instructionsPath, BASE, {mode: 0o600}); }
  account() { return this.control.account(); }
  models() { return this.control.models(); }
  login() { return this.control.login(); }
  async generate(request, {signal, delta}) {
    if (signal.aborted) throw new BridgeError('Cancelled.', 499, 'cancelled');
    if (!(await this.account()).connected) throw new BridgeError('Sign in with ChatGPT on the setup page.', 401, 'login_required');
    const models = await this.models();
    const chosen = request.model === 'subscription-default' ? models.find(m => m.isDefault) ?? models[0] : models.find(m => m.model === request.model || m.id === request.model);
    if (!chosen) throw new BridgeError('Choose a model returned by /v1/models.', 400, 'unknown_model');
    if (signal.aborted) throw new BridgeError('Cancelled.', 499, 'cancelled');
    validateEffort(chosen,request.effort);
    await this.writeInstructions(this.instructionsPath,BASE+(request.instructions?'\n\n'+request.instructions:''),{mode:0o600});
    const model = chosen.model ?? chosen.id;
    const args = ['exec', '--ephemeral', '--skip-git-repo-check', '--json', '-s', 'read-only', '-m', model,
      '-c', `model_instructions_file=${JSON.stringify(this.instructionsPath)}`,
      ...(request.effort?['-c',`model_reasoning_effort=${JSON.stringify(request.effort)}`]:[]),
      ...(request.verbosity?['-c',`model_verbosity=${JSON.stringify(request.verbosity)}`]:[]), '-'];
    const startedAt = performance.now();
    return new Promise((resolve, reject) => {
      let completed = false, failure, usage, firstMessageMs = null, processSpawnMs = null, firstEventMs = null, killTimer;
      const seen = new Set();
      const child = this.spawnProcess(this.binary, args, {env: this.env, cwd: this.cwd, stdio: ['pipe', 'pipe', 'pipe']});
      this.children.add(child);
      child.stderr.resume(); child.stdin.on('error', () => {});
      child.once('spawn', () => { processSpawnMs = Math.round(performance.now() - startedAt); });
      const terminate = error => {
        failure ??= error;
        child.kill('SIGTERM');
        killTimer ??= setTimeout(() => child.kill('SIGKILL'), 1500);
      };
      const abort = () => terminate(new BridgeError('Cancelled.', 499, 'cancelled'));
      const timer = setTimeout(() => terminate(new BridgeError('Generation exceeded time limit.', 504, 'generation_timeout')), this.timeoutMs);
      signal.addEventListener('abort', abort, {once: true});
      const lines = createInterface({input: child.stdout});
      lines.on('line', line => {
        if (failure) return;
        let event;
        try { event = JSON.parse(line); } catch { terminate(new BridgeError('Malformed CLI JSON output.', 502, 'cli_protocol')); return; }
        firstEventMs ??= Math.round(performance.now() - startedAt);
        if (event.type === 'item.completed' && event.item?.type === 'agent_message') {
          if (typeof event.item.text !== 'string') { terminate(new BridgeError('Invalid assistant message.', 502, 'cli_protocol')); return; }
          if (!seen.has(event.item.id)) {
            seen.add(event.item.id); firstMessageMs ??= Math.round(performance.now() - startedAt);
            delta(event.item.text);
          }
        }
        if (event.type === 'turn.completed') {
          completed = true;
          const u = event.usage;
          if (Number.isFinite(u?.input_tokens) && Number.isFinite(u?.output_tokens)) usage = {prompt_tokens: u.input_tokens, completion_tokens: u.output_tokens, total_tokens: u.input_tokens + u.output_tokens};
        }
        if (event.type === 'turn.failed' || event.type === 'error') {
          // Classify locally; never return raw errors that may embed transcript text.
          const message = event.error?.message ?? event.message ?? '';
          const limit = /rate.?limit|quota|usage limit/i.test(message);
          terminate(new BridgeError(limit ? 'Subscription limit reached. Wait for reset.' : 'Codex CLI generation failed.', limit ? 429 : 502, limit ? 'rate_limit' : 'cli_failed'));
        }
      });
      const clean = () => { clearTimeout(timer); clearTimeout(killTimer); signal.removeEventListener('abort', abort); this.children.delete(child); lines.close(); };
      child.once('error', () => { clean(); reject(new BridgeError('Cannot launch Codex CLI.', 503, 'cli_spawn')); });
      child.once('close', code => {
        clean();
        if (failure) return reject(failure);
        if (code !== 0 || !completed || !seen.size) return reject(new BridgeError('CLI exited without a complete answer.', 502, 'cli_incomplete'));
        resolve({model, usage, firstTokenMs: null, firstMessageMs, firstEventMs, processSpawnMs,
          elapsedMs: Math.round(performance.now() - startedAt), adapter: this.name, delivery: this.delivery});
      });
      if (signal.aborted) abort();
      else child.stdin.end(promptFor(request.messages));
    });
  }
  shutdown() {
    for (const child of this.children) { child.kill('SIGTERM'); const timer = setTimeout(() => child.kill('SIGKILL'), 1500); timer.unref(); }
    this.control.shutdown();
  }
}
