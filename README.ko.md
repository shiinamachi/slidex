# slidex

[English documentation →](README.md)

**HTML-first 비즈니스 문서와 페이지형 PDF를 만드는 로컬 CLI 및 Codex Plugin**

`slidex`는 각 문서 프로젝트를 독립된 deck workspace로 관리하고, 디자인 토큰과
한국어 최적화 타이포그래피를 적용한 정적 HTML 슬라이드를 만든 뒤 슬라이드 PNG와
최종 PDF로 렌더링하며, 모든 산출물이 최신 상태인지 검증합니다.

---

## ⚡ Codex App으로 설치

아래 프롬프트를 **Codex App** 채팅창에 붙여넣으세요. Codex가 자동으로 CLI 설치,
플러그인 등록, 검증까지 수행합니다:

```text
Install slidex from https://github.com/shiinamachi/slidex; read INSTALL.md in that repository and complete every step: detect the local OS and architecture, download the matching release package from the latest GitHub Release tag, verify the SHA-256 checksum, extract and install the binary to a stable directory, add it to PATH, register the Codex plugin from the bundled marketplace, and run "slidex --help", "slidex update status --json", and "slidex doctor --render" to confirm success. If update status reports restartRequired, restart Codex, start a new thread, run "slidex codex app-server plugin-smoke --json", and then run "slidex update verify --json" before treating bundled skills as active. Report each step's result.
```

> 이 프롬프트의 상세 동작은
> [CODEX_INSTALL_PROMPT.md](CODEX_INSTALL_PROMPT.md)를 참고하세요.
> 전체 내부 설치 절차는 [INSTALL.md](INSTALL.md)에 있습니다.

---

## slidex란?

**로컬에서 재현 가능한 문서 제작 워크플로**가 필요할 때 사용합니다:

- 📊 비즈니스 프레젠테이션, 투자자 deck
- 📝 임원 보고서, 제안서
- 🏛️ 정부 심의 문서
- 🤝 고객 의사결정 자료

`slidex`는 호스팅 웹 앱이 **아닙니다**. 모든 작업은 로컬 머신에서 실행되며,
편집 가능한 원본은 HTML 파일이고 납품 산출물은 PDF입니다.

### 핵심 파일

| 파일 | 역할 |
|------|------|
| `decks/<deck_id>/out/final_deck.html` | 편집 가능한 시각 원본 |
| `decks/<deck_id>/out/final_deck.pdf` | 기본 납품 파일 |

---

## 주요 기능

| | 기능 | 설명 |
|---|------|------|
| 🗂️ | **Deck Workspace** | 각 프로젝트를 `decks/` 아래 독립 디렉터리로 관리 |
| 🏗️ | **HTML-first 빌드** | CSS 디자인 토큰, 16:9 와이드스크린, 한국어 폰트 프리셋 |
| 🖼️ | **자동 렌더링** | Headless Chrome/Chromium/Edge로 슬라이드 PNG + PDF 생성 |
| ✅ | **품질 검증** | QA 몽타주, QA 리포트, 최신성 검사 |
| 📦 | **패키지 검증** | 필수 산출물 존재 여부 및 HTML 일치 확인 |
| 🔌 | **Codex Plugin** | Codex App 브라우저에서 대화형 워크벤치로 deck 생성 |
| 📋 | **근거 기반** | 모든 주장은 출처가 있거나, 확인되었거나, 가정으로 표시 |

---

## 빠른 시작

### 1. Deck 생성

```bash
slidex init customer-retention
```

### 2. Brief 작성

생성된 brief에 문서 목표, 청중, 자료를 작성합니다:

```text
decks/customer-retention/brief.md
```

### 3. 지원 자료 추가

로고, 데이터 파일, 참조 문서, 브랜드 가이드라인을 deck workspace 디렉터리
(`assets/`, `brand/`, `data/`, `source/`)에 넣습니다.

### 4. 워크플로 실행

```bash
slidex run --deck decks/customer-retention
```

전체 파이프라인을 실행합니다: intake → strategy → spec → HTML 빌드 → 렌더 →
QA → 납품 요약 → 패키지 검사.

### 5. 결과물 확인

```text
decks/customer-retention/out/final_deck.html    ← 편집 가능한 HTML
decks/customer-retention/out/final_deck.pdf     ← 납품 PDF
decks/customer-retention/out/qa_montage.png     ← 시각 QA 개요
decks/customer-retention/out/qa_report.md       ← QA 상세 내역
decks/customer-retention/out/delivery_summary.md ← 납품 체크리스트
```

---

## Deck Workspace 구조

모든 문서 프로젝트는 `decks/<deck_id>/` 아래에 있습니다:

```text
decks/<deck_id>/
  brief.md              ← 문서 목표, 청중, 제약
  DESIGN.md             ← deck별 시각 방향
  assets/               ← 로고, 제품 이미지, 참조 파일
    reference_docs/
    images/
  brand/                ← 브랜드 가이드라인, 색상, 폰트
  data/                 ← CSV, XLSX, 차트/테이블 원천 데이터
  source/               ← PDF, DOCX, 스크린샷, 회의 노트
  out/                  ← 생성된 결과물 (아래 참조)
```

### 생성 결과물

| 파일 | 설명 |
|------|------|
| `out/strategy.md` | 콘텐츠 전략 |
| `out/deck_spec.json` | 구조화된 슬라이드 스펙 |
| `out/final_deck.html` | 편집 가능한 HTML 슬라이드 |
| `out/final_deck.generated_baseline.html` | diff용 베이스라인 |
| `out/rendered_slides/*.png` | 개별 슬라이드 PNG |
| `out/final_deck.pdf` | 페이지형 PDF |
| `out/render_manifest.json` | 렌더 메타데이터 및 해시 |
| `out/qa_montage.png` | 시각 QA 몽타주 |
| `out/qa_report.md` | QA 결과 |
| `out/notes.md` | 발표자 노트 |
| `out/delivery_summary.md` | 최종 납품 체크리스트 |

---

## 워크플로 파이프라인

```text
  init ──→ intake ──→ strategy ──→ spec ──→ build ──→ render ──→ qa ──→ finalize ──→ package
  │                                                                                    │
  └──── slidex run --deck decks/<deck_id> ─────────────────────────────────────────────┘
```

표준 엔드투엔드 워크플로는 `slidex run`을 사용합니다. 개별 stage 명령은 검사나
수리가 필요할 때만 사용합니다:

```bash
slidex inspect --deck decks/<deck_id> --write
slidex intake --deck decks/<deck_id>
slidex strategy --deck decks/<deck_id>
slidex spec --deck decks/<deck_id>
slidex build --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex finalize --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
```

자료가 충분하지 않으면 `slidex`가 exit code `3`으로 멈추고
`out/intake_questions.md`에 질문을 작성합니다. `brief.md`에 답을 추가한 뒤 다시
실행합니다.

---

## HTML 직접 편집

`out/final_deck.html`을 직접 수정했다면, PDF가 최신이라고 보기 전에 sync와
재렌더를 수행합니다:

```bash
slidex sync-html-edits --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
```

---

## Codex Plugin Workbench

저장소에 포함된 Codex Plugin(`plugins/slidex/`)을 통해 Codex App 인앱 브라우저에서
대화형 워크벤치로 deck brief를 만들 수 있습니다.

```bash
slidex workbench start --deck-id customer-retention
```

반환된 `http://127.0.0.1:<port>/workbench/<session>` URL을 Codex App 브라우저에서
엽니다. 워크벤치를 저장하면 `brief.md`와 워크벤치 산출물이 deck의 `out/`
디렉터리에 작성됩니다. 이후 일반 워크플로를 실행합니다:

```bash
slidex run --deck decks/customer-retention
```

---

## 문제 해결

CLI와 렌더 환경을 점검합니다:

```bash
slidex doctor --render
```

Chrome이 감지되지 않으면 아래 환경 변수 중 하나를 설정합니다:

```text
CHROME_BIN · GOOGLE_CHROME_BIN · CHROMIUM_BIN · MSEDGE_BIN
CHROME_FOR_TESTING_BIN · PLAYWRIGHT_CHROMIUM_BIN
PLAYWRIGHT_CHROME_BIN · PUPPETEER_EXECUTABLE_PATH
```

Deck별 진단:

```bash
slidex doctor --deck decks/<deck_id> --render
slidex inspect --deck decks/<deck_id>
```

스키마 검증:

```bash
slidex validate-spec --spec decks/<deck_id>/out/deck_spec.json
```

---

## 명령어 참조

| 명령어 | 설명 |
|--------|------|
| `slidex init <deck_id>` | 새 deck workspace 생성 |
| `slidex run --deck decks/<deck_id>` | 전체 워크플로 실행 |
| `slidex render --deck decks/<deck_id>` | PNG 및 PDF 렌더링 |
| `slidex qa --deck decks/<deck_id>` | 품질 검증 |
| `slidex package --deck decks/<deck_id>` | 납품 산출물 검증 |
| `slidex sync-html-edits --deck decks/<deck_id>` | 수동 HTML 편집 동기화 |
| `slidex doctor --render` | CLI 및 렌더 환경 점검 |
| `slidex workbench start --deck-id <deck_id>` | Codex 워크벤치 시작 |
| `slidex validate-spec --spec <path>` | Deck spec JSON 검증 |

전체 명령어 목록은 `slidex --help`에서 확인합니다.
Advanced render override와 Codex 전용 명령 예시는
[commands.md](commands.md)를 참고합니다.

---

## 라이선스

MIT License입니다. Copyright (c) 2026 shiinamachi. 자세한 내용은
[LICENSE](LICENSE)를 참고합니다.
