# slidex Desktop Design System

이 문서는 `apps/desktop` 전용 디자인 기준이다. 덱 작업공간의 `DESIGN.md`와
분리하며, Electron desktop 앱 UI를 수정하거나 새 화면을 만들 때 이 문서를 우선한다.

## Reference Basis

2026-06-09 기준으로 다음 공개 문서를 확인해 구조를 잡았다.

- Tailwind CSS v4는 CSS `@theme` 변수로 색상, 폰트, 반경, 그림자 토큰을 정의하고
  유틸리티로 노출한다: <https://tailwindcss.com/docs/theme>,
  <https://tailwindcss.com/docs/colors>
- Fluent 2는 raw value에 가까운 global token과 의미를 담은 alias token을 분리한다:
  <https://fluent2.microsoft.design/design-tokens>
- Atlassian Design System은 foundations를 tokens, accessibility, content, spacing,
  grid, color, typography, elevation, border, radius 등으로 구성한다:
  <https://atlassian.design/foundations>
- 접근성 기준은 색상만으로 의미를 전달하지 않고, 일반 텍스트 4.5:1 및 큰 텍스트와
  그래픽 3:1 이상의 대비를 목표로 한다:
  <https://atlassian.design/foundations/accessibility>
- Design Tokens Community Group은 도구와 플랫폼 간 시각 언어 공유를 위한 토큰
  표준화를 다룬다: <https://www.designtokens.org/>

## Product Tone

slidex Desktop은 CLI 워크플로를 조작하는 운영형 생산성 앱이다. 첫 화면은 마케팅
랜딩이 아니라 작업공간, 실행 상태, 검수 상태, 패키징 상태가 바로 보이는 조용한
데스크톱 셸이어야 한다.

화이트 모드를 기본으로 한다. 배경은 차가운 off-white, 주요 표면은 흰색, 경계선은
밝은 중립 회색, 텍스트는 거의 검정에 가까운 중립색을 쓴다. Primary accent는
`#fd79a8`이며, 브랜드 신호, 주요 액션, 선택 상태, 포커스 링, 하이라이트에 사용한다.

## Token Architecture

토큰의 코드 소스는 `src/renderer/styles.css`의 Tailwind `@theme static` 블록이다.
미사용 토큰도 디자인 시스템 계약으로 남아야 하므로 static option을 사용한다. React
컴포넌트에서는 raw hex 대신 `bg-surface`, `text-ink`, `border-border`, `bg-accent`
같은 의미 기반 Tailwind 클래스를 사용한다.

토큰은 세 계층으로 생각한다.

- Primitive: 실제 색상값, 반경, 그림자, 폰트 스택.
- Semantic: `canvas`, `surface`, `border`, `ink`, `accent`, `state-*`처럼 용도를
  드러내는 이름.
- Component: 버튼, 사이드바, 패널, 상태 배지 같은 UI 단위의 조합. 필요할 때만
  컴포넌트 클래스로 승격한다.

## Color

주요 토큰:

- `canvas`: 앱 전체 배경. 큰 면적에 사용한다.
- `surface`: 패널, 헤더, 사이드바의 기본 표면.
- `surface-muted`: 리스트 행, 비활성 세그먼트, 보조 영역.
- `border`, `border-strong`: 구획선과 hover/active 경계.
- `ink`, `ink-muted`, `ink-subtle`: 본문, 보조 텍스트, 메타 텍스트.
- `accent`: `#fd79a8`. primary CTA, 선택 상태의 핵심 색.
- `accent-soft`, `accent-muted`: 선택 배경, hover, ring.
- `accent-strong`: 작은 텍스트나 배지에 쓰는 고대비 accent.
- `accent-ink`: `accent` 배경 위의 텍스트.
- `state-info`, `state-success`, `state-warning`, `state-danger`: 상태 의미 색.

`#fd79a8` 위에 흰색 작은 텍스트를 올리지 않는다. 대비가 필요한 텍스트는
`accent-ink` 또는 `accent-strong`을 사용한다.

## Typography

기본 폰트는 `Inter, Pretendard, Noto Sans KR, system-ui` 순서의 sans stack이다.
한국어 UI가 깨지지 않아야 하며, `word-break: keep-all`, `line-break: strict` 기본값을
유지한다.

폰트 크기는 Tailwind 기본 스케일을 사용한다. viewport width로 폰트 크기를 스케일하지
않는다. letter spacing은 0을 기본으로 하며, 장식적 자간 조절을 추가하지 않는다.

## Spacing, Radius, Elevation

간격은 Tailwind 기본 spacing scale을 사용한다. 운영형 UI이므로 과도한 여백보다
스캔 가능한 밀도를 우선한다.

패널과 카드의 radius는 최대 8px이다. 컨트롤은 6px를 기본으로 한다. 상태 점이나
작은 배지는 pill 형태를 허용하지만, 큰 카드나 섹션을 과도하게 둥글게 만들지 않는다.

그림자는 `shadow-panel`, `shadow-control`만 기본으로 사용한다. 표면 구분은 먼저
border와 배경으로 해결하고, 그림자는 떠 있는 패널이나 클릭 가능한 컨트롤에만 제한한다.

## Layout

기본 구조는 좌측 navigation rail, 상단 command bar, 중앙 workspace, 우측 inspector다.
Electron `BrowserWindow`의 최소 폭은 1024px이며, CSS에서 별도의 body 최소 폭을
강제하지 않는다. 좁은 desktop 폭에서는 inspector를 아래로 내려 가로 스크롤을 피한다.
모바일 우선 반응형이 아니라 Electron desktop shell을 우선하되, 텍스트가 컨테이너를
넘치지 않도록 grid/flex의 `minmax(0, 1fr)`, `truncate`, 고정 컨트롤 높이를 사용한다.

hero, full-bleed marketing section, 장식용 gradient/orb 배경은 사용하지 않는다. 앱은
반복 작업을 위한 도구처럼 보여야 한다.

## Components

Primary button은 `bg-accent text-accent-ink` 조합을 사용한다. Secondary button은
`bg-surface border-border text-ink-muted`에서 시작하고 hover 시 `text-ink`,
`border-border-strong`으로 올린다.

Navigation item의 active 상태는 `bg-accent-soft text-accent-strong ring-accent-muted`
조합을 사용한다. 상태는 색만으로 전달하지 말고 텍스트 라벨도 함께 둔다.

Panel은 `bg-surface border-border rounded-panel shadow-panel` 조합을 기본으로 한다.
Panel 안에 또 다른 큰 card를 중첩하지 않는다. 반복 아이템은 `surface-muted` 배경과
얇은 border로 구분한다.

Segmented control, toggle, checkbox, select, numeric input이 필요한 경우 native control
또는 명확한 컴포넌트로 만든다. 텍스트만 담긴 둥근 사각형을 범용 아이콘 버튼처럼
사용하지 않는다.

## Accessibility

모든 interactive element는 keyboard focus가 보여야 하며, focus ring은 `accent`를
사용한다. 비활성 상태를 색상만으로 표현하지 않는다. 오류, 경고, 성공, 정보 상태는
텍스트와 상태 색을 함께 쓴다.

본문 텍스트 대비는 4.5:1 이상, 큰 텍스트와 아이콘성 그래픽은 3:1 이상을 목표로 한다.
contrast가 불확실하면 더 어두운 토큰인 `ink`, `accent-strong`, `state-*`를 선택한다.

## Implementation Rules

- Tailwind 토큰 변경은 `src/renderer/styles.css`의 `@theme static`에서 시작한다.
- React 컴포넌트에서 raw hex, ad hoc shadow, arbitrary radius를 반복하지 않는다.
- 새 공통 UI가 두 번 이상 반복되면 `src/renderer` 아래에 작고 명확한 컴포넌트로
  추출한다.
- 새 dependency가 필요하면 exact version으로 추가하고 lockfile, 문서, 검증을 같은
  변경에 포함한다.
- 디자인 토큰, 컴포넌트 규칙, 화면 밀도가 바뀌면 이 문서를 함께 갱신한다.

## Verification

Desktop UI 변경 시 최소 검증:

```bash
cd apps/desktop
pnpm run typecheck
pnpm run build
```

시각 변경이 있으면 Vite renderer 또는 Electron preview를 띄워 화면을 직접 확인한다.
렌더링된 화면에서 텍스트 겹침, 과도한 색상 집중, focus 상태, 컨트롤 높이 변화, 상태
라벨 누락을 확인한다.
