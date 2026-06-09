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

const statusToneClass: Record<StatusTone, string> = {
  info: "bg-state-info text-ink-inverse",
  success: "bg-state-success text-ink-inverse",
  warning: "bg-state-warning text-ink-inverse"
};

const statusSoftClass: Record<StatusTone, string> = {
  info: "bg-state-info-soft text-state-info",
  success: "bg-state-success-soft text-state-success",
  warning: "bg-state-warning-soft text-state-warning"
};

function classNames(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(" ");
}

function DesktopRoot(): ReactElement {
  return (
    <main className="desktop-root bg-canvas text-ink antialiased" aria-label="slidex Desktop">
      <div className="grid min-h-screen grid-cols-[248px_minmax(0,1fr)]">
        <aside className="border-r border-border bg-surface/92 px-5 py-5 backdrop-blur">
          <div className="flex h-full flex-col gap-6">
            <div className="flex items-center gap-3">
              <div className="grid size-9 place-items-center rounded-ui bg-accent text-sm font-semibold text-accent-ink shadow-control">
                sx
              </div>
              <div>
                <p className="text-sm font-semibold leading-5 text-ink">slidex</p>
                <p className="text-xs leading-4 text-ink-muted">Desktop workspace</p>
              </div>
            </div>

            <nav className="flex flex-col gap-1" aria-label="Workspace">
              {workspaces.map((item) => (
                <button
                  key={item.label}
                  className={classNames(
                    "flex h-10 items-center justify-between rounded-ui px-3 text-left text-sm transition",
                    item.active
                      ? "bg-accent-soft font-semibold text-accent-strong ring-1 ring-accent-muted"
                      : "text-ink-muted hover:bg-surface-muted hover:text-ink"
                  )}
                  aria-current={item.active ? "page" : undefined}
                  type="button"
                >
                  <span>{item.label}</span>
                  <span
                    className={classNames(
                      "rounded-full px-2 py-0.5 text-xs",
                      item.active ? "bg-surface text-accent-strong" : "bg-canvas-soft text-ink-subtle"
                    )}
                  >
                    {item.meta}
                  </span>
                </button>
              ))}
            </nav>

            <div className="mt-auto rounded-panel border border-border bg-surface-muted p-3">
              <div className="mb-3 flex items-center justify-between">
                <p className="text-xs font-semibold uppercase text-ink-subtle">Local CLI</p>
                <span className="size-2 rounded-full bg-state-success" />
              </div>
              <p className="text-sm font-medium text-ink">slidex 0.1.0</p>
              <p className="mt-1 text-xs leading-5 text-ink-muted">Renderer, QA, package jobs ready</p>
            </div>
          </div>
        </aside>

        <section className="flex min-w-0 flex-col">
          <header className="flex h-16 items-center justify-between border-b border-border bg-surface/86 px-6 backdrop-blur">
            <div>
              <p className="text-xs font-medium uppercase text-ink-subtle">Workspace</p>
              <h1 className="text-lg font-semibold leading-6 text-ink">Investor Update Deck</h1>
            </div>
            <div className="flex items-center gap-2">
              <button
                className="h-9 rounded-control border border-border bg-surface px-3 text-sm font-medium text-ink-muted shadow-control transition hover:border-border-strong hover:text-ink"
                type="button"
              >
                Preview
              </button>
              <button
                className="h-9 rounded-control bg-accent px-3 text-sm font-semibold text-accent-ink shadow-control transition hover:bg-accent-muted"
                type="button"
              >
                New deck
              </button>
            </div>
          </header>

          <div className="grid min-h-0 flex-1 grid-cols-1 gap-5 p-5 xl:grid-cols-[minmax(0,1fr)_320px]">
            <div className="flex min-w-0 flex-col gap-5">
              <section className="rounded-panel border border-border bg-surface p-5 shadow-panel">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <p className="text-sm font-medium text-ink-muted">Current deck</p>
                    <h2 className="mt-1 text-2xl font-semibold leading-8 text-ink">
                      Q3 board narrative
                    </h2>
                  </div>
                  <span className="rounded-full bg-accent-soft px-3 py-1 text-xs font-semibold text-accent-strong ring-1 ring-accent-muted">
                    HTML current
                  </span>
                </div>

                <div className="mt-6 grid grid-cols-1 gap-3 md:grid-cols-3">
                  <MetricCard label="Slides" value="14" detail="16:9 widescreen" />
                  <MetricCard label="Sources" value="27" detail="all claims mapped" />
                  <MetricCard label="Exports" value="PDF" detail="last render 09:42" />
                </div>
              </section>

              <section className="rounded-panel border border-border bg-surface p-5 shadow-panel">
                <div className="mb-4 flex items-center justify-between">
                  <div>
                    <h2 className="text-base font-semibold text-ink">Run pipeline</h2>
                    <p className="text-sm text-ink-muted">Spec, render, QA, package</p>
                  </div>
                  <div className="rounded-ui border border-border bg-surface-muted p-1">
                    <button
                      className="h-8 rounded-control bg-surface px-3 text-sm font-semibold text-ink shadow-control"
                      type="button"
                    >
                      Today
                    </button>
                    <button className="h-8 rounded-control px-3 text-sm font-medium text-ink-muted" type="button">
                      All
                    </button>
                  </div>
                </div>

                <div className="grid gap-3">
                  {pipelineSteps.map((step) => (
                    <div
                      className="grid grid-cols-[12px_minmax(0,1fr)_auto] items-center gap-3 rounded-ui border border-border bg-surface-muted px-4 py-3"
                      key={step.label}
                    >
                      <span className={classNames("size-2 rounded-full", statusToneClass[step.tone])} />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-ink">{step.label}</p>
                        <p className="truncate text-xs text-ink-muted">{step.detail}</p>
                      </div>
                      <span className={classNames("rounded-full px-2.5 py-1 text-xs font-semibold", statusSoftClass[step.tone])}>
                        {step.tone}
                      </span>
                    </div>
                  ))}
                </div>
              </section>
            </div>

            <aside className="rounded-panel border border-border bg-surface p-5 shadow-panel">
              <div className="flex items-start justify-between">
                <div>
                  <h2 className="text-base font-semibold text-ink">Inspector</h2>
                  <p className="text-sm text-ink-muted">decks/investor-update</p>
                </div>
                <span className="rounded-full bg-state-success-soft px-2.5 py-1 text-xs font-semibold text-state-success">
                  Ready
                </span>
              </div>

              <dl className="mt-5 grid gap-4">
                <InspectorRow label="HTML" value="current" />
                <InspectorRow label="PNG slides" value="14 rendered" />
                <InspectorRow label="PDF" value="fresh" />
                <InspectorRow label="QA report" value="2 notes" />
              </dl>

              <div className="mt-6 border-t border-border pt-5">
                <p className="text-xs font-semibold uppercase text-ink-subtle">Recent checks</p>
                <div className="mt-3 grid gap-2">
                  <p className="rounded-control bg-accent-soft px-3 py-2 text-xs font-semibold text-accent-strong">
                    HTML synced after edit
                  </p>
                  <p className="rounded-control bg-state-info-soft px-3 py-2 text-xs font-semibold text-state-info">
                    Render manifest refreshed
                  </p>
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
    <div className="rounded-ui border border-border bg-surface-muted p-4">
      <p className="text-xs font-medium uppercase text-ink-subtle">{label}</p>
      <p className="mt-2 text-xl font-semibold text-ink">{value}</p>
      <p className="mt-1 text-xs leading-5 text-ink-muted">{detail}</p>
    </div>
  );
}

function InspectorRow({ label, value }: { label: string; value: string }): ReactElement {
  return (
    <div className="flex items-center justify-between gap-4">
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
