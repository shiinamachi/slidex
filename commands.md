# 명령어 가이드

이 문서는 PPTX 프롬프트 시스템을 설정하고, 이후 여러 덱을 같은 저장소에서
만드는 데 사용할 Codex CLI 명령을 정리합니다.

## 부트스트랩 실행

이 저장소를 처음 구성할 때는 다음 명령을 사용할 수 있습니다.

```bash
codex exec --sandbox workspace-write - < pptx-system-bootstrap-prompt.md
```

장시간 자율 작업으로 운영한다면 인터랙티브 세션에서 `/goal`을 사용할 수
있습니다.

```text
/goal Implement the complete PPTX production prompt-system workspace described in pptx-system-bootstrap-prompt.md without creating an actual presentation deck.
```

## 덱 작업공간 만들기

새 덱은 `decks/<deck_id>/` 아래에 만듭니다.

```bash
cp -R decks/_template decks/customer-retention
```

그 뒤 `decks/customer-retention/brief.md`를 작성하고 필요한 `assets/`,
`brand/`, `data/`, `source/` 자료를 추가합니다.

## 단계별 덱 제작 명령

여러 덱이 있으면 `DECK_ID`를 지정합니다. 각 단계는 이전 단계의 산출물을
`${OUT_DIR}`에서 읽습니다.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_intake_and_strategy.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/01_create_deck_spec.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/02_build_pptx.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/03_visual_qa.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/04_revise_deck.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/05_finalize_delivery.md
```

권장 순서는 전략, 덱 스펙, 편집 가능한 PPTX 제작, 슬라이드 렌더링, QA
몽타주, 시각 검사, 수정, 최종 납품입니다.

## 원샷 덱 제작 명령

빠르게 전체 흐름을 맡길 때는 원샷 프롬프트를 사용합니다.

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/one_shot_create_deck.md
```

원샷 실행에서도 `${OUT_DIR}/deck_spec.json`, `${OUT_DIR}/final_deck.pptx`,
렌더링 이미지, `${OUT_DIR}/qa_montage.png`, `${OUT_DIR}/qa_report.md`,
`${OUT_DIR}/notes.md`가 만들어져야 합니다.

## 인터랙티브 Codex CLI 흐름

1. `decks/_template/`를 복사해 `decks/<deck_id>/`를 만듭니다.
2. `decks/<deck_id>/brief.md`를 작성합니다.
3. 필요한 브랜드, 데이터, 참고 덱, 템플릿 파일을 덱 폴더에 추가합니다.
4. Codex CLI를 열고 현재 저장소를 작업 디렉터리로 둡니다.
5. 대상 덱이 여러 개라면 “`decks/<deck_id>`를 활성 덱으로 사용”이라고 먼저 지시합니다.
6. 먼저 전략 프롬프트를 실행하거나 붙여넣습니다.
7. 단계별 결과를 확인한 뒤 다음 프롬프트를 실행합니다.
8. QA 리포트에서 의미 있는 문제가 남아 있으면 수정 프롬프트를 반복합니다.
9. 최종 프롬프트로 납품 파일과 리스크를 확인합니다.

## 샌드박스 선택

- 일반적으로 `--sandbox workspace-write`를 사용합니다.
- 이 설정은 현재 작업공간 안에서 파일을 만들고 수정하는 데 충분합니다.
- `danger-full-access`나 비슷한 전체 접근 모드는 환경이 격리되고 신뢰할 수
  있을 때만 사용합니다.
- 알 수 없는 자료나 외부 파일을 다룰 때는 불필요한 네트워크 접근과 전체
  파일시스템 접근을 피합니다.
