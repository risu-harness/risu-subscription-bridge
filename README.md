# Risu Subscription Bridge — Mac

Risu의 캐릭터·로어북·대화 UI를 유지하며 ChatGPT 구독으로 응답을 생성하는 로컬 브리지다. **Codex App Server만 사용한다.** 요청마다 새 임시 스레드를 만들고, 생성 중 텍스트를 OpenAI-compatible SSE로 전달한다. `codex exec` 생성 구현은 제거했다.

## 설치 및 실행

처음 설치하거나 다시 실행할 때 같은 명령을 사용한다.

```sh
curl -fsSL https://raw.githubusercontent.com/risu-harness/risu-subscription-bridge/main/install.sh | sh
```

실행 중이면 재사용 / 종료 / 최신 버전 재시작을 선택한다. Enter는 재사용이다. 기존 CLI 인스턴스는 재사용 대신 같은 포트에서 App Server로 전환하며 진행 중인 응답은 중단된다. 터미널이 없으면 기본 재사용 정책을 사용한다. 설정 링크에 저장된 로컬 키를 자동으로 넣어 출력하고 브라우저를 연다.

설치 위치는 `~/.local/share/risu-subscription-bridge`. `data/`의 로그인·연결 키·생성 설정은 재설치해도 유지된다. 같은 설치의 동시 실행은 OS 잠금으로 제한한다. 기본 포트는 8787이며 다른 프로그램이 사용하면 8799까지 찾는다. 재시작 시 기존 포트를 유지한다. 터미널을 유지하고 Ctrl+C로 종료한다.

macOS Apple Silicon/Intel 분기를 지원한다. Node 22 이상과 Codex 0.153.0을 재사용하거나 공식 배포처에서 설치한다. Node 다운로드는 고정 SHA-256으로 확인하며 npm 설치는 lifecycle scripts를 비활성화한다. 관리자 권한·Homebrew·전역 npm 설치는 필요 없다. 실제 설치 검증은 Apple Silicon에서 수행했다.

## Risu 연결

- Custom API URL: 설정 페이지에 표시된 `/v1/chat/completions` 주소
- 키: 설정 페이지의 로컬 연결 키 (OpenAI API 키가 아님)
- 요청 모델: `subscription-default`이면 브라우저에 저장한 모델 사용; 명시한 모델이 있으면 우선
- 포맷: OpenAI Compatible
- Response 스트리밍: 켜면 생성 중 텍스트 표시

공식 웹 Risu는 localhost 호출을 자체 차단한다. Mac 데스크톱 Risu 또는 로컬 Node Risu를 사용한다. Node Risu는 로컬 네트워크 모드를 켜서 서버 프록시로 호출한다. 브라우저 직접 호출이 필요한 별도 로컬 Origin은 `BRIDGE_ALLOWED_ORIGINS`에 명시한다.

## 브라우저 생성 설정

모델, 모델이 지원하는 추론 강도, 답변 상세도, 추가 대화 지시문을 설정한다. 저장하면 다음 응답부터 적용되며 `data/generation-settings.json`에 보관한다. 기존 파일의 backend 값은 무시한다. 응답 생성 중에는 저장을 거절한다.

App Server의 `thread/start.config.model_verbosity`, `baseInstructions`, `turn/start.effort`로 전달한다. 추가 지시문은 공통 대화 지시문에 붙인다. 상세도는 모델 지원에 따라 적용되며 정확한 길이 제한이 아니다.

## 범위와 제한

- 로그인·모델 조회·생성 모두 공식 `codex app-server --listen stdio://` 경로다. Codex 실행 파일은 여전히 필요하다.
- 전용 CODEX_HOME을 사용하며 기존 Codex 인증을 복사하지 않는다. ChatGPT 로그인이 필요하다. API 키 과금 fallback은 없다.
- Risu가 전체 기록을 관리한다. 매 요청 새 ephemeral thread를 만들고 완료 후 unsubscribe한다. 편집·재생성·분기에 과거 응답을 중복 누적하지 않는다.
- 역할과 순서를 유지한 JSON 대화록으로 전달한다. 네이티브 system/user/assistant 역할과 의미가 완전히 같지는 않다.
- 텍스트 전용. 도구·이미지·복수 생성·JSON schema 요청은 거절한다.
- temperature, top_p, top_k, min_p, penalties, seed, logit_bias, max_tokens, max_completion_tokens는 적용하지 않는다. stop은 브리지에서 처리한다.
- 한 번에 한 응답, 제한 시간 180초. 클라이언트 중단 시 turn/interrupt. 실패한 SSE에는 정상 완료 DONE을 보내지 않는다.
- loopback 바인딩, Host/Origin 검사, bearer 키가 필요하다. 내부 설정·로그인·종료 API는 외부 Origin에서 접근할 수 없다.
- 임시 스레드는 로컬 대화 저장을 피하지만 서비스 측 기록이나 모든 Codex 화면에서의 비노출을 보장하지 않는다. 구독 한도는 무제한이 아니다.

## 개발 및 검증

```sh
npm start
npm test
npm run probe
```

probe는 실제 구독 한도를 사용하는 짧은 생성 시험이다. 테스트는 HTTP/SSE·취소·인증·스레드 정리·설정 전달·설정 마이그레이션·중복 실행을 검증한다. 50~100턴의 캐릭터 품질 검증을 대신하지 않는다.

환경 변수: `BRIDGE_PORT`, `BRIDGE_CODEX_BIN`, `BRIDGE_DATA_DIR`, `BRIDGE_ALLOWED_ORIGINS`, `BRIDGE_OPEN_BROWSER`, `BRIDGE_ACTION`. 설치용: `BRIDGE_INSTALL_DIR`, `BRIDGE_INSTALL_ONLY`, `BRIDGE_FORCE_DOWNLOAD`. `--restart` 지원. 과거 `BRIDGE_ADAPTER` 환경 변수는 무시하며 `--adapter app-server`는 이전 명령 호환용으로만 받아들인다.

[App Server 문서](https://learn.chatgpt.com/docs/app-server)
