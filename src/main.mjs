import {mkdir, readFile, writeFile, chmod} from 'node:fs/promises';
import {randomBytes} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {resolve, join} from 'node:path';
import {existsSync} from 'node:fs';
import {Codex} from './codex.mjs';
import {CodexExec} from './exec.mjs';
import {spawn} from 'node:child_process';
import {createBridge} from './server.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const runtime = resolve(process.env.BRIDGE_DATA_DIR || join(root, '.runtime'));
const home = join(runtime, 'codex'), cwd = join(runtime, 'work');
await mkdir(home, {recursive: true, mode: 0o700}); await mkdir(cwd, {recursive: true, mode: 0o700}); await chmod(runtime, 0o700);
const bundled = '/Applications/ChatGPT.app/Contents/Resources/codex';
const binary = process.env.BRIDGE_CODEX_BIN || (existsSync(bundled) ? bundled : 'codex');
let token;
try { token = (await readFile(join(runtime, 'bridge-key'), 'utf8')).trim(); }
catch (e) { if (e.code !== 'ENOENT') throw e; token = randomBytes(24).toString('hex'); await writeFile(join(runtime, 'bridge-key'), token, {mode: 0o600}); }
// This dedicated home contains no imported user configuration, skills, or MCP servers.
const disabled = ['shell_tool','unified_exec','code_mode','code_mode_host','js_repl','apps','connectors','plugins','multi_agent','collab','memory_tool','memories','remote_control','browser_use','computer_use','image_generation','view_image','apply_patch_freeform','hooks','codex_hooks','plugin_hooks','tool_search','skill_search'];
await writeFile(join(home, 'config.toml'), `model_provider = "openai"\nforced_login_method = "chatgpt"\nsandbox_mode = "read-only"\napproval_policy = "never"\nweb_search = "disabled"\nproject_doc_max_bytes = 0\ninclude_environment_context = false\ninclude_apps_instructions = false\ninclude_collaboration_mode_instructions = false\ncli_auth_credentials_store = "file"\n[history]\npersistence = "none"\n[analytics]\nenabled = false\n[features]\n${disabled.map(k => `${k} = false`).join('\n')}\n`, {mode: 0o600});
const env = {...process.env, CODEX_HOME: home};
for (const k of ['OPENAI_API_KEY', 'OPENAI_BASE_URL', 'OPENAI_ORG_ID', 'OPENAI_ORGANIZATION', 'CODEX_API_KEY', 'CODEX_ACCESS_TOKEN']) delete env[k];
const control = new Codex({binary, env, cwd});
await control.init();
const backend = process.env.BRIDGE_ADAPTER || 'exec';
if (!['exec', 'app-server'].includes(backend)) { control.shutdown(); throw Error('BRIDGE_ADAPTER must be exec or app-server'); }
const adapter = backend === 'exec' ? new CodexExec({binary, env, cwd, control}) : control;
if (backend === 'exec') await adapter.init();
const port = Number(process.env.BRIDGE_PORT || 8787);
const origins = process.env.BRIDGE_ALLOWED_ORIGINS?.split(',').map(x => x.trim()).filter(Boolean);
const server = createBridge({adapter, token, port, runtime, ...(origins ? {origins} : {})});
server.on('error', e => { console.error('Bridge listener failed:', e.code); adapter.shutdown(); process.exitCode = 1; });
server.listen(port, '127.0.0.1', () => {
  const setupURL = `http://127.0.0.1:${port}/#key=${token}`;
  console.log(`Risu bridge listening on http://127.0.0.1:${port}\nSetup: ${setupURL}\nNo prompts or credentials are written to bridge logs. The setup URL contains a local-only access key.`);
  if (process.env.BRIDGE_OPEN_BROWSER === '1' && process.platform === 'darwin') {
    const opener = spawn('/usr/bin/open', [setupURL], {stdio: 'ignore'});
    opener.once('error', () => console.error('브라우저를 열 수 없습니다. 위 Setup 주소를 직접 열어 주세요.'));
  }
});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => { server.close(); server.closeAllConnections(); adapter.shutdown(); });
