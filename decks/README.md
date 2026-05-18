# 여러 덱 작업공간

이 폴더는 같은 프롬프트 시스템 안에서 여러 PPTX 덱을 서로 섞지 않고
관리하기 위한 영역입니다. 각 덱은 `decks/<deck_id>/` 아래에 독립된 입력,
자료, 산출물을 둡니다.

## 권장 구조

```text
decks/
  customer-retention/
    brief.md
    assets/
      template.pptx
      reference_deck.pptx
      logo.png
    brand/
      guidelines.md
      colors.json
    data/
      retention.csv
    source/
      notes.pdf
    out/
      strategy.md
      deck_spec.json
      final_deck.pptx
      notes.md
      rendered_slides/
      qa_montage.png
      qa_report.md
```

`prompts/`, `schemas/`, `checklists/`는 저장소 공통 리소스입니다. 덱별
자료와 결과물은 항상 해당 덱 폴더 안에 둡니다.

## 새 덱 만들기

1. `decks/_template/`를 `decks/<deck_id>/`로 복사합니다.
2. `decks/<deck_id>/brief.md`를 작성합니다.
3. 필요한 템플릿, 참고 덱, 로고, 데이터, 원문 자료를 덱 폴더 안에 넣습니다.
4. 여러 덱이 있으면 실행 전에 대상 덱을 명시합니다.

예:

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_intake_and_strategy.md
```

인터랙티브 세션에서는 “`decks/customer-retention`을 활성 덱으로 사용”처럼
먼저 지시해도 됩니다.

## 공유 자료

여러 덱이 같은 브랜드나 공통 이미지를 써야 하면 `shared/brand/`,
`shared/assets/`, `shared/data/`를 둘 수 있습니다. 다만 덱별 파일이 있으면
덱별 파일을 우선하고, 공유 자료를 사용할 때는 `${OUT_DIR}/notes.md`에 사용
이유와 출처를 기록합니다.

## 기존 단일 덱 구조

루트의 `brief.md`, `assets/`, `brand/`, `data/`, `out/` 구조도 하위 호환을
위해 계속 사용할 수 있습니다. 여러 덱을 동시에 관리하려면 기존 파일을
`decks/<deck_id>/` 아래로 옮기는 것을 권장합니다.
