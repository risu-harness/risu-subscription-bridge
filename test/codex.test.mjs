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
test('handles notifications before turn/start reply, ignores other threads and reasoning, unloads ephemeral thread',async()=>{
  let text='';const a=new FakeCodex(async a=>{
    a.emit('notification',{method:'item/agentMessage/delta',params:{threadId:'other',delta:'wrong'}});
    a.event('item/reasoning/textDelta',{delta:'private'});
    a.event('item/agentMessage/delta',{delta:'hello'});
    a.event('thread/tokenUsage/updated',{tokenUsage:{last:{inputTokens:5,outputTokens:2,totalTokens:7}}});
    a.event('turn/completed',{turn:{id:'turn1',status:'completed'}});
  });
  const r=await a.generate(request,{signal:new AbortController().signal,delta:s=>text+=s});
  assert.equal(text,'hello');assert.equal(r.usage.total_tokens,7);
  assert.equal(a.calls[0].params.ephemeral,true);assert.equal(a.calls.at(-1).method,'thread/unload');
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
