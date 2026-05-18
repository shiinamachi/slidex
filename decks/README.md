# 여러 비즈니스 문서 작업공간

이 폴더는 같은 `slidex` 시스템 안에서 여러 문서 작업을 서로
섞지 않고 관리하기 위한 영역입니다. 각 작업은 `decks/<deck_id>/` 아래에
독립된 입력, 자료, 산출물을 둡니다.

## 권장 구조

```text
decks/
  customer-retention/
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
      retention.csv
    source/
      notes.pdf
      screenshots/
    out/
      intake_questions.md
      source_inventory.md
      strategy.md
      deck_spec.json
      final_deck.html
      final_deck.generated_baseline.html
      rendered_slides/
      final_deck.pdf
      render_manifest.json
      qa_montage.png
      qa_report.md
      notes.md
      delivery_summary.md
```

`prompts/`, `schemas/`, `checklists/`, `cmd/`, `internal/`은 저장소 공통
리소스입니다. 작업별 자료와 결과물은 항상 해당 작업 폴더 안에 둡니다.

## 새 작업 만들기

```bash
cp -R decks/_template decks/<deck_id>
```

`brief.md`를 작성하고 필요한 reference docs, 로고, 이미지, 데이터, 원문 자료를
작업 폴더 안에 넣습니다. 사용자가 제공한 PPTX는 `source/` 아래의 참고 문서로만
취급합니다.

예:

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_start_business_doc.md
```

## 공유 자료

여러 작업이 같은 브랜드나 공통 이미지를 써야 하면 `shared/brand/`,
`shared/assets/`, `shared/data/`를 둘 수 있습니다. 덱별 파일이 있으면 덱별
파일을 우선하고, 공유 자료를 사용할 때는 `${OUT_DIR}/notes.md`에 사용 이유와
출처를 기록합니다.
