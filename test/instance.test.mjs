import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,rm,mkdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import http from 'node:http';
import {acquireInstance,findInstance} from '../src/instance.mjs';
test('concurrent starts have one owner and closing permits restart',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'risu-instance-'));
 const results=await Promise.allSettled([acquireInstance(dir),acquireInstance(dir),acquireInstance(dir)]);
 assert.equal(results.filter(x=>x.status==='fulfilled').length,1);
 assert.ok(results.filter(x=>x.status==='rejected').every(x=>x.reason.code==='EADDRINUSE'));
 await new Promise(r=>results.find(x=>x.status==='fulfilled').value.close(r));
 const next=await acquireInstance(dir);await new Promise(r=>next.close(r));await rm(dir,{recursive:true});
});
test('SIGKILL releases ownership',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'risu-crash-'));
 const module=new URL('../src/instance.mjs',import.meta.url).href;
 const child=spawn(process.execPath,['--input-type=module','-e',`import {acquireInstance} from ${JSON.stringify(module)}; await acquireInstance(${JSON.stringify(dir)}); console.log('ready');`],{stdio:['ignore','pipe','inherit']});
 try {await once(child.stdout,'data');await assert.rejects(acquireInstance(dir),{code:'EADDRINUSE'});
 const exited=once(child,'exit');child.kill('SIGKILL');await exited;
 const next=await acquireInstance(dir);await new Promise(r=>next.close(r));
 } finally {child.kill('SIGKILL');await rm(dir,{recursive:true});}
});
test('parallel launchers reuse authenticated runtime without spawning harness',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'risu-reuse-'));const runtime=join(dir,'data');await mkdir(runtime);
 await writeFile(join(runtime,'bridge-key'),'test-key');
 let wrongRuntime=false;
 const server=http.createServer((req,res)=>{
 res.setHeader('Content-Type','application/json');
 if(req.headers.authorization!=='Bearer test-key'){res.writeHead(401);res.end('{}');return;}
 res.end(JSON.stringify({runtime:wrongRuntime?'/different':runtime,adapter:'app-server'}));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const port=server.address().port;
 try {assert.equal((await findInstance(runtime,[port])).port,port);
 await writeFile(join(dir,'install.json'),JSON.stringify({node:'/nonexistent',source:'/nonexistent',codex:'/nonexistent'}));
 const launch=new URL('../scripts/launch.mjs',import.meta.url);
 const children=Array.from({length:3},()=>spawn(process.execPath,[launch.pathname,dir],{env:{...process.env,BRIDGE_PORT:String(port),BRIDGE_OPEN_BROWSER:'0',BRIDGE_ACTION:'reuse'},stdio:'ignore'}));
 const codes=await Promise.all(children.map(c=>once(c,'exit')));assert.ok(codes.every(([code])=>code===0));
 wrongRuntime=true;assert.equal(await findInstance(runtime,[port]),null);
 wrongRuntime=false;await writeFile(join(runtime,'bridge-key'),'wrong');assert.equal(await findInstance(runtime,[port]),null);
 }finally{await new Promise(r=>server.close(r));await rm(dir,{recursive:true});}
});
test('stop route requires local origin and token, and releases listener',async()=>{
 const {createBridge}=await import('../src/server.mjs');
 const {stopInstance}=await import('../scripts/lifecycle.mjs');
 let stopped=false;
 const server=createBridge({adapter:{alive:true},token:'secret',shutdown:()=>{stopped=true;server.close();server.closeAllConnections();}});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const port=server.address().port;
 const url=`http://127.0.0.1:${port}/internal/stop`;
 try {
  assert.equal((await fetch(url,{method:'POST'})).status,401);
  assert.equal((await fetch(url,{method:'POST',headers:{Authorization:'Bearer secret',Origin:'https://risuai.xyz'}})).status,403);
  assert.equal(stopped,false);
  await stopInstance({port,token:'secret'},'/unused');assert.equal(stopped,true);
 }finally{server.close();server.closeAllConnections();}
});
