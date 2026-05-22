# slidex Desktop

Electron 기반 GUI를 구현하기 위한 보일러플레이트입니다. 현재 단계에서는 실제
`slidex` CLI 실행, deck 탐색, 렌더링 제어는 연결하지 않고 앱 셸, IPC 경계,
빌드/패키징 설정만 준비합니다.

## Runtime

루트 `.mise.toml`에서 Node `24.16.0`을 exact pin합니다. 이 앱의 package manager
pin은 `package.json`의 `packageManager` 필드에 기록된 `npm@11.13.0`입니다.

```bash
mise install
cd apps/desktop
npm install
```

## Commands

```bash
npm run dev
npm run typecheck
npm run build
npm run pack
```

## Structure

```text
src/main/      Electron main process bootstrap and IPC registration
src/preload/   contextBridge API exposed to the renderer
src/renderer/  Vite + React desktop shell
src/shared/    IPC channel names and shared TypeScript contracts
resources/     icons and installer assets for future packaging work
```

CLI 연결을 구현할 때는 `src/main/` 아래에서 `slidex` 바이너리를 child process로
실행하고, `src/shared/api.ts`의 contract를 확장하는 방식으로 진행합니다.
