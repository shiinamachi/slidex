# slidex Desktop

Electron 기반 GUI를 구현하기 위한 보일러플레이트입니다. 현재 단계에서는 실제
`slidex` CLI 실행, deck 탐색, 렌더링 제어는 연결하지 않고 앱 셸, IPC 경계,
빌드/패키징 설정만 준비합니다.

## Runtime

루트 `.mise.toml`에서 Node `24.16.0`과 pnpm `11.5.2`를 exact pin합니다. 이 앱의
package manager pin은 `package.json`의 `packageManager` 필드에 기록된
`pnpm@11.5.2`입니다.

```bash
mise install
cd apps/desktop
pnpm install
```

## Commands

```bash
pnpm run dev
pnpm run dev:wsl
pnpm run typecheck
pnpm run build
pnpm run pack
```

`pnpm run dev:wsl`은 WSL에서 renderer/dev main watch를 실행하고 Windows host에서
pinned Electron dev 앱을 띄웁니다. Windows host에 `mise`가 있으면
`mise exec -- pnpm dlx`로 `.mise.toml`의 pinned runtime을 사용하고, 없으면
`pnpm.cmd`, 그 다음 Windows Node.js의 `npx.cmd`를 fallback으로 사용합니다.

## Structure

```text
src/main/      Electron main process bootstrap and IPC registration
src/preload/   contextBridge API exposed to the renderer
src/renderer/  Vite + React + TanStack Router desktop shell
src/shared/    IPC channel names and shared TypeScript contracts
resources/     icons and installer assets for future packaging work
```

CLI 연결을 구현할 때는 `src/main/` 아래에서 `slidex` 바이너리를 child process로
실행하고, `src/shared/api.ts`의 contract를 확장하는 방식으로 진행합니다.
