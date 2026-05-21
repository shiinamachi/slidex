# 비즈니스 문서 작업공간 템플릿

이 폴더를 복사해 새 `slidex` 작업공간을 만듭니다.

```bash
cp -R decks/_template decks/<deck_id>
```

복사한 뒤에는 `brief.md`를 먼저 채우고, 필요한 경우 다음 하위 폴더를
사용합니다.

- `DESIGN.md`: 이 문서에만 적용할 선택적 스타일 프롬프트
- `assets/`: reference docs, 로고, 이미지
- `brand/`: 승인된 브랜드 가이드와 컬러 파일
- `data/`: 차트와 표에 사용할 CSV/XLSX
- `source/`: PDF, DOCX, 스크린샷, 회의 노트, 원문 자료
- `out/`: 프롬프트와 CLI가 생성하는 전략, spec, HTML, PDF, QA 결과

템플릿 폴더에는 실제 산출물을 만들지 않습니다.
