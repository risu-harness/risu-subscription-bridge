import {readFile} from 'node:fs/promises';
import {join} from 'node:path';
import {chooseAction,stopInstance,available} from './lifecycle.mjs';
import {spawn} from 'node:child_process';
import {findInstance} from '../src/instance.mjs';
import {parseOptions,helpText} from '../src/options.mjs';
const options=parseOptions(process.argv.slice(3));
if(options.help){console.log(helpText);process.exit(0);}
const dir=process.argv[2];
const cfg=JSON.parse(await readFile(join(dir,'install.json'),'utf8'));
const runtime=join(dir,'data');
const requested=Number(process.env.BRIDGE_PORT||8787);
if(!Number.isInteger(requested)||requested<1024||requested>65535)throw Error('Invalid BRIDGE_PORT');
const ports=[...new Set([requested,...Array.from({length:13},(_,i)=>8787+i)])];
async function reuse() {
  const existing=await findInstance(runtime,ports);
  if(!existing)return false;
  console.log(`생성 방식: ${existing.adapter || 'unknown'}`);
  const setupURL=`http://127.0.0.1:${existing.port}/#key=${existing.token}`;
  console.log(`이미 실행 중인 브리지를 사용합니다: http://127.0.0.1:${existing.port}/\n설정 페이지: ${setupURL}\n이 링크에는 로컬 연결 키가 포함되어 있습니다.`);
  if(process.env.BRIDGE_OPEN_BROWSER!=='0'&&process.platform==='darwin') {
    const opener=spawn('/usr/bin/open',[setupURL],{stdio:'ignore'});
    opener.on('error',()=>console.error('브라우저를 열지 못했습니다. 위 설정 페이지 링크로 접속하세요.'));
  }
  return true;
}
const existing=await findInstance(runtime,ports);
let start=!existing;
let port=requested;
if(existing) {
  console.log(`실행 중인 브리지: http://127.0.0.1:${existing.port}/`);
  let action=options.action || await chooseAction();
  if(action==='reuse' && ['exec','codex-exec'].includes(existing.adapter)) { console.log('기존 CLI 브리지를 App Server로 업그레이드합니다.'); action='restart'; }
  if(action==='reuse')await reuse();
  else {
    await stopInstance(existing,dir);
    console.log('브리지를 종료했습니다.');
    start=action==='restart';port=existing.port;
  }
}
if(start) {
  if(existing||process.env.BRIDGE_PORT){if(!await available(port))throw Error(`Port ${port} is already in use.`);}
  else {while(port<8800&&!await available(port))port++;if(port===8800)throw Error('No available port from 8787 to 8799.');}
  console.log(`브리지 시작 · http://127.0.0.1:${port} · App Server`);
  const child=spawn(cfg.node,[join(cfg.source,'src','main.mjs')],{cwd:cfg.source,stdio:'inherit',env:{...process.env,BRIDGE_CODEX_BIN:cfg.codex,BRIDGE_DATA_DIR:runtime,BRIDGE_PORT:String(port),BRIDGE_OPEN_BROWSER:process.env.BRIDGE_OPEN_BROWSER??'1'}});
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
