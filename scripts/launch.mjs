import {readFile} from 'node:fs/promises';
import {join} from 'node:path';
import net from 'node:net';
import {spawn} from 'node:child_process';
const dir=process.argv[2];
const cfg=JSON.parse(await readFile(join(dir,'install.json'),'utf8'));
async function available(port){return new Promise(resolve=>{const s=net.createServer();s.once('error',()=>resolve(false));s.listen(port,'127.0.0.1',()=>s.close(()=>resolve(true)));});}
let port=Number(process.env.BRIDGE_PORT||8787);
if(!Number.isInteger(port)||port<1024||port>65535)throw Error('Invalid BRIDGE_PORT');
if(process.env.BRIDGE_PORT){if(!await available(port))throw Error(`Port ${port} is already in use.`);}
else {while(port<8800&&!await available(port))port++;if(port===8800)throw Error('No available port from 8787 to 8799.');}
console.log(`브리지 시작 · http://127.0.0.1:${port} · CLI wrapper\n로그인은 브라우저에서 직접 진행하세요.`);
const child=spawn(cfg.node,[join(cfg.source,'src','main.mjs')],{cwd:cfg.source,stdio:'inherit',env:{...process.env,BRIDGE_CODEX_BIN:cfg.codex,BRIDGE_DATA_DIR:join(dir,'data'),BRIDGE_PORT:String(port),BRIDGE_ADAPTER:'exec',BRIDGE_OPEN_BROWSER:process.env.BRIDGE_OPEN_BROWSER??'1'}});
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>child.kill(signal));
child.once('error',e=>{console.error(e.message);process.exitCode=1;});
child.once('exit',code=>{process.exitCode=code??0;});
