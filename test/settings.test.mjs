import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {Settings,configuredAdapter} from '../src/settings.mjs';
test('settings persist, validate model effort, migrate legacy settings and preserve explicit Risu model',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'risu-settings-'));
 try {
 const s=new Settings(dir);await s.init();
 const value={model:'m',effort:'low',verbosity:'high',instructions:'말투'};
 const models=[{model:'m',supportedReasoningEfforts:[{reasoningEffort:'low'}]}];
 await s.save(value,models);const loaded=new Settings(dir);await loaded.init();assert.deepEqual(loaded.value,value);
 await assert.rejects(s.save({...value,effort:'bogus'},models));assert.deepEqual(s.value,value);
 const control={generate:r=>({backend:'app-server',...r})};
 const a=configuredAdapter(control,s);
 assert.equal(a.generate({model:'subscription-default'}).model,'m');
 assert.equal(a.generate({model:'explicit'}).model,'explicit');
 assert.equal(a.generate({}).effort,'low');
 await writeFile(s.path,JSON.stringify({...value,backend:'exec'}));const legacy=new Settings(dir);await legacy.init();assert.deepEqual(legacy.value,value);
 assert.equal(a.generate({}).backend,'app-server');
 }finally{await rm(dir,{recursive:true});}
});
