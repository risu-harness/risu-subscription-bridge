export function parseOptions(args, env = process.env) {
  let adapter = env.BRIDGE_ADAPTER || undefined;
  let action = env.BRIDGE_ACTION || undefined;
  let help = false;
  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === '--help' || arg === '-h') help = true;
    else if (arg === '--restart') action = 'restart';
    else if (arg === '--adapter') adapter = args[++i] ?? '';
    else if (arg.startsWith('--adapter=')) adapter = arg.slice('--adapter='.length);
    else throw Error(`Unknown option: ${arg}. Use --help.`);
  }
  if (adapter !== undefined && !['exec', 'app-server'].includes(adapter)) throw Error('--adapter / BRIDGE_ADAPTER must be exec or app-server');
  if (action !== undefined && !['reuse', 'stop', 'restart'].includes(action)) throw Error('BRIDGE_ACTION must be reuse, stop, or restart');
  return {adapter, action, help};
}
export const helpText = `Risu Subscription Bridge
  --adapter exec|app-server  생성 방식 선택 (새 실행 기본값: exec)
  --restart                 기존 브리지를 종료 후 같은 포트로 재시작
  --help                    도움말

BRIDGE_ADAPTER, BRIDGE_ACTION 환경 변수도 지원합니다. 플래그가 우선합니다.
모드를 바꾸려면 --restart를 함께 지정하세요. 진행 중인 응답은 중단됩니다.
플래그 없이 기존 인스턴스를 재시작하면 현재 생성 방식을 유지합니다.`;
