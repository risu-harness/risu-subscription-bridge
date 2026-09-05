import test from 'node:test';
import assert from 'node:assert/strict';
import {Codex} from '../src/codex.mjs';
const request={model:'subscription-default',messages:[{role:'user',content:'Hi'}]};
class FakeCodex extends Codex {
  constructor(run) {super({cwd:'/empty'});this.run=run;this.calls=[];}
  async account(){return{connected:true,type:'chatgpt'};}
  async models(){return[{model:'actual-model',isDefault:true}];}
  async rpc(method,params){
    this.calls.push({method,params});
    if(method==='thread/start')return{thread:{id:'thread1'}};
    if(method==='turn/start'){await this.run(this);return{turn:{id:'turn1',status:'completed'}};}
    return{};
  }
  event(method,params){this.emit('notification',{method,params:{threadId:'thread1',...params}});}
}
test('handles notifications before turn/start reply, ignores other threads and reasoning, unsubscribes ephemeral thread',async()=>{
  let text='';const a=new FakeCodex(async a=>{
    a.emit('notification',{method:'item/agentMessage/delta',params:{threadId:'other',delta:'wrong'}});
    a.event('item/reasoning/textDelta',{delta:'private'});
    a.event('item/agentMessage/delta',{delta:'hello'});
    a.event('thread/tokenUsage/updated',{tokenUsage:{last:{inputTokens:5,outputTokens:2,totalTokens:7}}});
    a.event('turn/completed',{turn:{id:'turn1',status:'completed'}});
  });
  const r=await a.generate(request,{signal:new AbortController().signal,delta:s=>text+=s});
  assert.equal(text,'hello');assert.equal(r.usage.total_tokens,7);
  assert.equal(a.calls[0].params.ephemeral,true);assert.equal(a.calls.at(-1).method,'thread/unsubscribe');
  assert.equal(a.listenerCount('notification'),0);
});
test('abort before turn/start response still interrupts and cleans up',async()=>{
  const c=new AbortController();const a=new FakeCodex(async a=>{a.event('turn/started',{turn:{id:'turn1'}});c.abort();});
  await assert.rejects(a.generate(request,{signal:c.signal,delta:()=>{}}),e=>e.code==='cancelled');
  assert.ok(a.calls.some(c=>c.method==='turn/interrupt'));assert.equal(a.listenerCount('notification'),0);
});
test('harness death rejects generation instead of leaving a pending HTTP request',async()=>{
  const a=new FakeCodex(async a=>{a.emit('stopped');});
  await assert.rejects(a.generate(request,{signal:new AbortController().signal,delta:()=>{}}),e=>e.code==='harness_stopped');
});
test('streams before completion and creates a fresh ephemeral thread for each edited history',async()=>{
  let completed=false;const delivered=[];
  const a=new FakeCodex(async a=>{
    completed=false;
    a.event('item/agentMessage/delta',{delta:'one'});
    a.event('item/agentMessage/delta',{delta:'two'});
    completed=true;
    a.event('turn/completed',{turn:{id:'turn1',status:'completed'}});
  });
  const options={signal:new AbortController().signal,delta:s=>{assert.equal(completed,false);delivered.push(s);}};
  await a.generate(request,options);
  await a.generate({...request,messages:[{role:'user',content:'Edited message'}]},options);
  assert.deepEqual(delivered,['one','two','one','two']);
  const starts=a.calls.filter(c=>c.method==='thread/start');
  assert.equal(starts.length,2);assert.ok(starts.every(c=>c.params.ephemeral===true));
  assert.equal(a.calls.filter(c=>c.method==='thread/unsubscribe').length,2);
  const turns=a.calls.filter(c=>c.method==='turn/start');
  assert.match(turns[1].params.input[0].text,/Edited message/);
  assert.doesNotMatch(turns[1].params.input[0].text,/Hi/);
});
test('cleanup RPC failure stops harness rather than silently accumulating threads',async()=>{
  const a=new FakeCodex(async a=>a.event('turn/completed',{turn:{id:'turn1',status:'completed'}}));
  const rpc=a.rpc.bind(a);let shutdown=false;
  a.rpc=(method,params)=>method==='thread/unsubscribe'?Promise.reject(Error('RPC failed')):rpc(method,params);
  a.shutdown=()=>shutdown=true;
  await assert.rejects(a.generate(request,{signal:new AbortController().signal,delta:()=>{}}),e=>e.code==='cleanup_failed');
  assert.equal(shutdown,true);
});
