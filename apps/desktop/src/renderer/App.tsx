import { useEffect, useState } from "react";
import type { ReactElement } from "react";
import type { DesktopAppInfo, SlidexStatus } from "../shared/api";

const navigationItems = ["작업공간", "실행", "자료", "설정"];

export function App(): ReactElement {
  const [appInfo, setAppInfo] = useState<DesktopAppInfo | null>(null);
  const [status, setStatus] = useState<SlidexStatus | null>(null);

  useEffect(() => {
    void window.slidex.getAppInfo().then(setAppInfo);
    void window.slidex.getSlidexStatus().then(setStatus);
  }, []);

  return (
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
          {navigationItems.map((item, index) => (
            <button className={index === 0 ? "active" : ""} key={item} type="button">
              {item}
            </button>
          ))}
        </nav>
      </aside>

      <main className="workspace">
        <header className="workspace-header">
          <div>
            <p className="eyebrow">GUI 보일러플레이트</p>
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
    </div>
  );
}
