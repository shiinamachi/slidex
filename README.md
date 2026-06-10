# slidex

`slidex`는 Codex Plugin과 Go CLI로 HTML-first 비즈니스 문서를 만드는 로컬
자동화 키트입니다. Codex Plugin은 새 deck 생성 UX의 진입점이고, Go CLI는 생성,
렌더링, QA, 패키징의 구현 원본입니다.

최종 시각 원본은 deck-local `out/final_deck.html`입니다. 기본 납품 파일은 같은
HTML에서 렌더링한 페이지형 `out/final_deck.pdf`입니다.

## 빠른 시작

Codex App에서 새 deck을 시작할 때는 `plugins/slidex` Codex Plugin을 사용합니다.
plugin MCP 설정은 `PATH`의 `slidex` binary를 실행하므로, 로컬 소스 변경 후 먼저
현재 CLI를 설치합니다.

```bash
mise install
mise exec -- go install ./cmd/slidex
```

Codex App local/worktree thread에서 `@slidex` 또는 `slidex-start`를 호출한 뒤
workbench를 시작합니다.

```bash
slidex workbench start --deck-id customer-retention
```

이 명령은 `decks/customer-retention/`을 만들거나 선택하고, `127.0.0.1`에만
bind되는 session-scoped workbench URL을 반환합니다. 반환된
`http://127.0.0.1:<port>/workbench/<session>` URL은 Codex App in-app browser에서
URL 클릭, 수동 navigation, 또는 `@Browser` navigation으로 엽니다.

workbench에서 저장하면 다음 파일만 갱신됩니다.

- `decks/customer-retention/brief.md`
- `decks/customer-retention/out/workbench_draft.json`
- `decks/customer-retention/out/workbench_manifest.json`

이후 표준 문서 생성은 CLI workflow로 실행합니다.

```bash
slidex run --deck decks/customer-retention
```

자료가 부족하면 `out/intake_questions.md`에 한국어 질문을 남기고 exit code `3`으로
멈춥니다. 질문에 답해 `brief.md`를 보강한 뒤 같은 명령을 다시 실행합니다.

## Plugin Workbench 검증

Codex Plugin workbench는 proprietary Canvas mount API를 가정하지 않습니다. 지원되는
표면은 loopback workbench URL을 Codex App browser/work surface에서 여는 방식입니다.

Codex App GUI를 열기 전에는 local HTTP 저장 smoke로 bootstrap과 저장 API를 점검할 수
있습니다.

```bash
slidex workbench save-smoke --workspace <tmp-workspace> --deck-id save-smoke
```

렌더링까지 headless로 확인하려면 `--screenshot`을 추가합니다.

```bash
slidex workbench save-smoke --workspace <tmp-workspace> --deck-id save-smoke --screenshot
```

이 smoke evidence는 Codex App GUI/browser 표시 증거가 아닙니다. 실제 Codex App
browser/work-surface를 확인한 뒤에는 deck-local evidence를 기록합니다.

```bash
slidex workbench evidence --deck-id customer-retention \
  --inspector "<name-or-role>" \
  --surface codex_app_in_app_browser \
  --invocation "@slidex create a deck called customer-retention" \
  --thread-id "<codex-app-thread-id-if-visible>" \
  --url "<workbench.url>" \
  --screenshot "<path-to-codex-browser-screenshot.png>" \
  --workbench-visible \
  --saved-input-verified
```

기록된 evidence가 현재 `brief.md`, draft, manifest와 계속 일치하는지는 다음 명령으로
검증합니다.

```bash
slidex workbench verify-evidence --deck-id customer-retention --require-screenshot
```

plugin/App Server 경로 자체를 headless로 점검하려면 다음 smoke를 사용합니다.

```bash
slidex codex app-server skill-smoke --workspace <tmp-workspace> --deck-id skill-smoke
```

## 작업공간 모델

모든 문서 작업은 `decks/<deck_id>/` 아래에 격리합니다.

```text
decks/<deck_id>/
  brief.md
  DESIGN.md
  assets/
    reference_docs/
    logo.png
    images/
  brand/
    guidelines.md
    colors.json
  data/
    *.csv
    *.xlsx
  source/
    *.pdf
    *.docx
    notes.md
    screenshots/
  out/
    intake_questions.md
    source_inventory.md
    strategy.md
    deck_spec.json
    final_deck.html
    final_deck.generated_baseline.html
    rendered_slides/
      slide_01.png
      slide_02.png
    final_deck.pdf
    render_manifest.json
    qa_montage.png
    qa_report.md
    notes.md
    delivery_summary.md
    html_edit_sync.md
```

`brief.md`, `DESIGN.md`, `assets/`, `brand/`, `data/`, `source/`는 입력입니다.
`out/`은 생성 상태와 납품 산출물입니다. root-level `brief.md`, `assets/`, `brand/`,
`data/`, `out/`은 신규 작업공간 모델이 아닙니다.

## 입력 원칙

가능한 자료만 넣으면 됩니다. 자료가 부족하면 Codex는 현재 자료를 검사한 뒤 한국어
Q&A를 반복해 책임 있게 제작 가능한 수준까지 brief를 보강해야 합니다.

- `brief.md`: 문서 목적, 청중, 의사결정 맥락, 제약
- `DESIGN.md`: 해당 deck에만 적용할 디자인 방향
- `assets/reference_docs/`: 참고 문서, 이미지, 벤치마크 자료
- `assets/logo.png`, `assets/images/`: 사용자가 제공한 로고와 이미지
- `brand/guidelines.md`, `brand/colors.json`: 브랜드 지침과 색상
- `data/*.csv`, `data/*.xlsx`: 차트와 표의 원자료
- `source/`: PDF, DOCX, 스크린샷, 회의 노트, 원문 자료

모든 material claim은 source가 있거나, 사용자 확인을 받았거나, assumption으로 표시해야
합니다. 근거 없는 성과, ROI, 고객 수, 인증, 보안, 특허, 시장 지위, 보장 표현은
제거하거나 다시 써야 합니다.

## 표준 CLI Workflow

```bash
slidex init customer-retention
slidex run --deck decks/customer-retention
```

`slidex run`은 intake gate, strategy, spec, HTML build, baseline, render, QA,
delivery summary, package gate를 한 흐름으로 실행합니다.

단계별 수리나 집중 검증이 필요할 때만 stage 명령을 직접 실행합니다.

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

직접 `out/final_deck.html`을 수정했다면 파생 산출물이 최신이라고 주장하기 전에
동기화합니다.

```bash
slidex sync-html-edits --deck decks/customer-retention
```

## 주요 명령

- `workbench`: Codex Plugin deck creation workbench 시작, 상태 확인, 중지, evidence 기록
- `run`: 표준 workflow를 package gate까지 실행
- `inspect`: deck 입력과 산출물 inventory 출력
- `intake`, `strategy`, `spec`, `build`, `finalize`: 표준 stage 산출물 생성
- `render`: HTML `.slide`를 PNG로 렌더링하고 PDF, manifest, montage 생성
- `qa`: HTML, 렌더 이미지, PDF, manifest, schema, claim provenance 검사
- `package`: 최종 산출물 존재와 render freshness 확인
- `sync-html-edits`: 직접 HTML 수정과 baseline/spec/notes/QA stale 상태 동기화
- `validate-spec`: `deck_spec.json`을 schema 계약에 맞춰 검증
- `doctor`: Go, Chrome, Codex CLI, protocol schema, plugin, workbench 상태 점검
- `codex`: Codex CLI 0.138.0 protocol/schema/doctor/review 보조 명령
- `goal`: deck-local goal state와 가능한 App Server goal API 동기화
- `migrate`, `clean`, `mcp-server`: migration, log retention, MCP stdio surface 제공

기존 `render --html ... --pdf ...` 형태는 advanced override로 유지합니다.

## 런타임과 버전

런타임 관리는 mise를 사용합니다. Go 버전은 `.mise.toml`과 `go.mod` `go` 지시문에서
모두 `1.26.3`으로 exact pin합니다.

```bash
mise install
mise exec -- go version
mise exec -- go test ./...
```

라이브러리와 런타임 버전은 항상 exact version으로 기록합니다. `latest`, `main`,
`master`, `HEAD`, `>=`, `<=`, `>`, `<`, `^`, `~`, `x`, `*` 같은 range 또는 floating
표기는 사용하지 않습니다. 원격 CSS, 폰트, 브라우저/렌더러, 이미지 처리 도구는 exact
version을 기록하거나 로컬에 vendoring하고 SHA-256을 남깁니다.

## 플랫폼 지원

`slidex` CLI와 Codex Plugin 경로는 Windows, Linux, macOS를 지원 대상으로 둡니다.
OS별 기본값은 기능 차이를 만들지 않도록 CLI 내부에서 선택합니다.

- Chrome/Chromium 탐색은 `CHROME_BIN`, `GOOGLE_CHROME_BIN`, `CHROMIUM_BIN`,
  `MSEDGE_BIN`을 먼저 보고, 이후 Windows Chrome/Edge 설치 경로, macOS app bundle,
  Linux PATH 명령명을 자동 탐색합니다.
- `slidex codex app-server start`는 Linux/macOS에서 Unix socket을, Windows에서
  `127.0.0.1` loopback WebSocket을 기본 transport로 사용합니다. 모든 OS에서
  `--listen`으로 명시 override할 수 있습니다.
- plugin doctor helper는 Unix shell용 `scripts/slidex-doctor.sh`와 Windows
  `cmd.exe`용 `scripts/slidex-doctor.cmd`를 함께 제공합니다.

## HTML/PDF 계약

- 정적 HTML/CSS slide structure를 사용합니다.
- `.deck` 루트 안에 `<section class="slide" data-slide-id="slide_01">` 형식의
  안정적인 slide 요소를 둡니다.
- 기본 화면비는 16:9 widescreen, 기본 렌더 크기는 `1920x1080`입니다.
- 지원 preset은 `wide-1080p`, `wide-720p`, `wide-900p`, `wide-1440p`, `custom`입니다.
- 전체 슬라이드를 하나의 PNG로 묻어 HTML 본문으로 쓰지 않습니다.
- CSS 변수로 design token을 관리합니다.
- 한국어 비즈니스 문서는 Pretendard font preset과 Korean-safe wrapping defaults를
  기본으로 사용합니다.
- HTML/PDF 외의 파일 형식을 필수 또는 선택 납품물로 제안하지 않습니다.

## 완료 기준

완료된 deck의 `out/`에는 다음 파일이 필요합니다.

- `strategy.md`
- `deck_spec.json`
- `final_deck.html`
- `final_deck.generated_baseline.html`
- `rendered_slides/*.png`
- `final_deck.pdf`
- `render_manifest.json`
- `qa_montage.png`
- `qa_report.md`
- `notes.md`
- `delivery_summary.md`

최종 성공을 주장하려면 현재 HTML에서 렌더링된 slide PNG와 PDF가 존재하고,
`qa_montage.png` 및 렌더 이미지를 실제로 검사해야 합니다.

## 개발 검증

변경 유형에 맞춰 다음 명령을 사용합니다.

```bash
go test ./...
go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json
go run ./cmd/slidex doctor --json
```

GitHub Actions `Cross Platform` workflow는 `ubuntu-24.04`, `macos-15`,
`windows-2025`에서 같은 검증을 실행하고, Linux/macOS/Windows `amd64`와 `arm64`
test binary cross-compile을 확인합니다. Actions는 SHA로 고정합니다.

template 변경은 임시 deck으로 `slidex init`을 smoke-test한 뒤 해당 deck을 제거합니다.
render/PDF 변경은 현재 HTML에서 PNG/PDF를 다시 만들고 `slidex qa`와 `slidex package`
freshness를 확인합니다.
