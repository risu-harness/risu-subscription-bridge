# Risu Subscription Bridge

RisuAI의 캐릭터·로어북·대화 UI를 그대로 사용하면서, ChatGPT 또는 Gemini의 Google 로그인으로 텍스트 응답을 생성하는 로컬 브리지입니다. Mac에서 실행하고 Risu의 Custom API에 연결합니다.

- **macOS Apple Silicon·Intel 지원** — 개발 도구나 관리자 권한 없이 설치
- **ChatGPT 로그인** — 별도 API 키 없이 구독 사용량 이용
- **응답 설정** — 모델, 추론 강도, 답변 상세도, 추가 지시사항 저장
- **텍스트·스트리밍·중단 지원** — 대화 기록은 Risu가 관리

## 시작하기

터미널에서 실행하세요.

```sh
curl -fsSL https://raw.githubusercontent.com/risu-harness/risu-subscription-bridge/main/install.sh | sh
```

1. 자동으로 열린 설정 페이지에서 **ChatGPT 로그인**을 누릅니다.
2. **공식 로그인 페이지 열기** 링크에서 로그인한 뒤 **상태 새로고침**을 누릅니다.
3. 아래 안내대로 Risu에 요청 주소와 로컬 연결 키를 입력합니다.
4. 원하는 응답 설정을 저장하고 **응답 확인**으로 연결을 테스트합니다.

브리지를 사용하는 동안 터미널을 열어 두세요. **Ctrl+C**로 종료합니다. 설정 페이지가 열리지 않으면 터미널에 표시된 `#key=` 포함 주소를 직접 여세요. 이 링크에는 로컬 연결 키가 포함되므로 공유하지 마세요.

### Gemini · Google 로그인 연결

Gemini 지원이 포함된 현재 소스 빌드에서 사용할 수 있습니다. 기존 고정 릴리스 설치 명령만 다시 실행하면 아직 이 변경이 포함되지 않을 수 있습니다.

1. 프로젝트 폴더에서 `sh scripts/install-gemini.sh`를 실행합니다. 별도 설치 공간에 Node.js 22 LTS와 **Gemini CLI 0.58.0**을 설치합니다. 시스템 Node/npm이나 관리자 권한은 필요하지 않습니다.
2. 업데이트한 브리지를 실행하고 설정 페이지의 **사용할 AI → Gemini · Google 로그인**을 선택합니다.
3. **Gemini 로그인**을 누르고 기본 브라우저에서 Google 로그인을 완료합니다. Google AI Pro/Ultra를 사용한다면 구독에 연결된 계정을 선택하세요. 브리지가 로그인 완료를 확인하고 CLI가 제공한 모델 목록을 표시합니다.
4. 모델과 추가 지시사항을 저장하고 **응답 확인**을 실행합니다. Risu의 요청 주소·키·`subscription-default`는 그대로 사용합니다.

ChatGPT와 Gemini의 응답 설정은 따로 저장되며 선택한 AI는 재시작 후에도 유지됩니다. 모델 ID를 명시한 요청도 **현재 선택한 AI**로 전달됩니다. provider를 자동 전환하거나 API 키 과금으로 우회하지 않습니다. Gemini에서는 추론 강도·답변 상세도 설정을 비활성화하며, 필요한 문체는 추가 지시사항으로 지정하세요.

Gemini CLI가 이미 있다면 `BRIDGE_GEMINI_BIN=/absolute/path/to/gemini`로 지정할 수 있습니다. 기본적으로 브리지 설치 폴더의 `gemini-cli/bin/gemini`를 찾고, 없으면 PATH의 `gemini`를 사용합니다. 기존 Gemini CLI 계정·설정은 가져오지 않고 **브리지 전용 Google 로그인**을 사용합니다. CLI는 Google 로그인으로 이용 가능한 모델과 사용량 한도를 따릅니다. 플랜 이름이나 정확한 잔여 한도를 브리지가 추정해 표시하지 않습니다.

개발용 실행 예시:

```sh
sh scripts/install-gemini.sh
go build -o /tmp/risu-bridge ./cmd/risu-bridge
/tmp/risu-bridge --restart
```

Google 로그인은 공식 CLI의 ACP `authenticate`로 처리하며 OAuth endpoint나 토큰 교환을 브리지가 재구현하지 않습니다. [공식 인증 안내](https://geminicli.com/docs/get-started/authentication/), [공식 ACP 구현](https://github.com/google-gemini/gemini-cli/tree/v0.58.0/packages/cli/src/acp).

### Risu 연결

**Mac용 [RisuAI 데스크톱 앱](https://github.com/kwaroran/RisuAI/releases/latest)을 사용하세요.** 공식 웹 버전은 localhost 요청을 자체 차단하므로 CORS 설정만으로 연결할 수 없습니다.

Risu의 **설정 → 채팅 봇 → 모델**에서 다음 값을 입력합니다.

| 항목 | 설정 |
|---|---|
| 모델 | Custom API |
| URL | 설정 페이지의 전체 `/v1/chat/completions` 주소 |
| 키/패스워드 | 설정 페이지의 로컬 연결 키 |
| 요청 모델 | `subscription-default` |
| 포맷 | OpenAI Compatible |
| 토크나이저 | Tiktoken (OpenAI) |
| Response 스트리밍 | 켜기 가능 |
| Ooba 모드 | 끄기 |

기본 주소는 `http://127.0.0.1:8787/v1/chat/completions`입니다. 포트가 사용 중이면 8799까지 빈 포트를 찾으므로 **설정 페이지에 표시되는 주소를 복사**하세요. 로컬 연결 키는 브리지 접근용이며 ChatGPT 비밀번호나 OpenAI API 키가 아닙니다.

### 응답 설정

설정 페이지에서 기본 모델·추론 강도·답변 상세도·추가 지시사항을 저장합니다. 저장한 값은 다음 요청부터 적용되며 재시작 후에도 유지됩니다. 연결 테스트도 저장된 설정을 사용하고 구독 사용량을 소모합니다.

`subscription-default` 요청에는 저장한 모델을 사용합니다. Risu에서 특정 모델 ID를 지정하면 그 모델이 우선합니다. 저장한 추론 강도가 해당 모델과 호환되지 않으면 오류가 반환되므로 설정 페이지에서 바꾸세요. 답변 상세도는 지원 모델에서 적용되며 글자 수나 출력 토큰 상한을 보장하지 않습니다.

## 실행과 업데이트

설치 위치는 `~/.local/share/risu-subscription-bridge`입니다. 다시 실행하려면:

```sh
sh "$HOME/.local/share/risu-subscription-bridge/bin/risu-bridge"
```

이미 실행 중이면 **재사용 / 종료 / 재시작**을 선택합니다. 비대화형 환경에서는 기존 인스턴스를 재사용합니다. 종료·재시작하면 진행 중인 응답은 중단됩니다.

```sh
sh "$HOME/.local/share/risu-subscription-bridge/bin/risu-bridge" --restart
```

브리지 업데이트는 설치 명령을 다시 실행한 뒤 **재시작**을 선택하세요. 설치만으로 실행 중인 프로세스가 교체되지는 않습니다. `data/`의 로그인 정보, 로컬 연결 키, 응답 설정은 보존됩니다.

### Codex 설치 방식

브리지는 공식 Codex App Server를 사용합니다. Node, Python, Go, npm, Homebrew, Git을 미리 설치할 필요는 없습니다.

| 설치 시 환경 | 사용 방식 | Codex 업데이트 |
|---|---|---|
| PATH에 `codex`가 있음 | 기존 실행 파일의 경로를 저장해 재사용 | 기존 설치 도구로 관리. 브리지가 교체하거나 삭제하지 않음 |
| PATH에 `codex`가 없음 | 공식 최신 안정 릴리스 다운로드 및 SHA-256 검증 | 실행 시 최신 안정 버전 확인. 다운로드·검증 실패 시 기존 버전 사용 |

어느 방식을 사용해도 브리지는 **별도 인증·설정 저장 공간**을 사용하므로 처음에는 브리지용 ChatGPT 로그인이 필요합니다. 기존 Codex를 실행할 수 없으면 설치를 중단합니다. 저장된 Codex 경로가 사라졌다면 기존 설치를 복구하거나 브리지 설치 명령을 다시 실행하세요.

브리지가 관리하는 Codex 업데이트는 SHA-256과 `--version` 실행을 검증한 뒤 바이너리를 교체합니다. 실행 중인 프로세스에는 다음 재시작부터 적용됩니다. 이 검증이 App Server 프로토콜 호환성을 보장하지는 않습니다.

## 문제 해결

| 증상 | 확인할 내용 |
|---|---|
| 로컬 연결 키 오류 | 터미널의 `#key=`가 포함된 설정 주소로 다시 열고, Risu에 같은 키를 입력 |
| 설정 페이지에 연결할 수 없음 | 실행 터미널이 열려 있는지 확인하고 브리지 재실행 |
| 설정 페이지 테스트는 성공하지만 Risu 연결 실패 | 데스크톱 앱 사용 여부, 전체 요청 주소, 키, 요청 모델 확인 |
| 지원하지 않는 모델·추론 강도 오류 | 설정 페이지에서 사용 가능한 모델과 추론 강도를 선택해 저장 |
| 요청 중 429 오류 | 다른 응답이 생성 중이면 완료 후 재시도. 구독 한도 오류이면 한도 초기화까지 대기 |
| Codex 종료·RPC 오류 | 브리지를 재실행하고 Codex 버전과 프로토콜 호환성 확인 |

설정 페이지의 **사용 범위와 연결 진단**에서 최근 요청의 처리 시간·사용량·무시된 옵션을 확인할 수 있습니다. 진단 정보에 로컬 경로가 포함될 수 있으므로 공유 전 내용을 확인하세요.

## 지원 범위

- 텍스트 입력, 단일 응답, JSON 또는 SSE 스트리밍, 중단과 stop 문자열을 지원합니다. 이미지 태그·감정 마크업 같은 텍스트는 보존하고 Risu가 렌더링합니다.
- `temperature`, `top_p`, `top_k`, `min_p`, `frequency_penalty`, `presence_penalty`, `logit_bias`, `seed`, `max_tokens`, `max_completion_tokens`는 적용되지 않습니다. 무시된 옵션은 JSON의 `bridge.ignored_parameters`, SSE의 `X-Bridge-Ignored-Parameters` 헤더, 설정 페이지 진단에 표시됩니다.
- 도구 호출, 이미지 입력, 구조화 출력, 복수 생성은 거부합니다.
- 한 번에 한 요청을 생성합니다. 입력은 최대 2 MiB, 생성 제한 시간은 180초이며 자동 재시도는 없습니다. 스트리밍 도중 실패하면 오류를 보내고 `[DONE]`은 보내지 않습니다.
- 선택한 AI 계정의 사용량 제한이 적용됩니다. API 키 과금으로 전환하지 않습니다.
- 현재 설치·릴리스 대상은 macOS입니다. 실제 Risu의 50~100턴 대화 품질과 안정성은 별도 실사용 검증 대상입니다.

## 구조와 데이터 처리

```text
Risu → localhost OpenAI-compatible HTTP/SSE → Go 브리지
                                            ├─ ChatGPT: Codex App Server
                                            └─ Gemini: 공식 Gemini CLI ACP
```

Go 표준 라이브러리만 사용하며 설정 페이지 HTML/JS는 바이너리에 포함됩니다. 주요 경로는 `/v1/chat/completions`, `/v1/models`, `/healthz`, 설정 페이지 `/`, 설정·로그인·진단용 `/internal/*`입니다.

**Risu가 대화 기록을 소유합니다.** ChatGPT는 요청마다 새 ephemeral thread를 만들고 완료·취소 후 구독을 해제합니다. 이전 thread를 재사용하지 않아 재생성·편집·분기 시 기존 응답이 별도로 누적되지 않습니다.

ChatGPT의 대화 기록은 실험적 `thread/inject_items`로 역할과 순서를 유지해 전달합니다. 마지막 user 메시지는 `turn/start`에만 전달합니다. 마지막 항목이 user가 아니면 전체 기록 뒤에 다음 assistant 응답을 요청하는 user 입력을 추가합니다. assistant 접두문 이어쓰기는 지원하지 않습니다. 메시지의 `name`은 `[speaker name: "이름"]` 형태로 본문 앞에 보존합니다.

이 입력 방식은 Codex 0.153.0 기준으로 구현되어 있으며, 주입 실패 시 오류를 반환합니다. Codex 자체 지시문도 적용되므로 OpenAI Chat Completions와 완전히 같은 의미의 동작을 보장하지 않습니다. Codex 버전 변경 시 이 실험적 프로토콜의 호환성을 확인해야 합니다.

Gemini는 매 요청 새 CLI 프로세스·세션을 사용하며 ACP `session/update`의 assistant 텍스트만 SSE로 전달합니다. 추론 텍스트는 전달하지 않습니다. 취소·stop·시간 초과 시 해당 subprocess를 종료합니다. **ACP에는 역할별 기록 주입이 없어**, 역할·이름·순서가 포함된 JSON 대화록을 한 user prompt로 전달합니다. Codex의 역할별 입력이나 OpenAI API와 의미적으로 동일하지 않으며 assistant prefill도 지원하지 않습니다. 긴 RP와 프리셋별 품질은 별도 검증이 필요합니다.

Gemini 작업마다 임시 `GEMINI_CLI_HOME`과 작업 디렉터리를 만들고 도구·MCP·확장·skills·hooks·telemetry를 끕니다. 별도 기본 지시문을 적용하고, CLI가 클라이언트 권한이나 도구를 요청하면 거부합니다. 종료 시 갱신된 Google 인증 파일만 `data/gemini/`에 보존하고 임시 기록을 삭제합니다. 강제 종료나 전원 중단 시에는 임시 `session-*` 디렉터리가 남을 수 있습니다. Google 서비스 측 데이터 처리는 별도입니다. Gemini 0.58.0은 고정 설치하며 자동 업데이트하지 않습니다.

브리지는 `127.0.0.1`에만 바인딩하며 Host/Origin과 로컬 bearer 키를 검사합니다. 설정 API는 로컬 Origin만 허용합니다. 별도 Codex 저장 공간에서 history 저장을 끄고 도구·셸·앱·플러그인·외부 검색을 비활성화하며 read-only/never 정책을 적용합니다. 기본 모드에서는 프롬프트와 인증정보를 로그에 기록하지 않습니다. `--debug` 모드에서는 아래 설명대로 요청 전문을 로컬 파일에 기록합니다. 응답 생성은 OpenAI 서비스에서 이루어지며, 이 로컬 설정이 서비스 측 데이터 보관이나 다른 제품 UI의 비노출을 보장하지는 않습니다.

## 개발과 릴리스

Go 1.23 이상이 필요합니다.

```sh
go test -race ./...
go vet ./...
go build -o /tmp/risu-bridge ./cmd/risu-bridge
BRIDGE_CODEX_BIN=/path/to/codex BRIDGE_DATA_DIR=/tmp/risu-bridge-data /tmp/risu-bridge
sh scripts/build-release.sh 0.2.2
```

`dist/`에 macOS arm64/amd64 압축 파일과 `SHA256SUMS`가 생성됩니다. GitHub Release에 업로드한 후 `install.sh`의 고정 브리지 릴리스 버전을 해당 태그로 맞춥니다.

테스트는 가짜 Codex/Gemini subprocess를 사용해 입력 변환, HTTP/SSE, 취소·동시 요청, stop 경계, 설정 저장, RPC 이벤트 순서를 검증합니다. 설치 테스트는 다운로드 fixture를 사용해 기존 Codex 재사용, 파일·파이프 실행, 경로 인용, 데이터 보존, 캐시·업데이트 실패와 종료 신호 처리를 검증합니다. Gemini 인증 흐름, provider별 설정 복원, 텍스트/추론 분리, 사용량 오류, 취소 후 임시 공간 정리도 검증합니다. 실제 구독 모델의 품질 시험을 대체하지 않습니다. `RISU_REAL_GEMINI_BIN=/path/to/gemini go test ./cmd/risu-bridge -run TestGeminiRealInitialize`는 로그인·생성 없이 실제 CLI의 ACP 초기화를 확인합니다.

로컬 바이너리로 설치를 검증하려면:

```sh
BRIDGE_SOURCE_BIN=/tmp/risu-bridge BRIDGE_INSTALL_DIR=/tmp/risu-go-install BRIDGE_INSTALL_ONLY=1 sh install.sh
```

설치 스크립트는 [BASH3 Boilerplate](https://github.com/kvz/bash3boilerplate) 규칙을 적용한 Bash 3.2 호환 본문을 `sh` 진입점으로 실행합니다. strict mode와 실패·신호 정리를 사용하고, 검증된 실행 파일과 launcher를 원자적으로 교체합니다. `--help`, `--install-only`, `--version`, `--restart`를 지원합니다.

### 환경 변수

| 변수 | 용도 |
|---|---|
| `BRIDGE_INSTALL_DIR` | 설치할 절대 경로 |
| `BRIDGE_INSTALL_ONLY=1` | 설치 후 실행하지 않음 |
| `BRIDGE_FORCE_DOWNLOAD=1` | 브리지 관리 Codex의 다운로드 캐시를 사용하지 않음 |
| `BRIDGE_SOURCE_BIN` | 개발용 로컬 브리지 바이너리 |
| `BRIDGE_DATA_DIR` | 인증·키·설정 저장 경로 |
| `BRIDGE_GEMINI_INSTALL_DIR` | Gemini 전용 설치 경로. 기본값은 브리지 설치 폴더 아래 `gemini-cli` |
| `BRIDGE_GEMINI_BIN` | 실행할 Gemini CLI 또는 전용 launcher의 절대 경로 |
| `BRIDGE_CODEX_BIN` | 직접 바이너리를 실행할 때 사용할 Codex 경로. 설치 launcher는 선택한 경로를 지정함 |
| `BRIDGE_PORT` | 사용할 포트 지정 |
| `BRIDGE_OPEN_BROWSER=0` | 설정 페이지 자동 열기 비활성화 |
| `BRIDGE_ACTION` | `reuse`, `stop`, `restart` |
| `BRIDGE_ALLOWED_ORIGINS` | 기본 외부 Origin 목록을 대체할 정확한 Origin 목록. 쉼표 구분, wildcard 미지원 |
| `LOG_LEVEL`, `NO_COLOR=1` | 설치 로그 수준(기본 6, 오류만 3)·색상 비활성화 |

기본 허용 Origin은 `https://risuai.xyz`, `https://risuai.net`, `tauri://localhost`, `http://tauri.localhost`와 브리지 자체 주소입니다. 로컬 Risu 서버는 필요한 Origin을 명시적으로 설정하세요. 외부 터널은 포함되지 않습니다.

### 디버깅 모드: 실제 Risu 입력 확인

빌드한 브리지 실행 파일에 `--debug`를 전달합니다. 이미 실행 중이면 `--restart`도 사용하세요.

```sh
/tmp/risu-bridge --restart --debug
```

Risu에서 ChatGPT 요청을 보내면 데이터 디렉터리(기본 `~/.local/share/risu-subscription-bridge/data`)의 `audit-latest.json`에 최신 유효 요청 1건을 저장합니다. 다음 요청은 기존 파일을 덮어씁니다. 연결 테스트 요청도 포함되며 Gemini 요청은 캡처하지 않습니다. 캡처 저장 실패 시 해당 요청은 모델에 보내지 않고 오류를 반환합니다.

파일에는 허용된 요청 필드와 대화 전문, 설정, 브리지가 전달할 기본 지시문·대화 기록·마지막 입력, 메시지별 문자 수, 기본 지시문 문장과의 대소문자 무시 일치 위치가 들어갑니다. 문자 수는 토큰 수가 아니며, 표현이 다른 의미상 중복은 실제 본문을 읽고 판단해야 합니다. 이는 브리지 경계의 캡처이지 App Server 내부 또는 서비스 측 최종 모델 입력 캡처가 아닙니다.

HTTP 헤더·인증 키용 필드·임의의 추가 필드는 수집하지 않습니다. **대화 본문과 추가 지시사항에 포함된 민감정보는 그대로 저장**됩니다. 파일 권한은 0600이며 자동 만료되지는 않습니다. 분석 후 파일을 삭제하고 `--debug` 없이 재시작하면 기록을 중단합니다. 이 모드는 프롬프트나 생성 설정을 변경하지 않습니다.
