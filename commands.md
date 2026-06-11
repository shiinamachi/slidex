# 명령어 가이드

이 문서는 `slidex` CLI로 HTML-first 비즈니스 문서와 페이지형 PDF를 만드는
명령을 정리합니다.

## 작업공간 만들기

```bash
mise exec -- slidex init customer-retention
```

그 뒤 `decks/customer-retention/brief.md`를 작성하고 필요한 `assets/`,
`brand/`, `data/`, `source/` 자료를 추가합니다. 스타일 방향이 있으면
`decks/customer-retention/DESIGN.md`에 작성합니다.

## 런타임 준비

Go 런타임은 mise로 exact pin합니다. 현재 Go 핀은 `.mise.toml`과 `go.mod`의
`go` 지시문에 기록된 `1.26.3`입니다.

```bash
mise install
mise exec -- go version
mise exec -- go install ./cmd/slidex
```

## 플랫폼 지원

지원 대상 OS는 Windows, Linux, macOS입니다. 렌더링은 OS별 Chrome/Chromium 또는
Microsoft Edge 설치 위치를 자동 탐색하며, 필요하면 `CHROME_BIN` 또는 `--chrome`으로
명시할 수 있습니다. App Server managed mode는 Linux/macOS에서 Unix socket을 우선
선택하고 OS socket path 제한을 넘으면 `127.0.0.1` loopback WebSocket으로 폴백합니다.
Windows에서는 `127.0.0.1` loopback WebSocket을 기본 transport로 선택합니다.

## Primary CLI Workflow

새 deck creation을 Codex App에서 시작할 때는 plugin React workbench를 사용합니다.
이 경로는 새 deck 생성의 필수 진입점입니다. `slidex init`, 수동 디렉터리 생성,
`out/final_deck.html` 직접 작성으로 폴백하지 않습니다.

```bash
mise exec -- slidex workbench start --deck-id customer-retention
```

반환된 loopback URL은 Codex App in-app browser에서 Browser plugin / `@Browser`
navigation을 우선 시도하고, 불가능하면 URL 클릭 또는 수동 navigation으로 엽니다.
Public Codex 0.138.0 계약에는 plugin-owned arbitrary Canvas mount 또는 직접
browser-open request API가 확인되지 않았습니다.
Workbench는 로컬 React Wizard로 표시되며, 사용자가 `Complete & generate`를 선택하면
`brief.md`, `out/workbench_draft.json`, `out/workbench_manifest.json`을 저장하고
`slidex run --deck decks/<deck_id> --non-interactive`를 백그라운드로 시작합니다.
생성 상태와 로그 경로는 `out/workbench_manifest.json`의 `generationStatus` 및
`generationLogPath`에 기록됩니다. Codex Plugin MCP는 PATH의 `slidex`를 실행하므로
local plugin 검증 전 `mise exec -- go install ./cmd/slidex`로 설치 binary를 현재 소스와
맞춥니다.
기본 `decks/_template`은 CLI 바이너리에 내장되어 있어 사용자 workspace에 템플릿
폴더가 없어도 `workbench start`가 새 deck workspace를 만들 수 있습니다.

기본 실행은 `slidex run`을 사용합니다.

```bash
mise exec -- slidex run --deck decks/customer-retention
```

자료가 부족하면 `out/intake_questions.md`에 한국어 질문을 남기고 exit code `3`으로
멈춥니다. 수리나 재실행이 필요할 때만 stage 명령을 직접 사용합니다.

```bash
mise exec -- slidex inspect --deck decks/customer-retention --write
mise exec -- slidex intake --deck decks/customer-retention
mise exec -- slidex strategy --deck decks/customer-retention
mise exec -- slidex spec --deck decks/customer-retention
mise exec -- slidex build --deck decks/customer-retention
mise exec -- slidex render --deck decks/customer-retention
mise exec -- slidex qa --deck decks/customer-retention
mise exec -- slidex finalize --deck decks/customer-retention
mise exec -- slidex package --deck decks/customer-retention
```

## Advanced Render Override

Bash:

```bash
mise exec -- slidex render \
  --html decks/customer-retention/out/final_deck.html \
  --out decks/customer-retention/out/rendered_slides \
  --pdf decks/customer-retention/out/final_deck.pdf \
  --manifest decks/customer-retention/out/render_manifest.json \
  --pdf-mode paginated \
  --selector .slide \
  --width 1920 \
  --height 1080 \
  --font-preset pretendard
```

PowerShell:

```powershell
$env:CHROME_BIN = "C:\Program Files\Google\Chrome\Application\chrome.exe"
mise exec -- slidex render `
  --html decks/customer-retention/out/final_deck.html `
  --out decks/customer-retention/out/rendered_slides `
  --pdf decks/customer-retention/out/final_deck.pdf `
  --manifest decks/customer-retention/out/render_manifest.json `
  --pdf-mode paginated `
  --selector .slide `
  --width 1920 `
  --height 1080 `
  --font-preset pretendard
Remove-Item Env:\CHROME_BIN
```

Windows `cmd.exe`:

```bat
set "CHROME_BIN=C:\Program Files\Google\Chrome\Application\chrome.exe"
mise exec -- slidex render ^
  --html decks/customer-retention/out/final_deck.html ^
  --out decks/customer-retention/out/rendered_slides ^
  --pdf decks/customer-retention/out/final_deck.pdf ^
  --manifest decks/customer-retention/out/render_manifest.json ^
  --pdf-mode paginated ^
  --selector .slide ^
  --width 1920 ^
  --height 1080 ^
  --font-preset pretendard
set "CHROME_BIN="
```

## Codex, Goal, Migration

Bash:

```bash
mise exec -- slidex doctor --deck decks/customer-retention --codex --render --json
mise exec -- slidex codex schema refresh --codex-version 0.138.0
mise exec -- slidex codex app-server probe
mise exec -- slidex codex review --deck decks/customer-retention --stage delivery
mise exec -- slidex goal set --deck decks/customer-retention --objective "현재 HTML/PDF 산출물의 package gate 통과"
mise exec -- slidex goal status --deck decks/customer-retention
mise exec -- slidex migrate --deck decks/customer-retention --from html-pdf
```

PowerShell:

```powershell
mise exec -- slidex doctor --deck decks/customer-retention --codex --render --json
mise exec -- slidex workbench start --deck-id customer-retention
mise exec -- slidex goal set --deck decks/customer-retention --objective "현재 HTML/PDF 산출물의 package gate 통과"
mise exec -- slidex goal status --deck decks/customer-retention
```

Windows `cmd.exe`:

```bat
mise exec -- slidex doctor --deck decks/customer-retention --codex --render --json
mise exec -- slidex workbench start --deck-id customer-retention
mise exec -- slidex goal set --deck decks/customer-retention --objective "현재 HTML/PDF 산출물의 package gate 통과"
mise exec -- slidex goal status --deck decks/customer-retention
```

`/goal`은 Codex TUI slash command이고, `slidex goal`은 deck state와 App Server
goal API를 동기화하는 CLI wrapper입니다. 자동화나 CI에서는 `slidex goal`을
사용합니다.

문서와 acceptance 기준의 canonical 이름은 `slidex`입니다.

## 설치와 배포

일반 사용자에게 보여줄 설치 안내는 한 줄짜리 Codex App one-shot prompt만 사용합니다.
실제 release package 설치, checksum 검증, Codex plugin setup 절차는 Codex가
저장소 안의 `INSTALL.md`를 읽고 수행합니다. 기본 설치는 GitHub CLI 설치나
GitHub 로그인을 요구하지 않으며, release package 무결성은 SHA-256 checksum으로
검증합니다.

Production channel:

```text
Install slidex production build from https://github.com/shiinamachi/slidex. Read INSTALL.md in that repository and follow the production channel install instructions.
```

Canary channel:

```text
Install slidex canary build from https://github.com/shiinamachi/slidex. Read INSTALL.md in that repository and follow the canary channel install instructions; keep canary separate from any existing production install.
```

Release package update:

```bash
slidex update status --json
slidex update check --json
slidex update apply --yes --json
slidex codex app-server plugin-smoke --json
slidex update verify --json
```

On Windows, `update apply` may report `pendingActivation: true` if the active
executable could be locked. Complete that staged handoff before plugin smoke by
running the reported `pendingActivationCommand`. It uses an activator binary
outside the old install root and staged candidate so the directories can be
renamed safely:

```bash
<pendingActivationCommand from update status>
```

`update apply`는 release archive와 checksum 파일의 SHA-256 검증만 수행합니다.
GitHub CLI 로그인은 사용하지 않습니다.

개발자 source build:

```bash
mise exec -- go install ./cmd/slidex
export PATH="$(mise exec -- go env GOPATH)/bin:$PATH"
mise exec -- go test ./...
```

Windows PowerShell에서는 `export` 대신 다음 형태를 사용합니다.

```powershell
$env:Path = "$(mise exec -- go env GOPATH)\bin;$env:Path"
mise exec -- go test ./...
```

Windows `cmd.exe`에서는 다음 형태를 사용합니다.

```bat
for /f "delims=" %G in ('mise exec -- go env GOPATH') do set "PATH=%G\bin;%PATH%"
mise exec -- go test ./...
```

배포 전에는 Go/Codex exact pin, `go.sum`, vendored App Server protocol schema,
companion skill/plugin package, hook manifest, CLI/plugin version lock,
README/commands 문서, shell completion 생성 절차를 함께 검증합니다.
CLI와 Codex Plugin 버전 운영 규칙은 [VERSIONING.md](VERSIONING.md)를 따릅니다.

GitHub Actions `Cross Platform` workflow는 `ubuntu-24.04`, `macos-15`,
`windows-2025` 테스트와 Linux/macOS/Windows `amd64`, `arm64` cross-compile을
실행합니다. `v*` tag push에서는 같은 workflow의 `Release Binaries` job이 release
package와 SHA-256 checksum file을 만들고 GitHub Release asset으로 업로드합니다.
