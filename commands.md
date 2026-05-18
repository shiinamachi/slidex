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

## 단계별 프롬프트

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_start_business_doc.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/01_create_business_strategy.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/02_create_business_doc_spec.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/03_build_html_deck.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/04_render_html.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/05_business_visual_qa.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/06_revise_html_deck.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/07_sync_user_html_edits.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/08_finalize_business_delivery.md
```

권장 순서는 intake gate, 전략, spec, HTML 작성, 렌더링/PDF 생성, QA, 수정,
HTML edit sync, 최종 납품 검증입니다.

## 원샷 실행

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/one_shot_create_business_doc.md
```

원샷 프롬프트는 intake gate가 완료되지 않았으면 멈추고 한국어 질문을 해야
합니다. brief가 불완전한 상태에서 내용을 꾸며내면 안 됩니다.

## CLI 렌더링

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

## CLI 명령

```bash
mise exec -- slidex inspect --deck decks/customer-retention
mise exec -- slidex validate-spec --spec decks/customer-retention/out/deck_spec.json
mise exec -- slidex render --html decks/customer-retention/out/final_deck.html --pdf decks/customer-retention/out/final_deck.pdf
mise exec -- slidex qa --deck decks/customer-retention
mise exec -- slidex sync-html-edits --deck decks/customer-retention
mise exec -- slidex package --deck decks/customer-retention
```

문서와 acceptance 기준의 canonical 이름은 `slidex`입니다.

## 샌드박스

- 일반 프롬프트 실행에는 `--sandbox workspace-write`를 사용합니다.
- 알 수 없는 외부 자료를 다룰 때는 불필요한 네트워크 접근과 전체 파일시스템
  접근을 피합니다.
