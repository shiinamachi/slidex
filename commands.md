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

Go와 Node 런타임 및 Electron 보일러플레이트용 pnpm은 mise로 exact pin합니다.
현재 Go 핀은 `.mise.toml`과 `go.mod`의 `go` 지시문에 기록된 `1.26.3`이고,
Node 핀은 `.mise.toml`의 `24.16.0`, pnpm 핀은 `11.5.2`입니다.

```bash
mise install
mise exec -- go version
mise exec -- go install ./cmd/slidex
```

## Primary CLI Workflow

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

## Codex, Goal, Migration

```bash
mise exec -- slidex doctor --deck decks/customer-retention --codex --render --json
mise exec -- slidex codex schema refresh --codex-version 0.132.0
mise exec -- slidex codex app-server probe
mise exec -- slidex codex review --deck decks/customer-retention --stage delivery
mise exec -- slidex goal set --deck decks/customer-retention --objective "현재 HTML/PDF 산출물의 package gate 통과"
mise exec -- slidex goal status --deck decks/customer-retention
mise exec -- slidex migrate --deck decks/customer-retention --from html-pdf
```

`/goal`은 Codex TUI slash command이고, `slidex goal`은 deck state와 App Server
goal API를 동기화하는 CLI wrapper입니다. 자동화나 CI에서는 `slidex goal`을
사용합니다.

문서와 acceptance 기준의 canonical 이름은 `slidex`입니다.

## Desktop GUI Boilerplate

Electron 앱 보일러플레이트는 `apps/desktop/` 아래에 있습니다. 현재는 GUI shell,
preload IPC, Vite + React renderer, TypeScript build, packaging 설정만 포함하며
실제 `slidex` CLI 실행 연결은 다음 구현 단계의 범위입니다.

```bash
cd apps/desktop
mise exec -- pnpm install
mise exec -- pnpm run dev
mise exec -- pnpm run typecheck
mise exec -- pnpm run build
```

## 설치와 배포

```bash
mise exec -- go install ./cmd/slidex
export PATH="$(go env GOPATH)/bin:$PATH"
mise exec -- go test ./...
```

배포 전에는 Go/Codex exact pin, `go.sum`, vendored App Server protocol schema,
companion skill/plugin package, hook manifest, README/commands 문서, shell completion
생성 절차를 함께 검증합니다.
