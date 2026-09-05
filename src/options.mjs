export function parseOptions(args, env = process.env) {
  let action = env.BRIDGE_ACTION || undefined;
  let help = false;
  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === '--help' || arg === '-h') help = true;
    else if (arg === '--restart') action = 'restart';
    // Keep existing App Server shortcuts working. Stale BRIDGE_ADAPTER is ignored.
    else if (arg === '--adapter' || arg.startsWith('--adapter=')) {
      const value=arg==='--adapter'?args[++i]:arg.slice('--adapter='.length);
      if(value!=='app-server')throw Error('CLI 생성은 제거되었습니다. --adapter 없이 실행하세요.');
    } else throw Error(`Unknown option: ${arg}. Use --help.`);
  }
  if (action !== undefined && !['reuse', 'stop', 'restart'].includes(action)) throw Error('BRIDGE_ACTION must be reuse, stop, or restart');
  return {action, help};
}
export const helpText = `Risu Subscription Bridge · App Server
  --restart  기존 브리지를 종료 후 같은 포트로 재시작
  --help     도움말
BRIDGE_ACTION=reuse|stop|restart 환경 변수도 지원합니다.
진행 중인 응답은 재시작 시 중단됩니다.`;
