# Risu Subscription Bridge

RisuAI의 캐릭터·로어북·대화 UI를 유지하면서 ChatGPT 구독 인증을 사용하는 로컬 브리지입니다. **Go 실행 파일**이 공식 Codex App Server를 실행하고, OpenAI-compatible HTTP/SSE 요청을 변환합니다. API 키 과금 fallback은 없습니다. 구독의 사용량 제한은 적용됩니다.

## Mac에서 명령어 하나로 설치·실행

```sh
curl -fsSL https://raw.githubusercontent.com/risu-harness/risu-subscription-bridge/main/install.sh | sh
```

Node, Python, Go, npm, Homebrew, Git, sudo가 필요하지 않습니다. 설치 스크립트가 Mac CPU를 확인하고 GitHub Releases에서 미리 빌드한 Go 브리지와 공식 Codex 0.153.0 네이티브 실행 파일을 내려받아 SHA-256을 검증한 뒤 실행합니다. Apple Silicon과 Intel용 파일을 제공합니다. 첫 ChatGPT 로그인은 직접 진행해야 합니다.

설치 위치는 `~/.local/share/risu-subscription-bridge`입니다. 실행 후 열리는 설정 페이지에서 **ChatGPT 로그인 → 공식 로그인 링크 → 상태 확인** 순으로 진행하세요. 터미널을 열어 두어야 하며 Ctrl+C로 종료합니다. 백그라운드 서비스는 아닙니다.

재실행도 같은 curl 명령을 사용하거나 다음 명령을 실행합니다.

```sh
sh "$HOME/.local/share/risu-subscription-bridge/bin/risu-bridge"
```

실행 중인 인스턴스가 있으면 **1) 재사용 / 2) 종료 / 3) 최신 버전으로 재시작**을 선택합니다. Node 버전에서 Go 버전으로 업데이트할 때는 3번을 선택하세요. 기존 프로세스는 설치만으로 교체되지 않습니다. 종료·재시작하면 진행 중인 응답은 중단됩니다. 비대화형 환경에서는 기존 인스턴스를 재사용합니다.

```sh
sh "$HOME/.local/share/risu-subscription-bridge/bin/risu-bridge" --restart
```

기존 `data/`의 ChatGPT 로그인, 로컬 연결 키, 모델·추론 강도·답변 상세도·추가 지시사항 설정은 보존합니다. 이전 Node 런타임과 옛 릴리스는 자동 삭제하지 않지만 Go 버전 실행에는 사용하지 않습니다. 아주 오래된 버전이 종료 API를 지원하지 않으면 해당 터미널에서 Ctrl+C 후 다시 실행하세요.

## Risu 연결

**공식 RisuAI 웹 버전은 localhost 요청을 자체 차단합니다. Mac에서는 [RisuAI 데스크톱 앱](https://github.com/kwaroran/RisuAI/releases/latest)을 사용하세요.** CORS를 바꾸는 것만으로 웹 버전의 이 검사를 해결할 수 없습니다.

Risu 설정 → 채팅 봇 → 모델에서 다음 값을 입력합니다.

| 항목 | 값 |
|---|---|
| 모델 | Custom API |
| URL | 설정 페이지에 표시되는 전체 `/v1/chat/completions` 주소 |
| 키/패스워드 | 설정 페이지의 **로컬 연결 키** |
| 요청 모델 | `subscription-default` |
| 포맷 | OpenAI Compatible |
| 토크나이저 | Tiktoken (OpenAI) |
| Response 스트리밍 | 켜기 가능 |
| Ooba 모드 | 끄기 |

기본 포트는 8787이고 다른 프로그램이 사용 중이면 8799까지 빈 포트를 선택합니다. 예를 들어 실제 포트가 8788이면 URL은 `http://127.0.0.1:8788/v1/chat/completions`입니다. 요청 모델의 Claude 예시 값은 반드시 교체하세요. 키는 ChatGPT 비밀번호나 OpenAI API 키가 아닙니다.

설정 페이지에서 모델, 해당 모델의 추론 강도, 답변 상세도, 추가 지시사항을 저장할 수 있습니다. `subscription-default` 요청에는 저장한 모델을 사용합니다. Risu에서 명시적인 모델 ID를 보내면 그 모델이 우선하며, 저장한 추론 강도가 호환되지 않으면 오류를 반환합니다.

먼저 설정 페이지의 연결 테스트를 실행하고, Risu에서 짧은 텍스트 대화를 시험하세요. 일반 루트 주소에서 `Local bridge key required`가 나오면 터미널의 **`#key=`가 포함된 설정 페이지 주소**를 다시 여세요. 해당 주소와 로컬 키를 공개하지 마세요.

## 동작과 한계

- Risu가 대화 기록을 소유합니다. 요청마다 새 `ephemeral` thread를 만들고 완료·취소 후 `thread/unsubscribe`합니다. 과거 thread를 재사용하지 않으므로 재생성·편집·분기에 이전 응답이 중복 누적되지 않습니다.
- App Server의 실제 `item/agentMessage/delta`를 SSE로 전송합니다. CLI `exec` 생성 방식은 이전 변경에서 제거되었으며, Go 버전도 현재 App Server 동작을 유지합니다.
- system/developer/user/assistant 텍스트와 이름을 순서 있는 JSON 대화록으로 전달합니다. API의 네이티브 역할 채널과 완전히 같은 의미는 아닙니다.
- `temperature`, `top_p`, `top_k`, `min_p`, penalties, `logit_bias`, `seed`, `max_tokens`, `max_completion_tokens`는 적용되지 않습니다. JSON의 `bridge.ignored_parameters`, SSE 헤더, 상태 페이지에서 확인할 수 있습니다. 특히 API식 출력 토큰 상한을 보장하지 않습니다.
- 도구 호출, 이미지 입력, 구조화 출력, 복수 생성은 거부합니다. 문자로 표현된 이미지 태그·감정 마크업은 보존하며 실제 렌더링은 Risu가 담당합니다.
- 한 번에 한 요청, 생성 제한 시간 180초, 입력 2 MiB 제한입니다. 동시 요청에는 429를 반환하며 자동 재시도는 없습니다. SSE 도중 실패하면 오류 이벤트를 보내고 `[DONE]`을 보내지 않습니다.
- 브리지는 프롬프트나 인증정보를 로그에 기록하지 않습니다. Codex의 별도 저장 공간, history none, ephemeral을 사용하지만 서비스 측 보관이나 모든 제품 UI의 비노출을 보장하지 않습니다.
- `127.0.0.1` 바인딩, 정확한 Host/Origin 검사, 로컬 bearer 키, 설정 API의 로컬 Origin 제한을 유지합니다. 도구·셸·앱·플러그인·외부 검색을 비활성화하고 read-only/never 정책을 사용합니다.

## 개발 및 릴리스

Go 1.23 이상이 필요합니다. 표준 라이브러리만 사용하며 설정 페이지 HTML/JS를 바이너리에 embed합니다. 페이지의 JavaScript는 브라우저에서 실행되며 Node 런타임은 필요하지 않습니다.

```sh
go test -race ./...
go vet ./...
go build -o /tmp/risu-bridge ./cmd/risu-bridge
BRIDGE_CODEX_BIN=/path/to/codex BRIDGE_DATA_DIR=/tmp/risu-bridge-data /tmp/risu-bridge
sh scripts/build-release.sh 0.2.0
```

`dist/`에 macOS arm64/amd64 압축 파일과 `SHA256SUMS`가 생성됩니다. 릴리스 시 바이너리와 checksum을 GitHub Release에 업로드한 후 `install.sh`의 고정 릴리스 버전을 해당 태그로 맞춥니다. 테스트는 가짜 Codex subprocess를 포함하여 HTTP/SSE, 취소·동시 실행, stop 경계, 입력 검증, 설정 저장, RPC 알림/응답 순서 경합을 검증합니다. 실제 구독 모델 품질이나 50~100턴 Risu 대화 시험을 대체하지 않습니다.

개발용 로컬 바이너리 설치 검증:

```sh
BRIDGE_SOURCE_BIN=/tmp/risu-bridge BRIDGE_INSTALL_DIR=/tmp/risu-go-install BRIDGE_INSTALL_ONLY=1 sh install.sh
```

설치 환경 변수: `BRIDGE_INSTALL_DIR`, `BRIDGE_INSTALL_ONLY=1`, `BRIDGE_FORCE_DOWNLOAD=1`, 개발 전용 `BRIDGE_SOURCE_BIN`.
실행 환경 변수: `BRIDGE_DATA_DIR`, `BRIDGE_CODEX_BIN`, `BRIDGE_PORT`, `BRIDGE_OPEN_BROWSER=0`, `BRIDGE_ACTION=reuse|stop|restart`, `BRIDGE_ALLOWED_ORIGINS`(정확한 Origin을 쉼표로 구분; wildcard 미지원).

기본 허용 Origin: `https://risuai.xyz`, `https://risuai.net`, `tauri://localhost`, `http://tauri.localhost`, 브리지 자체 주소. 로컬 Risu 서버 등은 필요한 Origin을 명시적으로 추가하세요. 외부 터널은 기본 설치에 포함되지 않습니다.
