# slidex

[English documentation](README.md)

`slidex`는 HTML-first 비즈니스 문서와 페이지형 PDF를 만들기 위한 로컬 CLI 및
Codex Plugin workflow입니다. 각 문서 프로젝트를 독립된 deck workspace로 관리하고,
정적 HTML slide를 만든 뒤 slide PNG와 PDF로 렌더링하며, 공유 가능한 상태인지
delivery artifact의 최신성을 검사합니다.

편집 가능한 시각 원본은 다음 파일입니다.

```text
decks/<deck_id>/out/final_deck.html
```

기본 납품 파일은 다음 파일입니다.

```text
decks/<deck_id>/out/final_deck.pdf
```

## slidex의 용도

`slidex`는 비즈니스 프레젠테이션, 제안서, 임원 보고서, 투자자 자료, 정부 심의
문서, 고객 의사결정 deck처럼 로컬에서 재현 가능한 문서 제작 workflow가 필요할 때
사용합니다.

`slidex`는 hosted web app이 아닙니다. 사용자의 컴퓨터에서
`decks/<deck_id>/` 아래에 파일을 쓰며, 최종 package는 로컬 HTML, PNG, PDF, JSON,
Markdown artifact로 구성됩니다.

## 설치

### 요구 사항

- Windows, macOS, Linux 중 하나
- HTML을 PNG/PDF로 렌더링하기 위한 Chrome, Chromium, 또는 Microsoft Edge
- 선택 사항: bundled Codex Plugin을 사용할 경우 Codex App 또는 Codex CLI
- source build 전용: Git과 `mise`

### 권장: GitHub Release package로 설치

일반 사용자는 packaged release를 사용합니다. package에는 `slidex` binary,
deck template, schema, Codex plugin file, repo marketplace가 함께 들어 있습니다.
Release package는 GitHub Actions에서 빌드합니다. code signing은 이후 작업으로
미룹니다.

설치 절차는 다음 파일을 따릅니다.

```text
INSTALL.md
```

Codex App one-shot prompt:

```text
Install slidex from https://github.com/shiinamachi/slidex. Follow INSTALL.md in that repository: resolve the published GitHub Release tag first, use that exact tag's package for this OS/CPU, verify the SHA-256 checksum, add slidex to PATH, install the included Codex plugin with `codex plugin marketplace add <install-dir>` and `codex plugin add slidex@slidex-local` when Codex CLI is available, run `slidex --help` and `slidex doctor --render`, then report any manual follow-up.
```

### Source build fallback

개발 중이거나 대상 platform용 release package가 없을 때만 이 경로를 사용합니다. 이
저장소는 `.mise.toml`과 `go.mod`에서 Go 버전을 정확히 pin합니다. source build
명령은 저장소 root에서 실행합니다.

```bash
git clone https://github.com/shiinamachi/slidex.git
cd slidex
mise install
mise exec -- go install ./cmd/slidex
```

Go install directory를 `PATH`에 추가합니다.

macOS 또는 Linux:

```bash
export PATH="$(mise exec -- go env GOPATH)/bin:$PATH"
```

Windows PowerShell:

```powershell
$env:Path = "$(mise exec -- go env GOPATH)\bin;$env:Path"
```

Windows `cmd.exe`:

```bat
for /f "delims=" %G in ('mise exec -- go env GOPATH') do set "PATH=%G\bin;%PATH%"
```

다음 terminal session에서도 `slidex`를 사용하려면 같은 PATH 설정을 shell profile이나
system environment에 저장합니다.

설치를 확인합니다.

```bash
slidex --help
slidex doctor --render
```

`slidex` 명령을 찾을 수 없다면, 다음 명령이 출력하는 경로 아래의 `bin` directory가
`PATH`에 포함되어 있는지 확인합니다.

```bash
mise exec -- go env GOPATH
```

### 기존 설치 업데이트

Release package 설치는 `INSTALL.md`의 release package 절차를 새 release tag에 대해
반복합니다.

Source build 설치는 다음을 실행합니다.

```bash
git pull
mise install
mise exec -- go install ./cmd/slidex
slidex doctor --render
```

Codex Plugin은 `PATH`에서 `slidex` binary를 찾습니다. plugin을 사용한다면 저장소
변경 사항을 pull한 뒤 CLI를 다시 설치해야 합니다.

## 빠른 시작

새 deck workspace를 만듭니다.

```bash
slidex init customer-retention
```

생성된 brief를 편집합니다.

```text
decks/customer-retention/brief.md
```

필요한 source material, brand file, image, spreadsheet, note를 같은 deck directory
아래에 추가합니다. 그다음 표준 workflow를 실행합니다.

```bash
slidex run --deck decks/customer-retention
```

실행이 끝나면 delivery summary를 엽니다.

```text
decks/customer-retention/out/delivery_summary.md
```

사용자가 주로 검토하는 파일은 다음과 같습니다.

```text
decks/customer-retention/out/final_deck.html
decks/customer-retention/out/final_deck.pdf
decks/customer-retention/out/qa_montage.png
decks/customer-retention/out/qa_report.md
decks/customer-retention/out/delivery_summary.md
```

## Deck Workspace

모든 문서 프로젝트는 `decks/<deck_id>/` 아래에 있습니다.

```text
decks/<deck_id>/
  brief.md
  DESIGN.md
  assets/
    reference_docs/
    images/
  brand/
    guidelines.md
    colors.json
  data/
  source/
  out/
```

입력 파일:

- `brief.md`: 문서 목표, 청중, 의사결정 맥락, 제약, source note
- `DESIGN.md`: 해당 deck에만 적용되는 시각 방향
- `assets/`: 로고, 제품 이미지, reference image, 지원 파일
- `brand/`: brand guideline, color, font, 관련 규칙
- `data/`: chart나 table의 원천 데이터인 CSV, XLSX 등
- `source/`: PDF, DOCX, screenshot, 회의 note, 기타 evidence

생성 파일:

- `out/strategy.md`
- `out/deck_spec.json`
- `out/final_deck.html`
- `out/final_deck.generated_baseline.html`
- `out/rendered_slides/*.png`
- `out/final_deck.pdf`
- `out/render_manifest.json`
- `out/qa_montage.png`
- `out/qa_report.md`
- `out/notes.md`
- `out/delivery_summary.md`

portable deck ID를 사용합니다. deck ID는 문자 또는 숫자로 시작해야 하며, 문자,
숫자, `_`, `-`, `.`를 사용할 수 있습니다. Windows 예약 device name인 `CON`,
`NUL`, `COM1`, `LPT1` 같은 이름은 피합니다.

## 입력 준비

명확한 `brief.md`에서 시작합니다. 최소한 다음 내용을 포함합니다.

- 문서가 답해야 하는 비즈니스 질문
- 청중과 의사결정자
- 원하는 결정 또는 다음 단계
- 필요한 언어, tone, 길이, format
- 사용이 승인된 사실
- source evidence가 필요하거나 assumption으로 다뤄야 하는 claim

`slidex run`을 실행하기 전에 supporting material을 deck workspace에 넣습니다.
모든 material claim은 source가 있거나, 사용자가 확인했거나, assumption으로 표시되어야
합니다. ROI, 시장 지위, 고객 수, 인증, 보안, 특허, 보장, outcome에 관한 근거 없는
claim은 delivery 전에 제거하거나 다시 작성해야 합니다.

## Workflow 실행

표준 명령은 다음과 같습니다.

```bash
slidex run --deck decks/customer-retention
```

이 명령은 intake, strategy, spec, HTML build, baseline creation, rendering, QA,
delivery summary, package check를 실행합니다.

자료가 충분하지 않으면 `slidex`는 exit code `3`으로 멈추고 질문을 다음 파일에
작성합니다.

```text
decks/customer-retention/out/intake_questions.md
```

질문에 대한 답을 `brief.md`에 추가한 뒤 같은 명령을 다시 실행합니다.

workflow의 특정 부분만 검사하거나 수리해야 할 때만 stage command를 직접 사용합니다.

```bash
slidex inspect --deck decks/customer-retention --write
slidex intake --deck decks/customer-retention
slidex strategy --deck decks/customer-retention
slidex spec --deck decks/customer-retention
slidex build --deck decks/customer-retention
slidex render --deck decks/customer-retention
slidex qa --deck decks/customer-retention
slidex finalize --deck decks/customer-retention
slidex package --deck decks/customer-retention
```

## 결과물 검토

deck을 공유하기 전에 다음 파일을 확인합니다.

- `out/final_deck.pdf`
- `out/qa_montage.png`
- `out/rendered_slides/*.png`
- `out/qa_report.md`
- `out/delivery_summary.md`

`slidex package --deck decks/<deck_id>`는 필요한 delivery file이 존재하고, 렌더링된
artifact가 현재 HTML과 일치하는지 검증합니다.

`out/final_deck.html`을 직접 수정했다면 PDF나 PNG가 최신이라고 보기 전에 edit를
sync합니다.

```bash
slidex sync-html-edits --deck decks/customer-retention
slidex render --deck decks/customer-retention
slidex qa --deck decks/customer-retention
slidex package --deck decks/customer-retention
```

## 선택 사항: Codex Plugin Workbench

이 저장소는 `plugins/slidex`에 local Codex Plugin을 포함합니다. plugin은 local
loopback workbench를 통해 deck brief를 만드는 front door입니다. build, render, QA,
package stage의 source of truth는 계속 CLI입니다.

plugin을 사용하기 전에 설치된 `slidex` binary가 최신인지 확인합니다.

```bash
mise exec -- go install ./cmd/slidex
```

workbench를 시작합니다.

```bash
slidex workbench start --deck-id customer-retention
```

반환된 `http://127.0.0.1:<port>/workbench/<session>` URL을 Codex App browser에서
엽니다. workbench를 저장하면 다음 파일이 작성됩니다.

```text
decks/customer-retention/brief.md
decks/customer-retention/out/workbench_draft.json
decks/customer-retention/out/workbench_manifest.json
```

이후 일반 workflow를 실행합니다.

```bash
slidex run --deck decks/customer-retention
```

## 문제 해결

CLI와 render environment를 확인합니다.

```bash
slidex doctor --render
```

Chrome이 감지되지 않으면 다음 environment variable 중 하나에 browser binary path를
설정합니다.

```text
CHROME_BIN
GOOGLE_CHROME_BIN
CHROMIUM_BIN
MSEDGE_BIN
CHROME_FOR_TESTING_BIN
PLAYWRIGHT_CHROMIUM_BIN
PLAYWRIGHT_CHROME_BIN
PUPPETEER_EXECUTABLE_PATH
```

deck별 점검:

```bash
slidex doctor --deck decks/customer-retention --render
slidex inspect --deck decks/customer-retention
```

schema validation:

```bash
slidex validate-spec --spec decks/customer-retention/out/deck_spec.json
```

## Command Reference

자주 쓰는 명령:

```text
slidex init <deck_id>
slidex run --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
slidex sync-html-edits --deck decks/<deck_id>
slidex doctor --deck decks/<deck_id> --render
slidex workbench start --deck-id <deck_id>
```

전체 command list는 `slidex --help`에서 확인합니다. Advanced render override와
Codex-specific command 예시는 `commands.md`를 참고합니다.
