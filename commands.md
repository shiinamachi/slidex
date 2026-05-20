# 명령어 가이드

이 문서는 `slidex` 프롬프트 시스템과 로컬 CLI를 사용해
HTML-first 비즈니스 문서와 페이지형 PDF를 만드는 명령을 정리합니다.

## 작업공간 만들기

```bash
cp -R decks/_template decks/customer-retention
```

그 뒤 `decks/customer-retention/brief.md`를 작성하고 필요한 `assets/`,
`brand/`, `data/`, `source/` 자료를 추가합니다. 스타일 방향이 있으면
`decks/customer-retention/DESIGN.md`에 작성합니다.

## 런타임 준비

Go 런타임은 mise로 exact pin합니다. 현재 핀은 `.mise.toml`과 `go.mod`의
`go` 지시문에 기록된 `1.26.3`입니다.

```bash
mise install
mise exec -- go version
mise exec -- go install ./cmd/slidex
```

## Primary CLI Workflow

기본 실행은 prompt 파일을 직접 조합하지 않고 `slidex run`을 사용합니다.

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
mise exec -- slidex migrate --deck decks/customer-retention --from legacy-html-pdf
```

문서와 acceptance 기준의 canonical 이름은 `slidex`입니다.

## Advanced Prompt Fallback

직접 prompt 파일을 실행하는 방식은 CLI가 없는 환경이나 디버깅용 fallback입니다.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_start_business_doc.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/one_shot_create_business_doc.md
```

## 샌드박스

- 일반 프롬프트 실행에는 `--sandbox workspace-write`를 사용합니다.
- 알 수 없는 외부 자료를 다룰 때는 불필요한 네트워크 접근과 전체 파일시스템
  접근을 피합니다.

## 설치와 배포

```bash
mise exec -- go install ./cmd/slidex
export PATH="$(go env GOPATH)/bin:$PATH"
mise exec -- go test ./...
```

배포 전에는 Go/Codex exact pin, `go.sum`, vendored App Server protocol schema,
companion skill/plugin package, hook manifest, README/commands 문서, shell completion
생성 절차를 함께 검증합니다.
