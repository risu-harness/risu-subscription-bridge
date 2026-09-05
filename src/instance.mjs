import net from 'node:net';
import {createHash} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import {join, resolve} from 'node:path';

// The bridge itself owns this OS lock. It survives launcher exit, and the OS
// releases it even after SIGKILL. Hash collisions fail closed, never spawn twice.
export async function acquireInstance(runtime) {
  const port = 30000 + createHash('sha256').update(resolve(runtime)).digest().readUInt32BE(0) % 20000;
  const guard = net.createServer(socket => socket.destroy());
  await new Promise((ok, fail) => { guard.once('error', fail); guard.listen(port, '127.0.0.1', ok); });
  return guard;
}

export async function findInstance(runtime, ports) {
  let token;
  try { token = (await readFile(join(runtime, 'bridge-key'), 'utf8')).trim(); }
  catch (e) { if (e.code === 'ENOENT') return null; throw e; }
  const results = await Promise.all(ports.map(async port => {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/internal/status`, {
        headers: {Authorization: `Bearer ${token}`}, signal: AbortSignal.timeout(2500),
      });
      if (!response.ok) return null;
      const status = await response.json();
      return resolve(status.runtime || '/') === resolve(runtime) ? {port, token, adapter: status.adapter} : null;
    } catch { return null; }
  }));
  return results.find(Boolean) ?? null;
}
