// Uses only the isolated bridge account. Does not read the user's usual Codex credentials.
import {readFile} from 'node:fs/promises';
const base=process.env.BRIDGE_URL||'http://127.0.0.1:8787';
const token=(await readFile(new URL('../.runtime/bridge-key',import.meta.url),'utf8')).trim();
const headers={Authorization:`Bearer ${token}`,'Content-Type':'application/json'};
const status=await(await fetch(base+'/internal/status',{headers})).json();
if(!status.account?.connected){console.log('LOGIN_REQUIRED: Open the setup URL printed by npm start and sign in with ChatGPT.');process.exitCode=2;}
else {
  const started=performance.now();
  const res=await fetch(base+'/v1/chat/completions',{method:'POST',headers,body:JSON.stringify({model:'subscription-default',stream:true,stream_options:{include_usage:true},messages:[{role:'system',content:'당신은 허구의 성인 캐릭터 강서우입니다. 남편에게 한국어로 다정하게 한두 문장만 말합니다. 행동은 별표로 표시합니다. 사용자의 행동을 대신 쓰지 마세요.'},{role:'user',content:'서우야, 나 왔어. 오늘은 일찍 끝났어.'}]})});
  console.log('HTTP',res.status);console.log(await res.text());console.log('Elapsed ms',Math.round(performance.now()-started));
  if(!res.ok)process.exitCode=1;
}
