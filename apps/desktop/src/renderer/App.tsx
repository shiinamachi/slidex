import type { ReactElement } from "react";
import { createRootRoute, createRouter } from "@tanstack/react-router";

type StatusTone = "info" | "success" | "warning";

interface WorkspaceItem {
  label: string;
  meta: string;
  active?: boolean;
}

interface PipelineStep {
  label: string;
  detail: string;
  tone: StatusTone;
}

const workspaces: WorkspaceItem[] = [
  { label: "Decks", meta: "12", active: true },
  { label: "Runs", meta: "3" },
  { label: "QA", meta: "5" },
  { label: "Packages", meta: "2" }
];

const pipelineSteps: PipelineStep[] = [
  {
    label: "Spec sync",
    detail: "brief.md -> deck_spec.json",
    tone: "success"
  },
  {
    label: "HTML render",
    detail: "final_deck.html current",
    tone: "info"
  },
  {
    label: "Visual QA",
    detail: "2 slides need review",
    tone: "warning"
  }
];

const statusDotClass: Record<StatusTone, string> = {
  info: "bg-state-info",
  success: "bg-state-success",
  warning: "bg-state-warning"
};

const statusBadgeClass: Record<StatusTone, string> = {
  info: "bg-state-info-soft text-state-info",
  success: "bg-state-success-soft text-state-success",
  warning: "bg-state-warning-soft text-state-warning"
};

function classNames(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(" ");
}

function DesktopRoot(): ReactElement {
  const desktopPlatform = window.slidex?.platform ?? "linux";

  return (
    <main
      className="desktop-root bg-canvas text-ink antialiased"
      data-platform={desktopPlatform}
      aria-label="slidex Desktop"
    >
      <div className="grid min-h-screen grid-cols-[240px_minmax(0,1fr)] grid-rows-[60px_minmax(0,1fr)]">
        <div className="ds-titlebar-traffic-zone col-start-1 row-start-1" aria-hidden="true" />

        <header className="ds-command-bar col-start-2 row-start-1 flex h-[60px] items-center justify-between">
          <div>
            <p className="text-[11px] font-semibold uppercase text-ink-subtle">Workspace</p>
            <h1 className="text-lg font-semibold leading-6 text-ink">Investor Update Deck</h1>
          </div>
          <div className="flex items-center gap-2">
            <button className="ds-btn ds-btn-secondary" type="button">
              Preview
            </button>
            <button className="ds-btn ds-btn-primary" type="button">
              New deck
            </button>
          </div>
        </header>

        <aside className="ds-rail col-start-1 row-start-2 px-4 py-5">
          <div className="flex h-full flex-col gap-7">
            <div className="flex items-center gap-3 px-1">
              <div className="ds-brand-mark" aria-hidden="true">
                sx
              </div>
              <div>
                <p className="text-sm font-semibold leading-5 text-ink">slidex</p>
                <p className="text-xs leading-4 text-ink-subtle">Desktop workspace</p>
              </div>
            </div>

            <nav className="flex flex-col gap-0.5" aria-label="Workspace">
              {workspaces.map((item) => (
                <button
                  key={item.label}
                  className={classNames(
                    "ds-nav-item text-left",
                    item.active && "ds-nav-item-active"
                  )}
                  aria-current={item.active ? "page" : undefined}
                  type="button"
                >
                  <span>{item.label}</span>
                  <span
                    className={classNames(
                      "rounded-full px-2 py-0.5 text-xs font-medium",
                      item.active
                        ? "bg-surface text-accent-strong"
                        : "bg-canvas-soft text-ink-subtle"
                    )}
                  >
                    {item.meta}
                  </span>
                </button>
              ))}
            </nav>

            <div className="ds-rail-card mt-auto">
              <div className="mb-2.5 flex items-center justify-between">
                <p className="text-[11px] font-semibold uppercase text-ink-subtle">Local CLI</p>
                <span className="relative flex size-2">
                  <span className="absolute inline-flex size-full animate-ping rounded-full bg-state-success opacity-40" />
                  <span className="relative inline-flex size-2 rounded-full bg-state-success" />
                </span>
              </div>
              <p className="text-sm font-semibold text-ink">slidex 0.1.0</p>
              <p className="mt-1 text-xs leading-5 text-ink-muted">Renderer, QA, package jobs ready</p>
            </div>
          </div>
        </aside>

        <section className="col-start-2 row-start-2 flex min-h-0 min-w-0 flex-col">
          <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_300px] xl:gap-5 xl:p-5">
            <div className="flex min-w-0 flex-col gap-4 xl:gap-5">
              <section className="ds-panel p-5">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <p className="text-sm font-medium text-ink-muted">Current deck</p>
                    <h2 className="mt-1 text-2xl font-semibold leading-8 text-ink">
                      Q3 board narrative
                    </h2>
                  </div>
                  <span className="ds-badge ds-badge-accent">HTML current</span>
                </div>

                <div className="mt-6 grid grid-cols-1 gap-3 md:grid-cols-3">
                  <MetricCard label="Slides" value="14" detail="16:9 widescreen" />
                  <MetricCard label="Sources" value="27" detail="all claims mapped" />
                  <MetricCard label="Exports" value="PDF" detail="last render 09:42" />
                </div>
              </section>

              <section className="ds-panel p-5">
                <div className="mb-4 flex items-center justify-between gap-4">
                  <div>
                    <h2 className="text-base font-semibold text-ink">Run pipeline</h2>
                    <p className="text-sm text-ink-muted">Spec, render, QA, package</p>
                  </div>
                  <div className="ds-segmented">
                    <button className="ds-segment ds-segment-active" type="button">
                      Today
                    </button>
                    <button className="ds-segment" type="button">
                      All
                    </button>
                  </div>
                </div>

                <div className="grid gap-2.5">
                  {pipelineSteps.map((step) => (
                    <div className="ds-pipeline-row" key={step.label}>
                      <span
                        className={classNames("size-2 rounded-full ring-2 ring-surface", statusDotClass[step.tone])}
                      />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-ink">{step.label}</p>
                        <p className="truncate text-xs text-ink-muted">{step.detail}</p>
                      </div>
                      <span className={classNames("ds-badge", statusBadgeClass[step.tone])}>
                        {step.tone}
                      </span>
                    </div>
                  ))}
                </div>
              </section>
            </div>

            <aside className="ds-panel p-5">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <h2 className="text-base font-semibold text-ink">Inspector</h2>
                  <p className="truncate text-sm text-ink-muted">decks/investor-update</p>
                </div>
                <span className="ds-badge bg-state-success-soft text-state-success">Ready</span>
              </div>

              <dl className="mt-5 grid gap-3.5">
                <InspectorRow label="HTML" value="current" />
                <InspectorRow label="PNG slides" value="14 rendered" />
                <InspectorRow label="PDF" value="fresh" />
                <InspectorRow label="QA report" value="2 notes" />
              </dl>

              <div className="mt-6 border-t border-border-subtle pt-5">
                <p className="text-[11px] font-semibold uppercase text-ink-subtle">Recent checks</p>
                <div className="mt-3 grid gap-2">
                  <p className="ds-callout bg-accent-soft text-accent-strong">HTML synced after edit</p>
                  <p className="ds-callout bg-state-info-soft text-state-info">Render manifest refreshed</p>
                </div>
              </div>
            </aside>
          </div>
        </section>
      </div>
    </main>
  );
}

function MetricCard({
  label,
  value,
  detail
}: {
  label: string;
  value: string;
  detail: string;
}): ReactElement {
  return (
    <div className="ds-metric-card">
      <p className="text-[11px] font-semibold uppercase text-ink-subtle">{label}</p>
      <p className="mt-2 text-xl font-semibold tabular-nums text-ink">{value}</p>
      <p className="mt-1 text-xs leading-5 text-ink-muted">{detail}</p>
    </div>
  );
}

function InspectorRow({ label, value }: { label: string; value: string }): ReactElement {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border-subtle pb-3.5 last:border-b-0 last:pb-0">
      <dt className="text-sm text-ink-muted">{label}</dt>
      <dd className="text-sm font-semibold text-ink">{value}</dd>
    </div>
  );
}

const rootRoute = createRootRoute({
  component: DesktopRoot
});

export const router = createRouter({
  routeTree: rootRoute
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
