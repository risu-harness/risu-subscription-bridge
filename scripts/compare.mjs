// P0: the same normalized transcript through App Server and codex exec.
// One real generation per route. No transcript is saved by this script.
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {readFile, writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {join} from 'node:path';
import {normalize, promptFor, BASE} from '../src/protocol.mjs';
import {Codex} from '../src/codex.mjs';

const runtime=fileURLToPath(new URL('../.runtime/',import.meta.url));
const token=(await readFile(join(runtime,'bridge-key'),'utf8')).trim();
const base=process.env.BRIDGE_URL||'http://127.0.0.1:8787';
const headers={Authorization:`Bearer ${token}`,'Content-Type':'application/json'};
const status=await(await fetch(base+'/internal/status',{headers})).json();
if(!status.account?.connected){console.log('LOGIN_REQUIRED');process.exit(2);}
const body=process.argv[2]?JSON.parse(await readFile(process.argv[2],'utf8')):{model:'subscription-default',messages:[{role:'system',content:'당신은 허구의 성인 캐릭터 강서우입니다. 남편을 맞아 한국어로 한두 문장만 답하세요. 사용자의 행동을 대신 쓰지 마세요.'},{role:'user',content:'서우야 나 왔어. 오늘은 일찍 끝났어.'}]};
const normalized=normalize(body);
const env={...process.env,CODEX_HOME:join(runtime,'codex')};
for(const k of ['OPENAI_API_KEY','OPENAI_BASE_URL','CODEX_API_KEY','CODEX_ACCESS_TOKEN'])delete env[k];
const binary=process.env.BRIDGE_CODEX_BIN||'/Applications/ChatGPT.app/Contents/Resources/codex';
const control=new Codex({binary,env,cwd:join(runtime,'work')});
await control.init();
let app,appText='';
try {app=await control.generate(normalized,{signal:new AbortController().signal,delta:s=>appText+=s});}
finally {control.shutdown();}
console.log(JSON.stringify({adapter:'app-server',...app,answer:appText}));
await writeFile(join(runtime,'base-instructions.txt'),BASE,{mode:0o600});
const cliStart=performance.now();let failed=false,usage,answer='',firstMessageMs;
const child=spawn(binary,['exec','--ephemeral','--skip-git-repo-check','--json','-s','read-only','-m',app.model,'-c',`model_instructions_file=${JSON.stringify(join(runtime,'base-instructions.txt'))}`,'-'],{cwd:join(runtime,'work'),env,stdio:['pipe','pipe','pipe']});
child.stderr.resume();child.stdin.on('error',()=>{});
child.once('error',()=>{console.error('CLI launch failed');failed=true;process.exitCode=1;});
const timeout=setTimeout(()=>{failed=true;child.kill('SIGTERM');},180000);
createInterface({input:child.stdout}).on('line',line=>{
  let event;try{event=JSON.parse(line);}catch{return;}
  if(event.type==='item.completed'&&event.item?.type==='agent_message'){answer+=event.item.text;firstMessageMs??=Math.round(performance.now()-cliStart);}
  if(event.type==='turn.completed')usage=event.usage;
  if(event.type==='turn.failed'||event.type==='error')failed=true;
});
child.stdin.end(promptFor(normalized.messages));
child.once('close',code=>{clearTimeout(timeout);console.log(JSON.stringify({adapter:'codex-exec',elapsedMs:Math.round(performance.now()-cliStart),firstMessageMs,model:app.model,usage,answer,exitCode:code}));if(code!==0||failed)process.exitCode=1;});
