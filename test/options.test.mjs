import test from 'node:test';
import assert from 'node:assert/strict';
import {parseOptions} from '../src/options.mjs';
test('App Server only: stale adapter environment ignored; old app-server shortcut accepted',()=>{
 assert.deepEqual(parseOptions([],{BRIDGE_ADAPTER:'codex-exec'}),{action:undefined,help:false});
 assert.equal(parseOptions(['--adapter=app-server','--restart'],{}).action,'restart');
 for(const args of [['--adapter'],['--adapter','exec'],['--typo']])assert.throws(()=>parseOptions(args,{}));
});
