# 비즈니스 문서 작업공간 템플릿

이 폴더는 `slidex init <deck_id>`가 새 deck 작업공간을 만들 때 사용하는
기본 템플릿입니다. 새 작업공간은 템플릿을 직접 복사하지 말고 CLI로 만듭니다.

```bash
slidex init <deck_id>
```

생성한 작업공간에서는 `brief.md`를 먼저 채우고, 필요한 경우 다음 하위 폴더를
사용합니다.

- `DESIGN.md`: 이 문서에만 적용할 선택적 디자인 가이드
- `assets/`: reference docs, 로고, 이미지
- `brand/`: 승인된 브랜드 가이드와 컬러 파일
- `data/`: 차트와 표에 사용할 CSV/XLSX
- `source/`: PDF, DOCX, 스크린샷, 회의 노트, 원문 자료
- `out/`: `slidex` CLI가 생성하는 전략, spec, HTML, PDF, QA 결과

템플릿 폴더에는 실제 산출물을 만들지 않습니다.
