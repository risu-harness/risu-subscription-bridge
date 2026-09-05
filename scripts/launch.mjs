import {readFile} from 'node:fs/promises';
import {join} from 'node:path';
import net from 'node:net';
import {spawn} from 'node:child_process';
import {findInstance} from '../src/instance.mjs';
const dir=process.argv[2];
const cfg=JSON.parse(await readFile(join(dir,'install.json'),'utf8'));
const runtime=join(dir,'data');
const requested=Number(process.env.BRIDGE_PORT||8787);
if(!Number.isInteger(requested)||requested<1024||requested>65535)throw Error('Invalid BRIDGE_PORT');
const ports=[...new Set([requested,...Array.from({length:13},(_,i)=>8787+i)])];
async function reuse() {
  const existing=await findInstance(runtime,ports);
  if(!existing)return false;
  console.log(`이미 실행 중인 브리지를 사용합니다: http://127.0.0.1:${existing.port}/`);
  if(process.env.BRIDGE_OPEN_BROWSER!=='0'&&process.platform==='darwin') {
    const opener=spawn('/usr/bin/open',[`http://127.0.0.1:${existing.port}/#key=${existing.token}`],{stdio:'ignore'});
    opener.on('error',()=>console.error('브라우저를 열지 못했습니다. 기존 설정 페이지를 사용하세요.'));
  }
  return true;
}
async function available(port){return new Promise(resolve=>{const s=net.createServer();s.once('error',()=>resolve(false));s.listen(port,'127.0.0.1',()=>s.close(()=>resolve(true)));});}
if(!await reuse()) {
  let port=requested;
  if(process.env.BRIDGE_PORT){if(!await available(port))throw Error(`Port ${port} is already in use.`);}
  else {while(port<8800&&!await available(port))port++;if(port===8800)throw Error('No available port from 8787 to 8799.');}
  console.log(`브리지 시작 · http://127.0.0.1:${port} · CLI wrapper`);
  const child=spawn(cfg.node,[join(cfg.source,'src','main.mjs')],{cwd:cfg.source,stdio:'inherit',env:{...process.env,BRIDGE_CODEX_BIN:cfg.codex,BRIDGE_DATA_DIR:runtime,BRIDGE_PORT:String(port),BRIDGE_ADAPTER:'exec',BRIDGE_OPEN_BROWSER:process.env.BRIDGE_OPEN_BROWSER??'1'}});
  const handlers=new Map(['SIGINT','SIGTERM','SIGHUP'].map(signal=>[signal,()=>child.kill(signal==='SIGHUP'?'SIGTERM':signal)]));
  for(const [signal,handler] of handlers)process.on(signal,handler);
  const code=await new Promise((ok,fail)=>{child.once('error',fail);child.once('exit',code=>ok(code));});
  for(const [signal,handler] of handlers)process.off(signal,handler);
  if(code===75) {
    let found=false;
    for(let i=0;i<20&&!found;i++) { await new Promise(r=>setTimeout(r,500)); found=await reuse(); }
    if(!found)throw Error('다른 브리지가 시작 중이거나 인스턴스 잠금 포트가 사용 중입니다. 잠시 후 같은 명령을 다시 실행하세요.');
  } else process.exitCode=code??0;
}
