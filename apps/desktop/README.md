# slidex Desktop Tombstone

이 디렉터리는 과거 Electron prototype의 migration reference입니다. slidex의 canonical
UX는 더 이상 별도 desktop app이 아니라 `plugins/slidex` Codex Plugin과
`slidex workbench start --deck-id <deck_id>` loopback workbench입니다.

새 기능, 검증, product documentation은 Go CLI, plugin package, MCP server, workbench,
doctor checks에 구현합니다. 이 Electron prototype은 shipping 또는 future product path가
아닙니다.

## Runtime

루트 `.mise.toml`에서 Node `24.16.0`과 pnpm `11.5.2`를 exact pin합니다. 이 앱의
package manager pin은 `package.json`의 `packageManager` 필드에 기록된
`pnpm@11.5.2`입니다.

```bash
mise install
cd apps/desktop
pnpm install
```

## Historical Commands

These commands are only for inspecting the archived prototype. They are not part
of the normal slidex development, release, or Codex Plugin workbench validation
path.

```bash
pnpm run dev
pnpm run dev:wsl
pnpm run typecheck
pnpm run build
```

아래 명령은 과거 prototype을 조사해야 할 때만 사용합니다. `pnpm run dev:wsl`은 WSL에서 renderer/dev main watch를 실행하고 Windows host에서
pinned Electron dev 앱을 띄웁니다. Windows host에 `mise`가 있으면
PATH에 없더라도 WinGet/Scoop/mise shims의 일반 설치 위치를 찾아
`mise exec node@24.16.0 npm:pnpm@11.5.2 -- pnpm add --allow-build=electron`로
Windows 전용 Electron cache를 준비한 뒤 실행합니다. 없으면 `pnpm.cmd`,
`npm.cmd`, 그 다음 Windows Node.js의 `npx.cmd`를 fallback으로 사용합니다.

`pnpm run pack`과 `pnpm run dist`는 의도적으로 실패합니다. Electron prototype은
shipping path가 아니며 새 배포 가능한 UX는 Codex Plugin workbench 경로에 둡니다.

## Structure

```text
src/main/      Electron main process bootstrap and IPC registration
src/preload/   contextBridge API exposed to the renderer
src/renderer/  Vite + React + TanStack Router desktop shell
src/shared/    IPC channel names and shared TypeScript contracts
resources/     legacy icons and installer assets kept as migration reference
```

새 CLI 연결은 이 Electron shell에 추가하지 않습니다. 필요한 UX는 Codex Plugin workbench로
구현합니다.
