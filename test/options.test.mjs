import test from 'node:test';
import assert from 'node:assert/strict';
import {parseOptions} from '../src/options.mjs';
test('adapter flag overrides environment; unknown or missing values fail before launch',()=>{
  assert.equal(parseOptions([],{}).adapter,undefined);
  assert.equal(parseOptions([],{BRIDGE_ADAPTER:'app-server'}).adapter,'app-server');
  assert.equal(parseOptions(['--adapter','exec'],{BRIDGE_ADAPTER:'app-server'}).adapter,'exec');
  assert.deepEqual(parseOptions(['--adapter=app-server','--restart'],{}),{adapter:'app-server',action:'restart',help:false});
  for(const args of [['--adapter'],['--adapter='],['--adapter','wrong'],['--typo']])assert.throws(()=>parseOptions(args,{}));
});
