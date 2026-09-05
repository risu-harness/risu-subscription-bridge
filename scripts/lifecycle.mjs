import {openSync,createReadStream,createWriteStream} from 'node:fs';
import {createInterface} from 'node:readline/promises';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {resolve,sep} from 'node:path';
import net from 'node:net';
const exec=promisify(execFile);
export async function chooseAction() {
  if(process.env.BRIDGE_ACTION) {
    if(!['reuse','stop','restart'].includes(process.env.BRIDGE_ACTION))throw Error('BRIDGE_ACTION: reuse, stop, restart');
    return process.env.BRIDGE_ACTION;
  }
  let input,output,rl;
  try {
    // stdin contains the curl script, so read the controlling terminal instead.
    input=createReadStream(null,{fd:openSync('/dev/tty','r'),autoClose:true});
    output=createWriteStream(null,{fd:openSync('/dev/tty','w'),autoClose:true});
    rl=createInterface({input,output});
  } catch {input?.destroy();output?.destroy();console.log('대화형 터미널이 없어 기존 브리지를 재사용합니다.');return 'reuse';}
  try {
    for(;;) {
      const answer=(await rl.question('\n1) 그대로 재사용 (기본값)\n2) 종료만 하기\n3) 종료 후 최신 버전으로 재시작\n종료·재시작 시 진행 중인 응답은 중단됩니다.\n선택 [1/2/3]: ')).trim();
      if(answer===''||answer==='1')return 'reuse';
      if(answer==='2')return 'stop';
      if(answer==='3')return 'restart';
    }
  } catch {return 'reuse';}
  finally {rl.close();input.destroy();output.end();}
}
export async function available(port) {return new Promise(r=>{const s=net.createServer();s.once('error',()=>r(false));s.listen(port,'127.0.0.1',()=>s.close(()=>r(true)));});}
export async function stopInstance(existing,dir) {
  const response=await fetch(`http://127.0.0.1:${existing.port}/internal/stop`,{method:'POST',headers:{Authorization:`Bearer ${existing.token}`,'Content-Type':'application/json'},body:'{}',signal:AbortSignal.timeout(5000)});
  if(response.status===404&&process.platform==='darwin') {
    // One-time migration for old releases without the authenticated stop route.
    // findInstance already verified the key and runtime. Only signal a listener
    // whose working directory belongs to this installation's release tree.
    const {stdout}=await exec('/usr/sbin/lsof',['-nP',`-iTCP:${existing.port}`,'-sTCP:LISTEN','-Fp']);
    const pids=[...new Set(stdout.split('\n').filter(x=>/^p\d+$/.test(x)).map(x=>Number(x.slice(1))))];
    if(pids.length!==1)throw Error('기존 프로세스를 안전하게 식별하지 못했습니다.');
    const {stdout:cwd}=await exec('/usr/sbin/lsof',['-a','-p',String(pids[0]),'-d','cwd','-Fn']);
    const path=cwd.split('\n').find(x=>x.startsWith('n'))?.slice(1);
    if(!path||!resolve(path).startsWith(resolve(dir,'releases')+sep))throw Error('설치 경로가 다른 프로세스는 종료하지 않습니다.');
    process.kill(pids[0],'SIGTERM');
  } else if(!response.ok)throw Error(`브리지 종료 실패 (${response.status})`);
  for(let i=0;i<100;i++){if(await available(existing.port))return;await new Promise(r=>setTimeout(r,100));}
  throw Error('기존 브리지가 아직 종료되지 않았습니다. 중복 실행하지 않습니다.');
}
