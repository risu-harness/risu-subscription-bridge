const $ = id => document.getElementById(id);
const initial = new URLSearchParams(location.hash.slice(1)).get('key');
if (initial) { sessionStorage.setItem('bridge-key', initial); history.replaceState(null, '', '/'); }
$('token').value = sessionStorage.getItem('bridge-key') || '';
$('endpoint').value = location.origin + '/v1/chat/completions';
const headers = () => ({'Content-Type': 'application/json', Authorization: 'Bearer ' + $('token').value});
async function call(path, options = {}) {
  if (!$('token').value.trim()) throw Error('브리지 실행 터미널에 표시된 Setup 주소(#key= 포함)를 열어 주세요. 일반 주소만 열면 로컬 연결 키가 전달되지 않습니다.');
  const r = await fetch(path, {...options, headers: headers()}); const data = await r.json();
  if (r.status === 401 && data.error?.code === 'unauthorized') throw Error('로컬 연결 키가 맞지 않습니다. 브리지 실행 터미널의 Setup 주소(#key= 포함)를 다시 열어 주세요.');
  if (!r.ok) throw Error(data.error?.message || '연결 실패'); return data;
}
let settingsLoaded=false, catalog=[];
function efforts(value='') {
 const chosen=$('models').value==='subscription-default' ? catalog.find(m=>m.isDefault)||catalog[0] : catalog.find(m=>(m.model||m.id)===$('models').value);
 $('effort').replaceChildren(new Option('모델 기본값',''),...(chosen?.supportedReasoningEfforts||[]).map(e=>new Option(e.reasoningEffort,e.reasoningEffort)));
 $('effort').value=value;
 if($('effort').selectedIndex<0)$('effort').value='';
}
async function loadSettings() {
 const r=await call('/internal/settings');catalog=r.models;
 $('models').replaceChildren(new Option('Codex 기본 모델','subscription-default'),...catalog.map(m=>new Option(m.displayName||m.model||m.id,m.model||m.id)));
 for(const key of ['models','verbosity','instructions'])$(key).value=r.settings[key==='models'?'model':key];
 efforts(r.settings.effort);settingsLoaded=true;
}
$('models').onchange=()=>efforts();
$('save-settings').onclick=async()=>{
 $('save-settings').disabled=true;
 try {await call('/internal/settings',{method:'POST',body:JSON.stringify({model:$('models').value,effort:$('effort').value,verbosity:$('verbosity').value,instructions:$('instructions').value})});$('saved').textContent='저장됨 · 다음 요청부터 적용';await refresh();}
 catch(e){$('error').textContent=e.message;$('saved').textContent='저장하지 못했습니다.';}
 finally{$('save-settings').disabled=false;}
};
async function refresh() {
  try {
    sessionStorage.setItem('bridge-key', $('token').value);
    const s = await call('/internal/status');
    $('delivery').textContent = 'App Server · 생성 중 텍스트를 순차 표시합니다.';
    $('status').textContent = s.account.connected ? '● ChatGPT 연결됨 · ' + (s.account.plan || '구독') : '○ ChatGPT 로그인이 필요합니다';
    $('diagnostics').textContent = JSON.stringify(s, null, 2); $('error').textContent = '';
    if (s.account.connected && !settingsLoaded) await loadSettings();
  } catch (e) { $('status').textContent = '○ 로컬 브리지 연결 확인이 필요합니다'; $('error').textContent = e.message; }
}
$('refresh').onclick = refresh;
$('login').onclick = async () => {
  try { const r = await call('/internal/login', {method: 'POST', body: '{}'}); const u = new URL(r.authUrl); if (u.protocol !== 'https:' || !['auth.openai.com', 'chatgpt.com'].includes(u.hostname)) throw Error('예상하지 못한 로그인 URL'); $('auth').href = u.href; $('auth').hidden = false; $('status').textContent = '아래 공식 로그인 링크를 열고 로그인한 뒤 상태 확인을 눌러주세요.'; }
  catch (e) { $('error').textContent = e.message; }
};
for (const [button, field] of [['copy-endpoint', 'endpoint'], ['copy-key', 'token']]) $(button).onclick = async () => { try { await navigator.clipboard.writeText($(field).value); } catch { $(field).select(); } };
let controller;
$('stop').onclick = () => controller?.abort();
$('test').onclick = async () => {
  controller = new AbortController(); $('test').disabled = true; $('stop').disabled = false; $('result').textContent = '응답 기다리는 중…';
  try { const r = await call('/v1/chat/completions', {method: 'POST', signal: controller.signal, body: JSON.stringify({model: 'subscription-default', messages: [{role: 'user', content: '한국어로 연결 성공이라고만 답해 주세요.'}], stream: false})}); $('result').textContent = r.choices[0].message.content; }
  catch (e) { $('result').textContent = e.name === 'AbortError' ? '중단했습니다.' : e.message; }
  finally { $('test').disabled = false; $('stop').disabled = true; refresh(); }
};
refresh();
