import test from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {PassThrough} from 'node:stream';
import {CodexExec} from '../src/exec.mjs';

function fixture(run) {
  let captured;
  const child = new EventEmitter(); child.stdin=new PassThrough();child.stdout=new PassThrough();child.stderr=new PassThrough();
  child.kill=()=>{child.stdout.end();queueMicrotask(()=>child.emit('close',null));return true;};
  const emit=e=>child.stdout.write(JSON.stringify(e)+'\n');
  const spawnProcess=(binary,args,options)=>{captured={binary,args,options};queueMicrotask(()=>{child.emit('spawn');run({child,emit});});return child;};
  const control={alive:true,account:async()=>({connected:true}),models:async()=>[{model:'test-model',isDefault:true}],shutdown(){}};
  return {adapter:new CodexExec({binary:'codex',env:{CODEX_HOME:'/tmp/example'},cwd:'/empty',control,spawnProcess}),child,get captured(){return captured;}};
}
const req={model:'subscription-default',messages:[{role:'system',content:'한국어 설정'},{role:'assistant',content:'이전 답변'},{role:'user',content:'새 질문'}]};
test('exec uses stdin/ephemeral; emits completed message once and excludes reasoning',async()=>{
  const f=fixture(({child,emit})=>{emit({type:'thread.started',thread_id:'abc'});emit({type:'item.completed',item:{id:'r',type:'reasoning',text:'secret'}});emit({type:'item.completed',item:{id:'a',type:'agent_message',text:'안녕'}});emit({type:'item.completed',item:{id:'a',type:'agent_message',text:'안녕'}});emit({type:'turn.completed',usage:{input_tokens:12,output_tokens:3}});child.stdout.end();setImmediate(()=>child.emit('close',0));});
  let text='';const r=await f.adapter.generate(req,{signal:new AbortController().signal,delta:s=>text+=s});
  assert.equal(text,'안녕');assert.equal(r.firstTokenMs,null);assert.equal(r.delivery,'completed-message');assert.equal(r.usage.total_tokens,15);
  assert.ok(f.captured.args.includes('--ephemeral'));assert.ok(!f.captured.args.join(' ').includes('새 질문'));
  assert.match(f.child.stdin.read().toString(),/한국어 설정/);assert.equal(f.adapter.children.size,0);
});
test('exec cancellation waits for process close and rejects',async()=>{
  const c=new AbortController();const f=fixture(()=>c.abort());
  await assert.rejects(f.adapter.generate(req,{signal:c.signal,delta(){}}),e=>e.code==='cancelled');assert.equal(f.adapter.children.size,0);
});
test('exec fails closed on malformed output, incomplete success and quota failure',async()=>{
  for(const kind of ['malformed','incomplete','quota']) {
    const f=fixture(({child,emit})=>{if(kind==='malformed')child.stdout.write('not json\n');if(kind==='quota')emit({type:'turn.failed',error:{message:'usage limit secret prompt'}});if(kind==='incomplete'){child.stdout.end();setImmediate(()=>child.emit('close',0));}});
    await assert.rejects(f.adapter.generate(req,{signal:new AbortController().signal,delta(){}}),e=>e.code===({malformed:'cli_protocol',incomplete:'cli_incomplete',quota:'rate_limit'})[kind]&&!e.message.includes('secret'));
  }
});
