# PPTX 제작 프롬프트 시스템

이 저장소는 Codex CLI가 고품질 PowerPoint 덱을 만들 때 사용할 파일 기반 프롬프트 시스템입니다. 앱이나 자동 생성 프로그램이 아니라, 전략 수립부터 PPTX 제작, 렌더링, 시각 QA, 수정, 최종 납품까지 이어지는 작업 절차와 운영 키트입니다.

## 사용자가 준비할 파일

가능한 파일만 넣으면 됩니다. 일부가 없어도 프롬프트는 누락 정보를 정리하고 합리적인 가정으로 진행하도록 설계되어 있습니다.

- `brief.md`: 덱 목적, 청중, 메시지, 제약
- `assets/template.pptx`: 반드시 따라야 할 템플릿
- `assets/reference_deck.pptx`: 참고할 기존 덱
- `assets/logo.png`: 로고나 브랜드 이미지
- `brand/guidelines.md`: 브랜드 가이드
- `brand/colors.json`: 브랜드 컬러
- `data/*.csv`, `data/*.xlsx`: 차트와 표에 사용할 데이터
- PDF, 스크린샷, 원문 문서 등 기타 참고 자료

템플릿이나 참고 덱이 있으면 future 작업은 먼저 그것을 검사하고 화면비, 글꼴, 컬러, 레이아웃 패턴을 맞춰야 합니다. 없을 때만 기본 16:9를 사용합니다.

## 새 덱을 만드는 흐름

1. `brief.template.md`를 복사해 `brief.md`를 작성합니다.
2. 필요하면 `brand/guidelines.template.md`와 `brand/colors.template.json`을 바탕으로 브랜드 파일을 만듭니다.
3. `prompts/00_intake_and_strategy.md`로 전략을 만들고 `out/strategy.md`를 생성합니다.
4. `prompts/01_create_deck_spec.md`로 `out/deck_spec.json`을 만듭니다.
5. `prompts/02_build_pptx.md`로 편집 가능한 `out/final_deck.pptx`를 만듭니다.
6. `prompts/03_visual_qa.md`로 슬라이드를 이미지로 렌더링하고 QA 몽타주와 리포트를 만듭니다.
7. `prompts/04_revise_deck.md`로 문제를 수정하고 다시 렌더링합니다.
8. `prompts/05_finalize_delivery.md`로 최종 산출물을 확인하고 납품 요약을 작성합니다.

최종 전달 전에는 렌더링, QA 몽타주, 시각 검사, 수정 단계를 반드시 거쳐야 합니다.

## 단계별 프롬프트 실행

각 단계는 다음처럼 실행합니다.

```bash
codex exec --sandbox workspace-write - < prompts/00_intake_and_strategy.md
codex exec --sandbox workspace-write - < prompts/01_create_deck_spec.md
codex exec --sandbox workspace-write - < prompts/02_build_pptx.md
codex exec --sandbox workspace-write - < prompts/03_visual_qa.md
codex exec --sandbox workspace-write - < prompts/04_revise_deck.md
codex exec --sandbox workspace-write - < prompts/05_finalize_delivery.md
```

## 원샷 프롬프트

한 번에 전체 흐름을 실행하려면 다음 명령을 사용합니다.

```bash
codex exec --sandbox workspace-write - < prompts/one_shot_create_deck.md
```

원샷 프롬프트도 전략, 덱 스펙, PPTX 제작, 렌더링, QA, 수정, 최종 확인을 모두 요구합니다.

## 브랜드 템플릿 사용

- `brand/guidelines.template.md`를 참고해 `brand/guidelines.md`를 만듭니다.
- `brand/colors.template.json`을 참고해 `brand/colors.json`을 만듭니다.
- 실제 브랜드가 없다면 가짜 브랜드를 만들지 말고, 사용자가 승인한 일반적인 시각 방향만 적습니다.
- 브랜드 컬러는 강조용으로 제한해 사용하고, 본문 가독성과 대비를 우선합니다.

## QA 기준

`checklists/`의 체크리스트를 함께 사용합니다.

- `checklists/design_qa.md`: 스토리, 레이아웃, 타이포그래피, 차트, 완성도
- `checklists/accessibility_qa.md`: 텍스트 크기, 대비, 대체 텍스트, 읽기 순서
- `checklists/delivery_qa.md`: 최종 파일, 깨진 이미지, 오버플로, 폰트, 문서화

QA는 실제 슬라이드를 렌더링한 이미지와 `out/qa_montage.png`를 눈으로 확인한 뒤에만 통과로 판단합니다.

## 최종 산출물

완료된 덱 작업에서는 `out/`에 다음 파일이 있어야 합니다.

- `out/strategy.md`
- `out/deck_spec.json`
- `out/final_deck.pptx`
- `out/notes.md`
- 렌더링된 슬라이드 이미지
- `out/qa_montage.png`
- `out/qa_report.md`
- 최종 요약과 미해결 리스크

이 초기 설정 작업은 실제 덱을 만들지 않으므로 `out/final_deck.pptx`를 생성하지 않습니다.
