# slidex

`slidex`은 Codex CLI가 비즈니스급 문서를 만들 때 사용하는
파일 기반 프롬프트 시스템과 로컬 자동화 키트입니다. 대상 산출물은
PowerPoint가 아니라 HTML을 시각 원본으로 삼아 렌더링한 페이지형 PDF입니다.

이 저장소는 앱이나 SaaS가 아닙니다. 비즈니스 계획서, IR/회사소개서, 정부지원
사업계획서, 제안서, 임원 리뷰 문서를 만들기 위한 durable prompt, JSON
schema, QA checklist, Go CLI, 작업공간 템플릿을 제공합니다.

## 작업공간 모델

각 문서 작업은 `decks/<deck_id>/` 아래에 격리합니다.

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
    *.pptx
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

`final_deck.html`이 최종 시각 원본이며, `final_deck.pdf`가 기본 납품 파일입니다.
PDF는 일반 프레젠테이션 내보내기처럼 한 슬라이드가 한 PDF 페이지가 되도록
만듭니다. 긴 세로형 PDF는 사용자가 별도 실험 산출물로 요청한 경우에만
만듭니다.

## 입력 파일

가능한 자료만 넣으면 됩니다. 자료가 부족하면 Codex는 먼저 현재 자료를 검사한
뒤 한국어 Q&A를 반복해 책임 있게 제작 가능한 수준까지 brief를 보강해야 합니다.

- `${ACTIVE_DECK_DIR}/brief.md`: 문서 목적, 청중, 의사결정 맥락, 제약
- `${ACTIVE_DECK_DIR}/DESIGN.md`: 이 문서에만 적용할 스타일 프롬프트
- `${ACTIVE_DECK_DIR}/assets/reference_docs/`: 참고 문서, 이미지, 벤치마크 자료
- `${ACTIVE_DECK_DIR}/assets/logo.png`, `${ACTIVE_DECK_DIR}/assets/images/`
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`, `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/data/*.csv`, `${ACTIVE_DECK_DIR}/data/*.xlsx`
- `${ACTIVE_DECK_DIR}/source/`: PDF, DOCX, PPTX, 스크린샷, 회의 노트, 원문 자료

사용자가 제공한 PPTX는 생성 대상이 아니라 수동 참고/source 문서로만 취급합니다.
이 시스템은 PPTX를 생성하거나 선택 납품물로 제안하지 않습니다.

## 표준 워크플로

1. 활성 작업공간을 결정합니다.
2. brief, DESIGN.md, brand, assets, data, source, 기존 out 자료를 검사합니다.
3. 문서 유형, 청중, 목적, 핵심 주장, 제약이 부족하면 Q&A를 반복합니다.
4. `${OUT_DIR}/intake_questions.md`와 `${OUT_DIR}/source_inventory.md`를 작성하고
   `${ACTIVE_DECK_DIR}/brief.md`를 확인된 사실과 승인된 가정으로 갱신합니다.
5. `${OUT_DIR}/strategy.md`를 만듭니다.
6. `schemas/deck_spec.schema.json`에 맞는 `${OUT_DIR}/deck_spec.json`을 만듭니다.
7. 정적 HTML/CSS 슬라이드 원본 `${OUT_DIR}/final_deck.html`을 작성합니다.
8. 같은 내용을 `${OUT_DIR}/final_deck.generated_baseline.html`에 기준선으로
   저장합니다.
9. `slidex render`로 `.slide` 요소를 PNG로 렌더링하고,
   `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`를 생성합니다.
10. 비즈니스 논리, claim provenance, 시각 품질, HTML/PDF 무결성, 접근성을 QA합니다.
11. QA 이슈를 HTML/spec/notes에 반영하고 다시 렌더링합니다.
12. 최종 검증 후 `${OUT_DIR}/delivery_summary.md`를 작성합니다.

최종 완료를 주장하려면 현재 HTML에서 렌더링된 이미지와 PDF가 존재하고, montage와
렌더 이미지를 실제로 검사해야 합니다.

## 프롬프트 실행

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

원샷 흐름은 intake gate가 완료된 경우에만 자율 실행합니다.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/one_shot_create_business_doc.md
```

## 로컬 CLI

표준 렌더링과 검증에는 canonical CLI 이름 `slidex`을 사용합니다. 문서와
acceptance 기준의 기준 이름은 항상 `slidex`입니다.

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

제공 명령:

- `inspect`: 작업공간 입력과 산출물 inventory를 JSON으로 출력합니다.
- `validate-spec`: `deck_spec.json`을 schema 기반 계약에 맞춰 검증합니다.
- `render`: HTML `.slide`를 PNG로 렌더링하고 페이지형 PDF, manifest, montage를
  만듭니다.
- `qa`: HTML, 렌더 이미지, PDF, manifest, schema, claim provenance 중심의
  기계 판독 가능한 QA findings를 출력합니다.
- `sync-html-edits`: 사용자가 직접 수정한 HTML을 baseline과 비교하고 spec/notes/QA
  stale 상태를 동기화합니다.
- `package`: 최종 산출물 존재와 manifest freshness를 확인합니다.

## 런타임과 버전 고정

런타임 관리는 mise를 사용합니다. 이 저장소의 Go 버전은 `.mise.toml`과
`go.mod`의 `go` 지시문에서 모두 `1.26.3`으로 exact pin합니다.

```bash
mise install
mise exec -- go version
mise exec -- go install ./cmd/slidex
mise exec -- go test ./...
```

라이브러리와 런타임 버전은 항상 exact version으로 기록합니다. `latest`,
`main`, `master`, `HEAD`, `>=`, `<=`, `>`, `<`, `^`, `~`, `x`, `*` 같은 range 또는
floating 표기는 사용하지 않습니다. 외부 CDN, 원격 CSS, 폰트, 이미지 처리 도구,
브라우저/렌더러 버전도 exact version을 기록하거나 로컬에 vendoring하고
`render_manifest.json`에 버전 또는 SHA-256을 남겨야 합니다.

## HTML 기준

- `<!doctype html>`과 적절한 `lang`을 사용합니다.
- `.deck` 루트 안에 `<section class="slide" data-slide-id="slide_01">` 형식의
  안정적인 slide 요소를 둡니다.
- 기본 화면비는 16:9 Widescreen, 기본 렌더 크기는 `1920x1080`입니다.
- 지원 preset: `wide-1080p`, `wide-720p`, `wide-900p`, `wide-1440p`, `custom`.
- 전체 슬라이드를 PNG로 묻어 HTML 본문으로 쓰지 않습니다.
- CSS 변수로 design token을 관리합니다.
- 한국어 비즈니스 문서는 기본적으로 Pretendard font preset을 사용하며,
  `deck_spec.json`과 CSS 변수에 font choice를 기록합니다.
- 한국어는 `word-break: keep-all`, `overflow-wrap: normal`, `hyphens: none`,
  `line-break: strict`를 기본으로 합니다.
- 가짜 지표, 고객명, 로고, 스크린샷, 제품명, 기술 범위를 만들지 않습니다.

## 최종 산출물

완료된 작업의 `${OUT_DIR}/`에는 다음 파일이 필요합니다.

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

직접 HTML을 수정한 경우 `html_edit_sync.md`도 필요합니다.
