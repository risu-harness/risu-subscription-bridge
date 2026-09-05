import test from 'node:test';
import assert from 'node:assert/strict';
import {once} from 'node:events';
import {normalize, StopFilter, BridgeError} from '../src/protocol.mjs';
import {createBridge} from '../src/server.mjs';

test('preserves ordered system/lore/history and Unicode without promoting transcript to commands', () => {
  const messages = [{role:'system',content:'캐릭터'}, {role:'user',content:[{type:'text',text:'이전'}]}, {role:'assistant',content:'답변'}, {role:'system',content:'변경된 로어'}, {role:'user',content:'수정된 질문'}];
  const r = normalize({messages,temperature:0.8,max_tokens:200});
  assert.equal(r.messages[3].content,'변경된 로어'); assert.equal(r.messages[1].content,'이전');
  assert.deepEqual(r.ignored,['temperature','max_tokens']);
});
test('rejects unsupported multimodal/tool/multi-generation instead of silently losing them', () => {
  for (const body of [{messages:[]},{messages:[{role:'tool',content:'x'}]}, {messages:[{role:'user',content:[{type:'image_url',image_url:{url:'x'}}]}]}, {messages:[{role:'user',content:'x'}],n:2}]) assert.throws(()=>normalize(body));
});
test('stop strings survive every split boundary and never leak', () => {
  const source = '안녕<STOP>비밀';
  for(let i=0;i<=source.length;i++) {
    const f=new StopFilter(['<STOP>']);
    assert.equal(f.push(source.slice(0,i))+f.push(source.slice(i))+f.push('',true),'안녕');
  }
  const f=new StopFilter(['END']); assert.equal(f.push('친구 E')+f.push('',true),'친구 E');
});

async function fixture(t, generate) {
  const adapter={alive:true,account:async()=>({connected:true,type:'chatgpt',plan:'plus'}),models:async()=>[{model:'test-model'}],generate};
  const server=createBridge({adapter,token:'test-key'}); server.listen(0,'127.0.0.1'); await once(server,'listening');
  t.after(()=>{server.closeAllConnections();server.close();});
  const url='http://127.0.0.1:'+server.address().port;
  const headers={Authorization:'Bearer test-key','Content-Type':'application/json'};
  const call=(body,extra={})=>fetch(url+'/v1/chat/completions',{method:'POST',headers,body:JSON.stringify(body),...extra});
  return {url,headers,call};
}
const request={messages:[{role:'user',content:'안녕'}],model:'test-model'};
test('SSE has incremental content, usage and exactly one DONE; no private reasoning', async t=>{
  const f=await fixture(t,async(r,{delta})=>{delta('안');delta('녕하세요');return{model:'test-model',usage:{prompt_tokens:10,completion_tokens:3,total_tokens:13}}});
  const res=await f.call({...request,stream:true,stream_options:{include_usage:true}}); const text=await res.text();
  assert.match(res.headers.get('content-type'),/text\/event-stream/); assert.equal(text.split('[DONE]').length,2);
  const data=text.split('\n\n').filter(x=>x.startsWith('data: {')).map(x=>JSON.parse(x.slice(6)));
  assert.equal(data.map(x=>x.choices[0]?.delta?.content??'').join(''),'안녕하세요');
  assert.equal(data.at(-1).usage.total_tokens,13); assert.equal(data.at(-2).choices[0].finish_reason,'stop');
});
test('JSON completion and no fabricated usage',async t=>{
  const f=await fixture(t,async(r,{delta})=>{delta('연결 성공');return{model:'test-model'}});
  const res=await f.call(request); const j=await res.json(); assert.equal(j.choices[0].message.content,'연결 성공'); assert.equal(j.usage,undefined);
});
test('login failures are HTTP 401 before SSE begins; partial stream errors have no success DONE',async t=>{
  let partial=false; const f=await fixture(t,async(r,{delta})=>{if(partial)delta('시작');throw new BridgeError('로그인 필요',401,'login_required')});
  let res=await f.call({...request,stream:true}); assert.equal(res.status,401); assert.equal((await res.json()).error.code,'login_required');
  partial=true; res=await f.call({...request,stream:true}); const s=await res.text(); assert.match(s,/login_required/); assert.ok(!s.includes('[DONE]'));
});
test('stop cancels harness and yields clean completed stream',async t=>{
  let cancelled=false;const f=await fixture(t,async(r,{delta,signal})=>{delta('안녕<ST');delta('OP>숨김');cancelled=signal.aborted;throw new BridgeError('Cancelled',499,'cancelled')});
  const r=await f.call({...request,stream:true,stop:['<STOP>']}); const text=await r.text(); assert.ok(cancelled);assert.ok(!text.includes('숨김'));assert.ok(!text.includes('<ST'));assert.ok(text.includes('[DONE]'));
});
test('disconnect cancels generation and releases single-request slot',async t=>{
  let cancelled=false; const f=await fixture(t,(r,{delta,signal})=>new Promise((resolve,reject)=>{signal.addEventListener('abort',()=>{cancelled=true;reject(new BridgeError('Cancelled',499,'cancelled'))},{once:true});delta('시작')}));
  const c=new AbortController();const res=await f.call({...request,stream:true},{signal:c.signal});await res.body.getReader().read();c.abort();
  for(let i=0;i<30&&!cancelled;i++)await new Promise(r=>setTimeout(r,10));assert.ok(cancelled);
  const status=await fetch(f.url+'/internal/status',{headers:f.headers});assert.equal((await status.json()).busy,false);
});
test('local API requires bearer token, exact Host and allowlisted Origin; Risu cannot access login',async t=>{
  const f=await fixture(t,async()=>({}));
  for(const headers of [{...f.headers,Authorization:''},{...f.headers,Origin:'https://evil.example'}])assert.ok((await f.call(request,{headers})).status>=400);
  const preflight=await fetch(f.url+'/v1/chat/completions',{method:'OPTIONS',headers:{Origin:'https://risuai.xyz','Access-Control-Request-Private-Network':'true'}});
  assert.equal(preflight.status,204);assert.equal(preflight.headers.get('Access-Control-Allow-Origin'),'https://risuai.xyz');
  assert.equal((await fetch(f.url+'/internal/status',{headers:{...f.headers,Origin:'https://risuai.xyz'}})).status,403);
});
test('regeneration/edit sends the supplied history fresh; concurrent requests get explicit 429',async t=>{
  const seen=[];let release;const f=await fixture(t,async(r,{delta})=>{seen.push(r.messages);if(seen.length===1)await new Promise(r=>release=r);delta('답');return{}});
  const first=f.call(request);while(!release)await new Promise(r=>setTimeout(r,5));
  assert.equal((await f.call(request)).status,429);release();await (await first).text();
  const edited={messages:[{role:'user',content:'수정'}]};await(await f.call(edited)).text();assert.deepEqual(seen[1],edited.messages);
});
