# PPTX 제작 프롬프트 시스템

이 저장소는 Codex CLI가 고품질 PowerPoint 덱을 만들 때 사용할 파일 기반
프롬프트 시스템입니다. 앱이나 자동 생성 프로그램이 아니라, 전략 수립부터
PPTX 제작, 렌더링, 시각 QA, 수정, 최종 납품까지 이어지는 작업 절차와 운영
키트입니다.

여러 덱을 같은 저장소에서 관리할 수 있도록 각 덱은
`decks/<deck_id>/` 아래에 독립된 작업공간을 가집니다. 기존 루트 기반
`brief.md`, `assets/`, `brand/`, `data/`, `out/` 구조도 하위 호환을 위해
지원하지만, 새 작업은 덱별 작업공간을 권장합니다.

## 덱별 작업공간

권장 구조는 다음과 같습니다.

```text
decks/<deck_id>/
  brief.md
  DESIGN.md
  assets/
    template.pptx
    reference_deck.pptx
    reference_decks/
      benchmark-board-deck.pptx
      product-launch-style.pptx
    logo.png
  brand/
    guidelines.md
    colors.json
  data/
    *.csv
    *.xlsx
  source/
    notes.pdf
    screenshots/
  out/
    strategy.md
    deck_spec.json
    final_deck.pptx
    notes.md
    rendered_slides/
    qa_montage.png
    qa_report.md
```

새 덱을 만들 때는 `decks/_template/`를 복사해 `decks/<deck_id>/`를 만들고
`brief.md`를 작성합니다. 덱마다 특정 스타일을 적용해야 하면 같은 폴더의
`DESIGN.md`에 스타일 프롬프트를 적습니다. 여러 덱이 있으면 프롬프트 실행
전에 대상 덱을 명시해야 합니다.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_intake_and_strategy.md
```

인터랙티브 세션에서는 “`decks/customer-retention`을 활성 덱으로 사용”처럼
먼저 지시해도 됩니다. 활성 덱 선택 규칙은
`prompts/_active_deck_context.md`에 정리되어 있습니다.

## 사용자가 준비할 파일

가능한 파일만 넣으면 됩니다. 일부가 없어도 프롬프트는 누락 정보를 정리하고
합리적인 가정으로 진행하도록 설계되어 있습니다.

- `${ACTIVE_DECK_DIR}/brief.md`: 덱 목적, 청중, 메시지, 제약
- `${ACTIVE_DECK_DIR}/DESIGN.md`: 이 덱에만 적용할 스타일 프롬프트
- `${ACTIVE_DECK_DIR}/assets/template.pptx`: 반드시 따라야 할 템플릿
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx`: 기존 단일 참고 덱 파일명
  (하위 호환)
- `${ACTIVE_DECK_DIR}/assets/reference_decks/`: 원하는 만큼 넣는 참고 덱 폴더
- `${ACTIVE_DECK_DIR}/assets/logo.png`: 로고나 브랜드 이미지
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`: 브랜드 가이드
- `${ACTIVE_DECK_DIR}/brand/colors.json`: 브랜드 컬러
- `${ACTIVE_DECK_DIR}/data/*.csv`, `${ACTIVE_DECK_DIR}/data/*.xlsx`: 차트와 표에 사용할 데이터
- `${ACTIVE_DECK_DIR}/source/`: PDF, 스크린샷, 원문 문서 등 기타 참고 자료

템플릿이나 참고 덱 세트가 있으면 향후 작업은 먼저 그것들을 검사하고 화면비,
글꼴, 컬러, 레이아웃 패턴을 맞춰야 합니다. 참고 덱은
`assets/reference_decks/`에 원하는 수만큼 둘 수 있으며, 기존
`assets/reference_deck.pptx`도 하위 호환으로 함께 검사합니다. 없을 때만 기본
16:9를 사용합니다. `DESIGN.md`가 있으면 덱별 스타일 방향으로 적용하되,
승인된 템플릿, 참고 덱 세트, 브랜드 가이드, 접근성, 편집 가능성 요구사항보다
우선하지 않습니다. 적용한
스타일 지시와 충돌 사항은 `strategy.md`, `deck_spec.json`, `notes.md`,
`qa_report.md`에 필요한 수준으로 기록합니다.

## 새 덱을 만드는 흐름

1. `decks/_template/`를 복사해 `decks/<deck_id>/`를 만듭니다.
2. `${ACTIVE_DECK_DIR}/brief.md`를 작성합니다.
3. 필요하면 `${ACTIVE_DECK_DIR}/DESIGN.md`에 덱별 스타일 프롬프트를 작성합니다.
4. 필요하면 덱별 `brand/`, `assets/`, `data/`, `source/` 자료를 추가합니다.
5. `prompts/00_intake_and_strategy.md`로 `${OUT_DIR}/strategy.md`를 생성합니다.
6. `prompts/01_create_deck_spec.md`로 `${OUT_DIR}/deck_spec.json`을 만듭니다.
7. `prompts/02_build_pptx.md`로 편집 가능한 `${OUT_DIR}/final_deck.pptx`를 만듭니다.
8. `prompts/03_visual_qa.md`로 슬라이드를 이미지로 렌더링하고 QA 몽타주와 리포트를 만듭니다.
9. `prompts/04_revise_deck.md`로 문제를 수정하고 다시 렌더링합니다.
10. `prompts/05_finalize_delivery.md`로 최종 산출물을 확인하고 납품 요약을 작성합니다.

최종 전달 전에는 렌더링, QA 몽타주, 시각 검사, 수정 단계를 반드시 거쳐야
합니다.

## 단계별 프롬프트 실행

각 단계는 다음처럼 실행합니다. 덱이 여러 개라면 `DECK_ID`를 지정하거나
인터랙티브 세션에서 활성 덱 폴더를 먼저 알려주세요.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_intake_and_strategy.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/01_create_deck_spec.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/02_build_pptx.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/03_visual_qa.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/04_revise_deck.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/05_finalize_delivery.md
```

## 원샷 프롬프트

한 번에 전체 흐름을 실행하려면 다음 명령을 사용합니다.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/one_shot_create_deck.md
```

원샷 프롬프트도 전략, 덱 스펙, PPTX 제작, 렌더링, QA, 수정, 최종 확인을 모두
요구합니다.

## 브랜드와 공유 자료

- 덱별 브랜드 파일은 `${ACTIVE_DECK_DIR}/brand/`에 둡니다.
- 브랜드 템플릿은 `brand/guidelines.template.md`와
  `brand/colors.template.json`을 참고합니다.
- 실제 브랜드가 없다면 가짜 브랜드를 만들지 말고, 사용자가 승인한 일반적인
  시각 방향만 적습니다.
- 여러 덱이 같은 자료를 써야 하면 `shared/brand/`, `shared/assets/`,
  `shared/data/`를 둘 수 있습니다.
- 공유 자료를 사용할 때도 덱별 파일이 우선이며, 사용 이유와 출처를
  `${OUT_DIR}/notes.md`에 기록합니다.

## QA 기준

`checklists/`의 체크리스트를 함께 사용합니다.

- `checklists/design_qa.md`: 스토리, 레이아웃, 타이포그래피, 차트, 완성도
- `checklists/accessibility_qa.md`: 텍스트 크기, 대비, 대체 텍스트, 읽기 순서
- `checklists/delivery_qa.md`: 최종 파일, 깨진 이미지, 오버플로, 폰트, 문서화

QA는 실제 슬라이드를 렌더링한 이미지와 `${OUT_DIR}/qa_montage.png`를 눈으로
확인한 뒤에만 통과로 판단합니다.

## 최종 산출물

완료된 덱 작업에서는 `${OUT_DIR}/`에 다음 파일이 있어야 합니다.

- `${OUT_DIR}/strategy.md`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/final_deck.pptx`
- `${OUT_DIR}/notes.md`
- `${OUT_DIR}/rendered_slides/`의 렌더링된 슬라이드 이미지
- `${OUT_DIR}/qa_montage.png`
- `${OUT_DIR}/qa_report.md`
- 최종 요약과 미해결 리스크

이 초기 설정 작업은 실제 덱을 만들지 않으므로 `final_deck.pptx`를 생성하지
않습니다.
