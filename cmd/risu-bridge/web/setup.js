const $ = id => document.getElementById(id);
const initial = new URLSearchParams(location.hash.slice(1)).get('key');
if (initial) {
  sessionStorage.setItem('bridge-key', initial);
  history.replaceState(null, '', '/');
}
$('token').value = sessionStorage.getItem('bridge-key') || '';
$('endpoint').value = location.origin + '/v1/chat/completions';

async function call(path, options = {}) {
  const token = $('token').value.trim();
  if (!token) throw Error('실행 터미널의 설정 링크(#key= 포함)를 다시 열거나 로컬 연결 키를 입력해 주세요.');
  const response = await fetch(path, {...options, headers: {'Content-Type': 'application/json', Authorization: 'Bearer ' + token}});
  const data = await response.json();
  if (response.status === 401 && data.error?.code === 'unauthorized') throw Error('로컬 연결 키가 맞지 않습니다. 실행 터미널의 설정 링크를 다시 열어 주세요.');
  if (!response.ok) throw Error(data.error?.message || '연결하지 못했습니다. 실행 터미널을 확인해 주세요.');
  return data;
}

let settingsLoaded = false, catalog = [], controller, provider = "chatgpt", loginPoll;
const providerName = () => provider === "gemini" ? "Gemini" : "ChatGPT";
function status(message, state = '') {
  $('status').textContent = message;
  $('status').dataset.state = state;
}
function efforts(value = '') {
  const chosen = $('models').value === 'subscription-default'
    ? catalog.find(model => model.isDefault) || catalog[0]
    : catalog.find(model => (model.model || model.id) === $('models').value);
  $('effort').replaceChildren(new Option('모델 기본값', ''), ...(chosen?.supportedReasoningEfforts || []).map(item => new Option(item.reasoningEffort, item.reasoningEffort)));
  $('effort').value = value;
  if ($('effort').selectedIndex < 0) $('effort').value = '';
}
async function loadSettings() {
  const data = await call('/internal/settings');
  catalog = data.models;
  $('models').replaceChildren(new Option('모델 기본값', 'subscription-default'), ...catalog.map(model => new Option(model.displayName || model.model || model.id, model.model || model.id)));
  $('models').value = data.settings.model;
  $('verbosity').value = data.settings.verbosity;
  $('instructions').value = data.settings.instructions;
  efforts(data.settings.effort);
  $("effort").disabled = provider === "gemini";
  $("verbosity").disabled = provider === "gemini";
  settingsLoaded = true;
}
$('models').onchange = () => { efforts(); $('saved').textContent = '변경한 설정을 저장해 주세요.'; };
for (const id of ['effort', 'verbosity', 'instructions']) {
  $(id).oninput = () => { $('saved').textContent = '변경한 설정을 저장해 주세요.'; };
}
$('token').oninput = () => {
  settingsLoaded = false;
  $('generation-settings').disabled = true;
  $('test').disabled = true;
  $('auth').hidden = true;
  $('copy-feedback').textContent = '';
  status('키를 변경했습니다. 상태 새로고침을 눌러 연결을 확인하세요.');
};
$('save-settings').onclick = async () => {
  $('save-settings').disabled = true;
  $('error').textContent = '';
  try {
    await call('/internal/settings', {method: 'POST', body: JSON.stringify({model: $('models').value, effort: $('effort').value, verbosity: $('verbosity').value, instructions: $('instructions').value})});
    $('saved').textContent = '저장했습니다. 다음 응답부터 적용됩니다.';
    await refresh();
  } catch (error) {
    $('error').textContent = error.message;
    $('saved').textContent = '저장하지 못했습니다.';
  } finally { $('save-settings').disabled = false; }
};
async function refresh() {
  $('refresh').disabled = true;
  try {
    sessionStorage.setItem('bridge-key', $('token').value.trim());
    const data = await call('/internal/status');
    if (provider !== data.provider) settingsLoaded = false;
    provider = data.provider || 'chatgpt';
    $('provider').value = provider;
    $('provider').disabled = data.busy || Boolean(controller);
    $('gemini-note').hidden = provider !== 'gemini';
    $('account-hint').textContent = provider === 'gemini' ? 'Google AI 구독에 연결된 계정으로 로그인하세요. Gemini CLI의 사용량 한도가 적용됩니다.' : '브리지 전용 로그인입니다. 기존 Codex 로그인과 별도로 연결하세요.';
    const connected = data.account.connected && !data.account.loggingIn;
    status(connected ? providerName() + ' 연결됨' + (data.account.plan ? ' · ' + data.account.plan : '') + (data.busy ? ' · 응답 생성 중' : '') : providerName() + ' 로그인이 필요합니다.', connected ? 'connected' : '');
    $('diagnostics').textContent = JSON.stringify(data, null, 2);
    $('version').textContent = 'Risu Bridge · ' + data.version;
    $('error').textContent = '';
    $('login').textContent = connected ? '로그인 확인' : providerName() + ' 로그인';
    $('login').disabled = Boolean(data.account.loggingIn) || data.account.available === false || data.busy;
    clearTimeout(loginPoll);
    if (data.account.loggingIn) {
      status('기본 브라우저에서 Google 로그인을 완료해 주세요. 완료되면 자동으로 확인합니다.');
      loginPoll = setTimeout(refresh, 2000);
    } else if (data.account.available === false) {
      status('Gemini CLI 설치가 필요합니다. 아래 설치 안내를 확인하세요.');
    } else if (data.account.error) {
      $('error').textContent = data.account.error;
    }
    if (connected) {
      $('auth').hidden = true;
      if (!settingsLoaded) await loadSettings();
    } else { settingsLoaded = false; }
    $('generation-settings').disabled = !connected || data.busy;
    $('test').disabled = !connected || data.busy || Boolean(controller);
    $('settings-hint').textContent = !connected ? providerName() + '를 연결하면 모델과 저장된 설정을 불러옵니다.' : data.busy ? '응답이 끝난 뒤 상태를 새로고침하면 설정을 변경할 수 있습니다.' : 'Risu의 요청 모델이 subscription-default일 때 아래 모델을 사용합니다. 특정 모델 ID를 지정하면 그 모델이 우선합니다.';
  } catch (error) {
    status('연결 상태를 확인할 수 없습니다.', 'error');
    $('error').textContent = error.message;
    $('generation-settings').disabled = true;
    $('test').disabled = true;
  } finally { $('refresh').disabled = false; }
}
$('refresh').onclick = refresh;
$('provider').onchange = async () => {
  const next = $('provider').value;
  $('provider').disabled = true;
  clearTimeout(loginPoll);
  try {
    await call('/internal/provider', {method:'POST', body:JSON.stringify({provider:next})});
    provider = next; settingsLoaded = false;
    $('auth').hidden = true; $('saved').textContent = ''; $('result').textContent = '아직 테스트하지 않았습니다.';
    await refresh();
  } catch (error) { $('provider').value = provider; $('error').textContent = error.message; }
  finally { $('provider').disabled = false; }
};
$('login').onclick = async () => {
  $('login').disabled = true;
  $('error').textContent = '';
  $('auth').hidden = true;
  try {
    const data = await call('/internal/login', {method: 'POST', body: '{}'});
    if (data.pending && data.provider === 'gemini') {
      settingsLoaded = false;
      await refresh();
      return;
    }
    const url = new URL(data.authUrl);
    if (url.protocol !== 'https:' || !['auth.openai.com', 'chatgpt.com'].includes(url.hostname)) throw Error('공식 로그인 주소를 확인할 수 없습니다. 다시 시도해 주세요.');
    $('auth').href = url.href;
    $('auth').hidden = false;
    settingsLoaded = false;
    $('generation-settings').disabled = true;
    $('test').disabled = true;
    status('아래 링크에서 로그인한 뒤 상태 새로고침을 눌러 주세요.');
  } catch (error) { $('error').textContent = error.message; $('login').disabled = false; }
  finally { if (provider !== 'gemini') $('login').disabled = false; }
};
for (const [button, field, label] of [['copy-endpoint', 'endpoint', '요청 주소'], ['copy-key', 'token', '로컬 연결 키']]) {
  $(button).onclick = async () => {
    if (!$(field).value.trim()) { $('copy-feedback').textContent = '먼저 실행 터미널의 설정 링크로 열어 키를 불러오세요.'; return; }
    try {
      await navigator.clipboard.writeText($(field).value.trim());
      $('copy-feedback').textContent = label + '를 복사했습니다.';
    } catch {
      $(field).focus();
      $(field).select();
      $('copy-feedback').textContent = '자동 복사가 지원되지 않습니다. 선택된 값을 직접 복사해 주세요.';
    }
  };
}
$('stop').onclick = () => controller?.abort();
$('test').onclick = async () => {
  controller = new AbortController();
  $('test').disabled = true;
  $('stop').disabled = false;
  $('generation-settings').disabled = true;
  $('result').textContent = '저장된 설정으로 응답을 기다리고 있습니다…';
  try {
    const data = await call('/v1/chat/completions', {method: 'POST', signal: controller.signal, body: JSON.stringify({model: 'subscription-default', messages: [{role: 'user', content: '한국어로 연결 성공이라고만 답해 주세요.'}], stream: false})});
    $('result').textContent = data.choices[0].message.content;
  } catch (error) { $('result').textContent = error.name === 'AbortError' ? '응답 생성을 중단했습니다.' : error.message; }
  finally {
    controller = undefined;
    $('stop').disabled = true;
    await refresh();
  }
};
refresh();
