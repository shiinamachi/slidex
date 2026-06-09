import { createContext, useContext, useEffect, useState } from "react";
import type { ReactElement } from "react";
import { Link, Outlet, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import type { DesktopAppInfo, SlidexStatus } from "../shared/api";

const navigationItems = [
  { label: "작업공간", to: "/" },
  { label: "실행", to: "/run" },
  { label: "자료", to: "/sources" },
  { label: "설정", to: "/settings" }
] as const;

type DesktopShellState = {
  appInfo: DesktopAppInfo | null;
  status: SlidexStatus | null;
};

const DesktopShellContext = createContext<DesktopShellState>({
  appInfo: null,
  status: null
});

function useDesktopShell(): DesktopShellState {
  return useContext(DesktopShellContext);
}

function AppLayout(): ReactElement {
  const [appInfo, setAppInfo] = useState<DesktopAppInfo | null>(null);
  const [status, setStatus] = useState<SlidexStatus | null>(null);

  useEffect(() => {
    void window.slidex.getAppInfo().then(setAppInfo);
    void window.slidex.getSlidexStatus().then(setStatus);
  }, []);

  return (
    <DesktopShellContext.Provider value={{ appInfo, status }}>
      <div className="shell">
        <aside className="sidebar" aria-label="주요 탐색">
          <div className="brand">
            <span className="brand-mark">sx</span>
            <div>
              <strong>slidex</strong>
              <span>Desktop</span>
            </div>
          </div>

          <nav className="nav-list">
            {navigationItems.map((item) => (
              <Link
                activeOptions={{ exact: item.to === "/" }}
                activeProps={{ className: "active" }}
                key={item.to}
                to={item.to}
              >
                {item.label}
              </Link>
            ))}
          </nav>
        </aside>

        <Outlet />
      </div>
    </DesktopShellContext.Provider>
  );
}

function WorkspaceRoute(): ReactElement {
  const { appInfo, status } = useDesktopShell();

  return (
    <main className="workspace">
      <header className="workspace-header">
        <div>
          <p className="eyebrow">Desktop Workspace</p>
          <h1>작업공간</h1>
        </div>
        <span className="version-chip">v{appInfo?.version ?? "0.1.0"}</span>
      </header>

      <section className="status-grid" aria-label="현재 상태">
        <div>
          <span>앱 모드</span>
          <strong>{appInfo?.mode === "production" ? "배포" : "개발"}</strong>
        </div>
        <div>
          <span>CLI Bridge</span>
          <strong>{status?.ready ? "준비됨" : "대기"}</strong>
        </div>
        <div>
          <span>Deck 경로</span>
          <strong>decks/*</strong>
        </div>
      </section>

      <section className="panel-grid">
        <article className="panel primary-panel">
          <div>
            <p className="panel-label">최근 Deck</p>
            <h2>연결 대기</h2>
          </div>
          <p>{status?.reason ?? "Desktop shell 로딩 중입니다."}</p>
        </article>

        <article className="panel">
          <p className="panel-label">실행 Queue</p>
          <h2>0</h2>
          <p>대기 중인 작업이 없습니다.</p>
        </article>

        <article className="panel">
          <p className="panel-label">산출물</p>
          <h2>out/</h2>
          <p>렌더링 결과는 기존 작업공간 규칙을 따릅니다.</p>
        </article>
      </section>
    </main>
  );
}

function RunRoute(): ReactElement {
  const { status } = useDesktopShell();

  return (
    <main className="workspace">
      <header className="workspace-header">
        <div>
          <p className="eyebrow">Workflow</p>
          <h1>실행</h1>
        </div>
      </header>

      <section className="panel-grid two-column">
        <article className="panel primary-panel">
          <div>
            <p className="panel-label">CLI Bridge</p>
            <h2>{status?.ready ? "준비됨" : "연결 대기"}</h2>
          </div>
          <p>{status?.reason ?? "CLI 상태를 확인하는 중입니다."}</p>
        </article>

        <article className="panel">
          <p className="panel-label">표준 명령</p>
          <h2>slidex run</h2>
          <p>Desktop 실행 흐름은 deck-local workspace 규칙을 기준으로 연결됩니다.</p>
        </article>
      </section>
    </main>
  );
}

function SourcesRoute(): ReactElement {
  return (
    <main className="workspace">
      <header className="workspace-header">
        <div>
          <p className="eyebrow">Evidence</p>
          <h1>자료</h1>
        </div>
      </header>

      <section className="panel-grid two-column">
        <article className="panel primary-panel">
          <div>
            <p className="panel-label">Deck 입력</p>
            <h2>brief.md</h2>
          </div>
          <p>Deck별 source, assets, brand, data 입력을 GUI에서 관리하는 영역입니다.</p>
        </article>

        <article className="panel">
          <p className="panel-label">근거 상태</p>
          <h2>검토 대기</h2>
          <p>자료 연결 후 unsupported claim 점검 결과를 이 화면에 표시합니다.</p>
        </article>
      </section>
    </main>
  );
}

function SettingsRoute(): ReactElement {
  const { appInfo } = useDesktopShell();

  return (
    <main className="workspace">
      <header className="workspace-header">
        <div>
          <p className="eyebrow">Desktop Settings</p>
          <h1>설정</h1>
        </div>
        <span className="version-chip">v{appInfo?.version ?? "0.1.0"}</span>
      </header>

      <section className="status-grid" aria-label="패키징 설정">
        <div>
          <span>Bundle ID</span>
          <strong>moe.shiinamachi.slidex.desktop</strong>
        </div>
        <div>
          <span>Runtime</span>
          <strong>Node 24.16.0 LTS</strong>
        </div>
        <div>
          <span>Package Manager</span>
          <strong>pnpm 11.5.2</strong>
        </div>
      </section>
    </main>
  );
}

const rootRoute = createRootRoute({
  component: AppLayout
});

const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: WorkspaceRoute
});

const runRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/run",
  component: RunRoute
});

const sourcesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sources",
  component: SourcesRoute
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsRoute
});

const routeTree = rootRoute.addChildren([workspaceRoute, runRoute, sourcesRoute, settingsRoute]);

export const router = createRouter({
  routeTree
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
