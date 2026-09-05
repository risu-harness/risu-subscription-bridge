# Risu Subscription Bridge

RisuAI의 캐릭터·로어북·대화 UI를 그대로 사용하면서, **ChatGPT 구독으로 로그인한 Codex를 통해 응답을 생성하는 macOS용 로컬 브리지**입니다. RisuAI에는 OpenAI-compatible Chat Completions 주소로 연결합니다.

현재 구현은 **Codex App Server**를 사용합니다. 이전 `codex exec` 생성 방식은 제거되었으며, 생성 중 도착하는 텍스트를 SSE로 전달합니다. 별도의 OpenAI API 키나 API 과금 fallback은 사용하지 않습니다. 사용량은 연결한 계정의 구독 한도를 따르며 무제한 사용을 제공하지 않습니다.

## 동작 구조

```text
RisuAI 데스크톱 / 로컬 Node Risu
  │ 캐릭터 설정·로어북·전체 대화 기록을 포함한 messages
  ▼
로컬 브리지 (127.0.0.1, 연결 키 인증)
  │ 요청마다 새로운 ephemeral thread 생성
  ▼
Codex App Server (전용 CODEX_HOME, ChatGPT 로그인)
  │ 생성 중 텍스트 delta
  ▼
브리지 → OpenAI-compatible SSE 또는 JSON → RisuAI
```

RisuAI가 대화 기록을 관리합니다. 브리지는 매 요청마다 새 임시 스레드를 만들고 완료·실패·취소 후 구독을 해제합니다. 다음 요청에서 이전 스레드를 재사용하지 않으므로, RisuAI의 편집·재생성·분기에 Codex 내부의 과거 대화가 중복해서 붙지 않습니다.

**공식 웹사이트 `risuai.xyz`는 로컬 주소 요청을 자체 차단합니다.** Mac에서 처음 시험할 때는 [RisuAI 데스크톱 릴리스](https://github.com/kwaroran/RisuAI/releases)를 사용하세요. 브리지의 CORS 설정만 변경해서 이 차단을 해결할 수는 없습니다.

## 빠른 시작

### 1. 설치하고 실행하기

Mac 터미널에서 실행합니다.

```sh
curl -fsSL https://raw.githubusercontent.com/risu-harness/risu-subscription-bridge/main/install.sh | sh
```

설치 스크립트는 소스와 필요한 실행 도구를 준비하고, 브리지를 실행한 뒤 설정 페이지를 엽니다.

- 설치 위치: `~/.local/share/risu-subscription-bridge`
- 기본 포트: `8787`. 다른 프로그램이 사용 중이면 `8799`까지 빈 포트를 찾습니다.
- macOS Apple Silicon 및 Intel 설치 분기를 제공합니다.
- Node.js 22 이상을 재사용하거나 Node.js **22.23.2**를 다운로드합니다.
- Codex **0.153.0**을 재사용하거나 공식 npm 패키지로 설치합니다.
- Node 다운로드는 고정 SHA-256으로 검증합니다. npm 설치의 lifecycle scripts는 실행하지 않습니다.
- 관리자 권한, Homebrew, 전역 npm 설치, 셸 설정 변경이 필요하지 않습니다.

브리지는 **터미널에서 실행되는 프로그램**입니다. 자동 시작 서비스가 아니므로 사용하는 동안 해당 터미널을 유지하세요. `Ctrl+C`로 종료할 수 있습니다.

### 2. ChatGPT 로그인과 연결 시험

1. 자동으로 열린 설정 페이지에서 **ChatGPT 로그인**을 누릅니다.
2. 표시되는 공식 로그인 링크를 열고 로그인합니다.
3. 설정 페이지로 돌아와 **상태 확인**을 누릅니다.
4. 연결됨 상태를 확인한 뒤 연결 테스트를 실행합니다. 테스트도 실제 구독 한도를 사용합니다.

브리지는 기존 Codex와 분리된 인증 저장 공간을 사용합니다. 다른 Codex 앱에서 이미 로그인했더라도 브리지에서 처음 한 번은 로그인해야 합니다.

설정 페이지가 자동으로 열리지 않으면 터미널에 출력된 **`Setup: http://127.0.0.1:포트/#key=...` 전체 주소**를 여세요. `#key=...`가 없는 주소를 새 탭에서 열면 연결 키가 전달되지 않을 수 있습니다. 이 링크에는 로컬 접근 키가 들어 있으므로 공개하지 마세요.

### 3. RisuAI 연결하기

RisuAI 데스크톱에서 설정 → 채팅 봇 → 모델을 열고 다음 값을 입력합니다.

| 항목 | 설정 |
| --- | --- |
| 모델 | `Custom API` |
| 보조 모델 | 보조 요청도 브리지로 보내려면 `Custom API` |
| URL | 브리지 설정 페이지의 요청 주소 전체. 예: `http://127.0.0.1:8787/v1/chat/completions` |
| 키/패스워드 | 브리지 설정 페이지에서 복사한 **로컬 연결 키** |
| 요청 모델 | `subscription-default` |
| 포맷 | `OpenAI Compatible` |
| 토크나이저 | `Tiktoken (OpenAI)` |
| Response 스트리밍 | 켜기: 생성 중 텍스트 표시. 끄기: 완성된 응답 표시 |
| Ooba 모드 | 끄기 |

실제 포트가 `8788` 등으로 표시되면 그 주소를 사용하세요. 키는 ChatGPT 비밀번호나 OpenAI API 키가 아닙니다. 모델 이름을 직접 입력하려면 브리지에서 조회된 모델을 사용해야 합니다. 기존 Claude 등의 모델 이름을 그대로 두면 `unknown_model` 오류가 납니다.

먼저 짧은 텍스트 메시지로 시험하세요. 캐릭터 파일·기존 대화는 RisuAI에서 가져와야 하며, 브리지가 웹 Risu의 데이터를 자동 이전하지는 않습니다.

로컬 Node Risu를 사용하는 경우에는 로컬 네트워크 모드를 통해 **Mac에서 실행 중인 Risu 서버**가 브리지를 호출하도록 구성합니다. 원격 서버의 프록시로 보내면 그 서버의 localhost를 가리킵니다. 별도 브라우저 Origin의 직접 접근이 필요하면 아래 `BRIDGE_ALLOWED_ORIGINS` 설정도 확인하세요.

## 모델과 생성 설정

브리지 설정 페이지에서 다음 값을 저장할 수 있습니다. 저장한 값은 다음 요청부터 적용됩니다. 응답 생성 중에는 저장할 수 없습니다.

| 설정 | 동작 |
| --- | --- |
| 모델 | 계정에서 조회한 모델 중 선택 |
| 추론 강도 | 선택한 모델이 지원하는 값 또는 모델 기본값 |
| 답변 상세도 | 기본값 / `low` / `medium` / `high`. 적용 여부는 모델 지원에 따름 |
| 추가 대화 지시문 | 공통 대화 지시문 뒤에 추가. 최대 16,000자 |

Risu 요청의 모델이 `subscription-default`이거나 생략되면 저장된 모델을 사용합니다. 저장된 모델도 `subscription-default`이면 Codex가 반환한 기본 모델을 사용합니다. Risu에서 모델을 명시하면 그 모델이 우선하지만, 저장된 추론 강도·상세도·추가 지시문은 계속 적용됩니다. 명시한 모델이 저장된 추론 강도를 지원하지 않으면 설정을 기본값으로 바꾸세요.

설정은 서버의 `data/generation-settings.json`에 저장됩니다. 브라우저에만 저장되는 값이 아닙니다. 상세도는 정확한 출력 길이 제한이 아닙니다.

## 재실행·업데이트·종료

최신 소스를 받아 실행하려면 설치 명령을 다시 사용합니다. 실행 중인 인스턴스가 있으면 다음 선택지가 나옵니다.

1. **재사용**: 기존 프로세스를 유지하고 설정 페이지를 엽니다. Enter 기본값입니다.
2. **종료**: 기존 프로세스만 종료합니다.
3. **재시작**: 기존 프로세스를 종료하고 새로 받은 버전을 같은 포트에서 실행합니다.

업데이트를 실제 실행 중인 프로세스에 반영하려면 **재시작**을 선택하세요. 종료·재시작은 진행 중인 응답을 중단합니다. 대화형 터미널이 없으면 기본적으로 재사용합니다. 단, 과거 CLI 인스턴스는 재사용을 선택해도 App Server로 재시작합니다.

다운로드 없이 설치된 버전을 실행하려면:

```sh
sh "$HOME/.local/share/risu-subscription-bridge/bin/risu-bridge"
```

설치된 버전으로 바로 재시작하려면:

```sh
sh "$HOME/.local/share/risu-subscription-bridge/bin/risu-bridge" --restart
```

실행 중인 인스턴스의 동작은 `BRIDGE_ACTION=reuse|stop|restart`로도 선택할 수 있습니다. 이 옵션은 설치판 런처에서 처리합니다. 실행 중인 인스턴스가 없으면 런처는 새로 시작합니다. 같은 데이터 경로의 중복 실행은 로컬 TCP 포트를 이용한 OS 잠금으로 제한합니다.

## 지원 범위와 API 차이

OpenAI-compatible 형식을 일부 지원하는 브리지이며, OpenAI API 전체와 동일하지 않습니다.

| 항목 | 현재 구현 |
| --- | --- |
| 엔드포인트 | `GET /v1/models`, `POST /v1/chat/completions` |
| 응답 | JSON 또는 SSE. `stream_options.include_usage` 지원 |
| 입력 | `system`, `developer`, `user`, `assistant`의 텍스트 및 텍스트 content 배열 |
| 역할 전달 | 역할·이름·순서를 포함한 JSON 대화록을 하나의 텍스트 입력으로 전달. 네이티브 역할 의미와 완전히 같지는 않음 |
| 생성 개수 | `n=1`만 지원 |
| stop | 브리지에서 처리. 최대 16개, 각 1,000자 이하. 일치하면 생성 중단 요청 |
| 적용하지 않는 값 | `temperature`, `top_p`, `top_k`, `min_p`, `frequency_penalty`, `presence_penalty`, `logit_bias`, `seed`, `max_tokens`, `max_completion_tokens` |
| 거절하는 요청 | 도구 호출, 이미지 등 비텍스트 입력, `response_format`, 복수 생성 |
| 요청 크기 | JSON 본문 최대 2 MiB. 모델의 컨텍스트 한도와는 별개 |
| 동시 생성 | 브리지당 한 건. 추가 요청은 `429 bridge_busy` |
| 시간 제한 | 생성 턴 대기 180초. 로그인·모델 조회·스레드 준비 등 RPC에는 별도 제한 적용 |
| 취소 | 클라이언트 연결 종료 시 `turn/interrupt` 요청 |
| 실패 처리 | 자동 재시도 없음. SSE 시작 후 실패하면 오류 이벤트로 종료하고 `[DONE]`을 보내지 않음 |

적용하지 않은 파라미터는 JSON 응답의 `bridge.ignored_parameters`, SSE의 `X-Bridge-Ignored-Parameters` 헤더, 설정 페이지 진단 정보에 표시합니다. 사용량은 Codex가 제공한 경우에만 반환합니다.

RisuAI가 프롬프트에 넣은 이미지·감정 표현용 **텍스트 태그**는 대화 텍스트로 전달됩니다. 실제 이미지 인식이나 이미지 생성은 지원하지 않습니다.

## 로컬 데이터와 접근 제어

설치판은 아래 경로를 사용합니다.

```text
~/.local/share/risu-subscription-bridge/
├── bin/risu-bridge            # 실행 명령
├── install.json              # 현재 소스와 실행 파일 경로
├── releases/                 # 설치별 소스
└── data/
    ├── bridge-key            # 로컬 연결 키
    ├── generation-settings.json
    ├── codex/                # 전용 CODEX_HOME: 로그인 및 Codex 설정
    └── work/                 # 전용 작업 디렉터리
```

재설치해도 `data/`는 유지됩니다. 인증 데이터와 추가 지시문이 포함될 수 있으므로 이 폴더를 저장소에 올리거나 공유하지 마세요.

브리지는 `127.0.0.1`에만 바인딩하며 Host·Origin과 bearer 키를 검사합니다. 기본 허용 Origin은 `https://risuai.xyz`, `https://risuai.net`, `tauri://localhost`, `http://tauri.localhost` 및 브리지 자체 주소입니다. 이 허용 목록이 Risu 웹 버전의 로컬 요청 차단을 해제하는 것은 아닙니다.

설정·로그인·종료용 `/internal/*`는 다른 Origin의 브라우저 요청을 거절합니다. 공개 상태 확인용 `/healthz`를 제외한 API에는 연결 키가 필요합니다. 전용 Codex 설정에서는 도구·셸·웹 검색 등을 비활성화하고, 모델이 요청한 도구 실행이나 승인을 받아들이지 않습니다.

브리지는 대화 본문과 Codex 원본 로그를 자체 로그로 출력하지 않습니다. 다만 터미널의 Setup 링크에는 연결 키가 포함됩니다. 임시 스레드와 기록 비저장 설정은 **서비스 측 기록이나 모든 Codex 화면에서의 비노출을 보장하지 않습니다.** 로컬 브리지를 사용해도 응답 생성을 위한 대화 내용은 모델 서비스로 전송됩니다.

## 문제 해결

| 증상 | 확인 및 조치 |
| --- | --- |
| `You are trying local request on web version` | 공식 웹 Risu의 자체 차단입니다. Risu 데스크톱 또는 로컬 Node Risu를 사용하세요. CORS를 `*`로 바꾸는 것으로 해결되지 않습니다. |
| `Local bridge key required.` | 설정 페이지는 터미널의 `#key=...` 포함 전체 주소로 다시 여세요. Risu에는 해당 브리지의 로컬 연결 키를 입력하세요. |
| `login_required` | 브리지 설정 페이지에서 ChatGPT 로그인 후 상태 확인을 누르세요. 기존 Codex 앱 로그인과 별도입니다. |
| `unknown_model` | Risu 요청 모델을 `subscription-default` 또는 `/v1/models`에 있는 모델로 변경하세요. |
| `effort_invalid` | 브리지에서 추론 강도를 모델 기본값으로 바꾸고 저장하세요. |
| `origin_denied` | 오류의 Origin을 확인해 필요한 주소만 허용 목록에 지정하고 재시작하세요. |
| `bridge_busy` | 기존 응답이 끝나거나 취소 처리가 끝난 뒤 다시 요청하세요. |
| `rate_limit` | 구독 한도 초기화 후 다시 사용하세요. API 과금으로 자동 전환하지 않습니다. |
| `harness_stopped`, `cleanup_failed` | 브리지를 재시작하세요. |
| 연결 거부·접속 실패 | 브리지 터미널이 실행 중인지, 설정 페이지와 Risu의 포트가 같은지 확인하세요. |

## 개발 및 검증

저장소 루트에서 Node.js 22 이상과 실행 가능한 Codex를 준비한 뒤 실행합니다. 별도 npm 의존성 설치는 필요하지 않습니다.

```sh
npm start
npm test
```

개발 실행은 기본적으로 저장소의 `.runtime/`를 사용합니다. 설치판의 `data/`와 키·인증이 다르며, `npm start`는 설치판 런처의 포트 자동 탐색이나 재사용 메뉴를 제공하지 않습니다. 포트 충돌 시 `BRIDGE_PORT=8788 npm start`처럼 지정하세요.

개발 브리지에 로그인한 후 실제 생성 시험을 실행할 수 있습니다.

```sh
npm run probe
# 개발 브리지 포트를 바꾼 경우
BRIDGE_URL=http://127.0.0.1:8788 npm run probe
```

`probe`는 실제 구독 한도를 사용합니다. 키를 항상 저장소의 `.runtime/bridge-key`에서 읽으므로 설치판 테스트에는 설정 페이지의 연결 테스트를 사용하세요. `BRIDGE_DATA_DIR`만 지정해도 probe의 키 경로가 바뀌지는 않습니다.

자동 테스트는 가짜 Codex/어댑터로 HTTP·SSE, 인증, 취소, 스레드 정리, 생성 설정, 이전 설정 호환, 실행 옵션, 중복 실행을 검증합니다. 실제 구독 인증·모델 응답 품질이나 Risu의 50~100턴 장기 대화를 검증하는 테스트는 아닙니다.

주요 코드 위치:

| 파일 | 역할 |
| --- | --- |
| `install.sh`, `scripts/launch.mjs`, `scripts/lifecycle.mjs` | 설치, 인스턴스 재사용·종료·재시작 |
| `src/main.mjs`, `src/instance.mjs` | 전용 실행 환경, 인증 키, 중복 실행 제한 |
| `src/codex.mjs` | App Server 로그인·모델 조회·임시 스레드 생성·delta·취소 |
| `src/server.mjs`, `src/protocol.mjs` | HTTP/SSE, 입력 검증, 접근 제어, stop 처리 |
| `src/settings.mjs`, `src/setup.html`, `src/setup.js` | 저장되는 생성 설정과 로컬 설정 UI |

## 환경 변수

| 변수 | 용도 / 기본값 |
| --- | --- |
| `BRIDGE_PORT` | HTTP 포트 / `8787`. 설치 런처에서 명시하면 다른 포트로 자동 변경하지 않음 |
| `BRIDGE_DATA_DIR` | 직접 실행의 데이터 경로 / `.runtime`. 설치 런처는 설치 경로의 `data`로 지정 |
| `BRIDGE_CODEX_BIN` | Codex 실행 파일 경로. 설치기는 고정 버전 여부를 검사하고, 설치 런처는 저장된 경로 사용 |
| `BRIDGE_ALLOWED_ORIGINS` | 쉼표로 구분한 Origin. 기본 외부 허용 목록을 **대체**하며 브리지 자체 Origin은 유지 |
| `BRIDGE_OPEN_BROWSER` | `1`이면 설정 페이지 열기. 설치 런처 기본값 `1`, 직접 실행은 명시해야 열림 |
| `BRIDGE_ACTION` | 설치 런처의 기존 인스턴스 처리: `reuse`, `stop`, `restart` |
| `BRIDGE_INSTALL_DIR` | 설치 경로. 절대 경로 필요 |
| `BRIDGE_INSTALL_ONLY` | `1`이면 설치 후 실행하지 않음 |
| `BRIDGE_FORCE_DOWNLOAD` | `1`이면 Node를 다운로드하고 Codex 자동 탐색을 건너뜀. 명시한 Codex 경로는 검사 후 사용 가능 |
| `BRIDGE_NODE_BIN` | 설치 시 사용할 Node 경로 |
| `BRIDGE_SOURCE_DIR` | 설치 시 복사할 로컬 소스 경로 |
| `BRIDGE_REPO`, `BRIDGE_REF` | 다운로드할 저장소와 ref / `risu-harness/risu-subscription-bridge`, `main` |
| `BRIDGE_URL` | 개발용 probe의 접속 주소 / `http://127.0.0.1:8787` |

파이프로 실행하는 설치 스크립트에 환경 변수를 전달할 때는 `sh` 쪽에 지정하세요.

```sh
curl -fsSL https://raw.githubusercontent.com/risu-harness/risu-subscription-bridge/main/install.sh | BRIDGE_ACTION=restart sh
```

과거 `BRIDGE_ADAPTER` 환경 변수는 무시합니다. `--adapter app-server`는 이전 명령과의 호환용으로만 허용하며 CLI 생성으로 전환하는 옵션은 없습니다.

## 참고

- [Codex App Server 문서](https://learn.chatgpt.com/docs/app-server)
- [RisuAI 소스 및 데스크톱 배포](https://github.com/kwaroran/RisuAI)
- [RisuAI 네트워크 요청 구현](https://github.com/kwaroran/RisuAI/blob/main/src/ts/globalApi.svelte.ts)
