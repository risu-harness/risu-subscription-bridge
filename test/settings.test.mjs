import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {Settings,configuredAdapter} from '../src/settings.mjs';
test('settings persist, validate model effort, route backends and preserve explicit Risu model',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'risu-settings-'));
 try {
 const s=new Settings(dir);await s.init();
 const value={backend:'app-server',model:'m',effort:'low',verbosity:'high',instructions:'말투'};
 const models=[{model:'m',supportedReasoningEfforts:[{reasoningEffort:'low'}]}];
 await s.save(value,models);const loaded=new Settings(dir);await loaded.init();assert.deepEqual(loaded.value,value);
 await assert.rejects(s.save({...value,effort:'bogus'},models));assert.deepEqual(s.value,value);
 const control={generate:r=>({backend:'app-server',...r})},cli={generate:r=>({backend:'exec',...r})};
 const a=configuredAdapter(control,cli,s);
 assert.equal(a.generate({model:'subscription-default'}).model,'m');
 assert.equal(a.generate({model:'explicit'}).model,'explicit');
 assert.equal(a.generate({}).effort,'low');
 await s.save({...value,backend:'exec'},models);assert.equal(a.generate({}).backend,'exec');
 }finally{await rm(dir,{recursive:true});}
});
