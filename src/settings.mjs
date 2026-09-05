import {readFile,writeFile,rename} from 'node:fs/promises';
import {join} from 'node:path';
import {BridgeError} from './protocol.mjs';
export class Settings {
  constructor(runtime){this.path=join(runtime,'generation-settings.json');this.value={model:'subscription-default',effort:'',verbosity:'',instructions:''};}
  async init(){try{const saved=JSON.parse(await readFile(this.path,'utf8')); delete saved.backend; this.value={...this.value,...saved};}catch(e){if(e.code!=='ENOENT')throw e;}}
  async save(value,models){
    const allowed=['model','effort','verbosity','instructions'];
    if(!value||allowed.some(k=>typeof value[k]!=='string')||Object.keys(value).some(k=>!allowed.includes(k)))throw new BridgeError('설정 형식이 올바르지 않습니다.',400,'settings_invalid');
    if(!['','low','medium','high'].includes(value.verbosity)||value.instructions.length>16000)throw new BridgeError('설정 값이 올바르지 않습니다.',400,'settings_invalid');
    const chosen=value.model==='subscription-default'?models.find(m=>m.isDefault)??models[0]:models.find(m=>(m.model??m.id)===value.model);
    if(!chosen)throw new BridgeError('사용 가능한 모델을 선택하세요.',400,'unknown_model');
    if(value.effort&&!chosen.supportedReasoningEfforts?.some(e=>e.reasoningEffort===value.effort))throw new BridgeError('이 모델이 지원하는 추론 강도를 선택하세요.',400,'effort_invalid');
    const next={...value};await writeFile(this.path+'.tmp',JSON.stringify(next,null,2),{mode:0o600});await rename(this.path+'.tmp',this.path);this.value=next;return next;
  }
}
export function configuredAdapter(control,settings){
 return {
  get alive(){return control.alive;},get name(){return control.name;},get delivery(){return control.delivery;},
  account:()=>control.account(),models:()=>control.models(),login:()=>control.login(),shutdown:()=>control.shutdown(),
  generate(request,handlers){const s={...settings.value};const model=request.model==='subscription-default'?s.model:request.model;
   return control.generate({...request,model,effort:s.effort,verbosity:s.verbosity,instructions:s.instructions},handlers);
  }
 };
}
export function validateEffort(chosen,effort){if(effort&&!chosen.supportedReasoningEfforts?.some(e=>e.reasoningEffort===effort))throw new BridgeError('선택 모델이 저장된 추론 강도를 지원하지 않습니다. 설정을 기본값으로 바꾸세요.',400,'effort_invalid');}
