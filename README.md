# Risu Subscription Bridge — Mac spike

목적: 기존 Risu의 캐릭터·로어북·이미지·대화 UI를 유지하면서, **요청마다 독립적인 `codex exec --ephemeral --json` 프로세스**로 텍스트 생성을 수행한다. 기본 생성 어댑터는 CLI wrapper다. 자체 채팅 UI·콘텐츠 저장소·API 과금 fallback은 없다.

로그인/계정 확인/실제 모델 목록 조회에는 기존 App Server를 제어 용도로 사용한다. CLI 모드의 캐릭터 본문은 App Server에 전달하지 않는다. 아직 App Server를 전혀 실행하지 않는 순수 CLI 설치판은 아니다. 비교용 생성 경로는 `BRIDGE_ADAPTER=app-server npm start`로 선택한다.

### CLI 우선 결정의 검증 상태

- **스트리밍:** 공식 exec JSONL 이벤트/변환 소스에서 assistant message는 완성 시 출력된다. 브리지는 이 메시지를 SSE로 전달하지만 토큰 실시간 스트리밍으로 부르지 않는다. 완료 문자열을 인위적으로 잘라 타자 효과를 만들지 않는다. 실제 설치 바이너리의 구독 출력 확인은 로그인 후 진행한다.
- **구조화된 문맥:** 역할·순서·내용을 보존한 JSON transcript를 stdin으로 전달한다. 네이티브 system/user/assistant 역할의 의미까지 동일하게 전달되는 것은 아니다. 문자 손실 없음과 모델의 지시 이해/장기 품질은 별도 검증이다.
- **시작 지연:** `processSpawnMs`는 OS 프로세스 생성 이벤트까지이며, Codex 초기화 전체가 아니다. `firstEventMs`, `firstMessageMs`, `elapsedMs`를 따로 측정한다. CLI에서는 `firstTokenMs=null`로 반환한다. API 과금과 비교한 절감액을 토큰 수에서 추정하지 않는다.
- **기록:** `--ephemeral`은 세션 파일 미저장 옵션이다. 일반 exec도 기본적으로 무기록이라는 가정은 하지 않는다. 전용 저장 폴더/임시 세션을 함께 사용하지만 서버 기록·데스크톱 UI 비노출은 실제 성공 호출 전후 확인이 남아 있다.

## 시작

Node.js 22 이상과 Codex가 필요하다. 이 Mac에서는 `/Applications/ChatGPT.app/Contents/Resources/codex` 0.153.0을 사용한다. PATH의 Homebrew Codex 0.104.0은 실행 시 137로 종료되어 사용하지 않았다.

```sh
cd /Users/soo/Documents/ChatGPT/risu/bridge
npm start
```

의존성 설치는 없다. 출력된 Setup URL을 열고 **ChatGPT 로그인 → 공식 로그인 링크**로 로그인한다. 기본 Codex의 인증을 복사하지 않으며, 브리지 전용 로그인이 필요하다. 로그인 후 ‘상태 확인’과 ‘구독으로 테스트’를 실행한다.

## Risu 설정

1. 모델 공급자에서 Reverse Proxy 또는 OpenAI-compatible을 선택한다. 정확한 명칭은 Risu 버전에 따라 다를 수 있다.
2. 요청 주소: `http://127.0.0.1:8787/v1/chat/completions` (base URL을 요구하는 곳은 `http://127.0.0.1:8787/v1`).
3. API key 칸: 설정 페이지의 **로컬 키 복사** 값. OpenAI API 키가 아니다.
4. 모델: `subscription-default` 또는 `/v1/models`가 실제 반환한 이름.
5. 도구 호출, JSON schema 출력, 복수 생성은 끈다. 첫 시험은 텍스트 카드로 한다. 이미지 태그/정규식 연출은 Risu가 계속 처리한다.
6. 스트리밍을 켜고 한 턴 생성 → 중단 → 재생성 → 사용자 메시지 수정 후 재생성을 확인한다.

브라우저의 Local Network Access 허용이 필요할 수 있다. Risu의 네트워크 라우팅은 **사용자 Mac에서 직접 localhost로 연결**해야 한다. 원격 프록시에 맡기면 원격 서버의 localhost를 가리킨다. 웹 Risu 연결이 막힐 경우 Mac 데스크톱 Risu 또는 로컬 Risu에서 시험한다. CORS 헤더만으로 모든 브라우저 보안 제한을 해결하지는 못한다.

기본 허용 Origin은 `https://risuai.xyz`, `https://risuai.net`과 브리지 자체 Origin이다. 로컬 Risu 등은 `BRIDGE_ALLOWED_ORIGINS`에 쉼표로 명시한다. `*` 허용은 하지 않는다.

## 구현 범위와 차이

- `GET /healthz`, `GET /v1/models`, `POST /v1/chat/completions` (JSON/SSE), 로그인·상태 페이지.
- Risu가 전체 기록의 소유자다. CLI는 요청마다 subprocess를 생성하고 종료한다. 비교용 App Server 모드는 매 요청 새 ephemeral thread를 사용하고 종료 후 unload한다. 두 방식 모두 재생성/편집/분기에서 이전 Codex 답변이 자동 누적되지 않는다.
- 원래 역할별 messages를 순서대로 JSON transcript로 전달한다. Codex의 네이티브 system/user/assistant 입력과 완전한 동등성을 주장하지 않는다. 동적 로어의 위치와 텍스트를 보존한다.
- **temperature/top_p/top_k/min_p/penalties/logit_bias/seed/max_tokens/max_completion_tokens는 적용하지 못한다.** SSE 응답 헤더 및 상태 진단/JSON 확장 필드에 무시된 항목을 표시한다. 특히 max_tokens를 엄격한 출력 상한으로 믿으면 안 된다.
- stop 문자열은 델타 경계를 포함해 처리하며, 감지되면 Codex interrupt로 연결한다.
- 같은 시점의 생성은 한 건으로 제한하며 다른 요청은 429 `bridge_busy`를 반환한다. 암묵적 재시도 없음.
- 클라이언트 연결 종료 시 중단. CLI는 SIGTERM 후 1.5초 이내 종료하지 않으면 SIGKILL을 보내고 실제 close까지 기다린다. 생성 제한 시간 180초. 실패한 SSE에는 error를 보내고 정상 완료 `[DONE]`는 보내지 않는다.
- usage는 harness가 실제 보고한 값만 사용한다. 토큰 수는 구독 잔여량이나 API 비용이 아니다.

## 로컬 상태와 보안

`.runtime/codex`에 전용 Codex 설정/인증, `.runtime/bridge-key`에 로컬 API 보호 키가 저장된다. 로그인 정보는 Codex가 관리한다. `.runtime`은 gitignore되고 소유자 접근으로 제한한다. setup URL의 `#key=`는 웹 서버 요청/리퍼러에는 포함되지 않지만 로컬 브라우저에 노출될 수 있으므로 공유하지 않는다.

127.0.0.1만 수신하며 Host·Origin·Bearer를 검사한다. 기존 Codex 설정·MCP·스킬을 가져오지 않고 도구·셸·브라우저·메모리·Remote 기능을 비활성화한다. 그래도 이 spike는 독립적인 보안 샌드박스 제품이 아니며 배포 전 tool availability 검증이 필요하다.

브리지 자체는 대화 본문을 파일이나 로그로 저장하지 않는다. Codex 자체의 임시 파일/로그/인증 및 OpenAI 서버 측 처리는 별개다. 모든 데이터가 Mac에만 남는다는 의미는 아니다. 기존 ChatGPT/Codex 화면과의 노출 분리는 실제 앱에서 추가 검증해야 한다.

## 검증

```sh
npm test
npm run probe
npm run compare
```

test는 비용 없는 가짜 어댑터로 HTTP/SSE, stop, 취소, 입력 검증, 실패, 접근 제한, 편집·동시성 계약을 검사한다. probe는 **실제 구독 한도를 사용**하여 짧은 RP 한 턴을 실행하며 로그인 전에는 종료 코드 2를 반환한다. compare는 동일 입력으로 App Server와 codex exec를 각각 한 번 호출한다(총 2회). `npm run compare -- /absolute/request.json`으로 Risu에서 내보낸 요청을 비교할 수 있다. CLI의 firstMessageMs는 완성 메시지 이벤트까지 시간이며 진정한 첫 토큰 시간과 다를 수 있다. 실제 Risu의 50~100턴은 별도의 P2 검증 항목이며 이 테스트로 완료 처리하지 않는다.

## 다음 단계

P0: 실제 Risu request/응답 확인 및 App Server 한 턴 → 동일 payload의 codex exec 비교. P1/P2: 실제 50~100턴, 모델 품질·TTFT·취소·재시작. P3: 검증된 protocol adapter를 Go/Rust 단일 바이너리로 포팅할지 결정하고 macOS/Windows 설치·서명·공급자 사용 범위를 검토한다. Node 버전은 로컬 검증용이며 원라인 설치 제품이 아니다.

환경 변수: `BRIDGE_ADAPTER`(exec 또는 app-server, 기본 exec), `BRIDGE_PORT`(8787), `BRIDGE_CODEX_BIN`, `BRIDGE_DATA_DIR`, `BRIDGE_ALLOWED_ORIGINS`.

출처: [Non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode), [exec events](https://github.com/openai/codex/blob/main/codex-rs/exec/src/exec_events.rs), [exec JSONL 변환](https://github.com/openai/codex/blob/main/codex-rs/exec/src/event_processor_with_jsonl_output.rs), [App Server](https://learn.chatgpt.com/docs/app-server), [Risu OpenAI request source](https://github.com/kwaroran/RisuAI/blob/main/src/ts/process/request/openAI/requests.ts). 로컬 설치된 Codex 0.153.0의 help와 generate-json-schema를 함께 확인했다.
