import { For, Show, createEffect, createMemo, createSignal, onCleanup } from "solid-js";

type WorkbenchInput = {
  initialRequest: string;
  title: string;
  audience: string;
  decisionGoal: string;
  sourceNotes: string;
  keyMessages: string;
  requiredClaims: string;
  constraints: string;
  outputExpectations: string;
};

type WorkbenchBoot = Partial<WorkbenchInput> & {
  apiBase: string;
  assetBase: string;
  deckDir: string;
  deckId: string;
  filePathHTML: string;
  sessionId: string;
  token: string;
  workbenchBase: string;
};

type WorkbenchManifest = {
  generationLog?: string;
  generationStatus?: string;
};

type DraftResponse = {
  draft?: {
    input?: Partial<WorkbenchInput>;
  };
};

type WorkbenchResponse = {
  error?: string;
  manifest?: WorkbenchManifest;
  status?: string;
};

declare global {
  interface Window {
    __SLIDEX_WORKBENCH__?: WorkbenchBoot;
  }
}

const boot = window.__SLIDEX_WORKBENCH__;
const requiredFields = ["title", "audience", "decisionGoal", "sourceNotes", "keyMessages", "outputExpectations"] as const;
const fieldLabels: Record<keyof WorkbenchInput, string> = {
  initialRequest: "요청 원문",
  title: "문서 제목",
  audience: "핵심 청중",
  decisionGoal: "결정 목표",
  sourceNotes: "근거 자료와 확정된 사실",
  keyMessages: "핵심 메시지",
  requiredClaims: "검증 필요 주장",
  constraints: "제외/주의사항",
  outputExpectations: "출력 기대",
};
const placeholders: Record<keyof WorkbenchInput, string> = {
  initialRequest: "플러그인 호출 때 남긴 요청을 그대로 보존합니다.",
  title: "예: 2026 파트너십 제안서",
  audience: "예: 전략 제휴 심사위원, 임원진, 조달 담당자",
  decisionGoal: "예: 3개월 파일럿 승인 여부 결정",
  sourceNotes: "확정된 사실, 사용 가능한 자료, 출처 상태를 적어주세요.",
  keyMessages: "반드시 전달해야 하는 3-5개 메시지를 적어주세요.",
  requiredClaims: "수치, 성과, 인증, 보안 주장을 검증 상태와 함께 적어주세요.",
  constraints: "제외할 표현, 민감 정보, 톤, 법무/보안 제약을 적어주세요.",
  outputExpectations: "슬라이드 수, 형식, 언어, 디자인 방향, PDF 기대치를 적어주세요.",
};
const steps: Array<{ id: string; title: string; fields: Array<keyof WorkbenchInput> }> = [
  { id: "request", title: "요청 정리", fields: ["initialRequest", "title", "audience"] },
  { id: "goal", title: "목표", fields: ["decisionGoal", "outputExpectations"] },
  { id: "evidence", title: "근거", fields: ["sourceNotes", "requiredClaims"] },
  { id: "message", title: "메시지", fields: ["keyMessages", "constraints"] },
  { id: "review", title: "검토", fields: [] },
];
const firstStep = steps[0]!;

function emptyInput(source: WorkbenchBoot): WorkbenchInput {
  return {
    initialRequest: source.initialRequest || "",
    title: source.title || "",
    audience: source.audience || "",
    decisionGoal: source.decisionGoal || "",
    sourceNotes: source.sourceNotes || "",
    keyMessages: source.keyMessages || "",
    requiredClaims: source.requiredClaims || "",
    constraints: source.constraints || "",
    outputExpectations: source.outputExpectations || "",
  };
}

function trimValues(values: WorkbenchInput): WorkbenchInput {
  return Object.fromEntries(
    Object.entries(values).map(([key, value]) => [key, String(value || "").trim()]),
  ) as WorkbenchInput;
}

function missingRequired(values: WorkbenchInput): Array<(typeof requiredFields)[number]> {
  const trimmed = trimValues(values);
  return requiredFields.filter((key) => trimmed[key].length < 8);
}

function updateValue(values: WorkbenchInput, key: keyof WorkbenchInput, value: string): WorkbenchInput {
  return { ...values, [key]: value };
}

function FieldControl(props: {
  keyName: keyof WorkbenchInput;
  multiline: boolean;
  setValues: (updater: (current: WorkbenchInput) => WorkbenchInput) => void;
  values: WorkbenchInput;
}) {
  const common = {
    name: props.keyName,
    value: props.values[props.keyName],
    placeholder: placeholders[props.keyName],
    spellcheck: true,
    onInput: (event: InputEvent & { currentTarget: HTMLInputElement | HTMLTextAreaElement }) =>
      props.setValues((current) => updateValue(current, props.keyName, event.currentTarget.value)),
  };
  return (
    <label class={props.multiline ? "field field-wide" : "field"}>
      <span>{fieldLabels[props.keyName]}</span>
      <Show
        when={props.multiline}
        fallback={<input {...common} autocomplete="off" />}
      >
        <textarea {...common} />
      </Show>
    </label>
  );
}

function StatusLine(props: { status: string; warning: boolean }) {
  return <output classList={{ status: true, warn: props.warning }}>{props.status}</output>;
}

export default function Workbench() {
  if (!boot) {
    return <div class="status warn">Workbench bootstrap data is unavailable.</div>;
  }
  const config = boot;

  const [values, setValues] = createSignal(emptyInput(config));
  const [stepIndex, setStepIndex] = createSignal(0);
  const [status, setStatus] = createSignal("Workbench ready");
  const [warning, setWarning] = createSignal(false);
  const [saving, setSaving] = createSignal(false);
  const [manifest, setManifest] = createSignal<WorkbenchManifest | null>(null);
  const activeStep = createMemo(() => steps[stepIndex()] || firstStep);
  const missing = createMemo(() => missingRequired(values()));
  const completionReady = createMemo(() => missing().length === 0);
  let saveTimer: ReturnType<typeof setTimeout> | undefined;

  createEffect(() => {
    let cancelled = false;
    fetch(`${config.apiBase}/draft`, { headers: { "X-Slidex-Workbench-Token": config.token } })
      .then((response) => (response.ok ? response.json() as Promise<DraftResponse> : null))
      .then((payload) => {
        if (cancelled || !payload?.draft?.input) return;
        setValues((current) => ({ ...current, ...payload.draft?.input }));
        setStatus("Recovered draft from out/workbench_draft.json");
      })
      .catch(() => undefined);
    onCleanup(() => {
      cancelled = true;
    });
  });

  createEffect(() => {
    if (manifest()?.generationStatus !== "running") return;
    const timer = setInterval(() => {
      fetch(`${config.apiBase}/session`, { headers: { "X-Slidex-Workbench-Token": config.token } })
        .then((response) => (response.ok ? response.json() as Promise<WorkbenchManifest> : null))
        .then((payload) => {
          if (!payload) return;
          setManifest(payload);
          if (payload.generationStatus === "completed") {
            setStatus("Generation completed");
            setWarning(false);
          } else if (payload.generationStatus === "failed") {
            setStatus("Generation failed; inspect the generation log");
            setWarning(true);
          }
        })
        .catch(() => undefined);
    }, 2500);
    onCleanup(() => clearInterval(timer));
  });

  async function post(path: string, nextStatus: string): Promise<WorkbenchResponse> {
    const data = trimValues(values());
    setSaving(true);
    setWarning(false);
    setStatus(nextStatus);
    try {
      const response = await fetch(`${config.apiBase}${path}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Slidex-Workbench-Token": config.token,
        },
        body: JSON.stringify(data),
      });
      const payload = response.headers.get("content-type")?.includes("application/json")
        ? await response.json() as WorkbenchResponse
        : {};
      if (!response.ok) {
        throw new Error(payload.error || response.statusText || "request failed");
      }
      if (payload.manifest) setManifest(payload.manifest);
      return payload;
    } finally {
      setSaving(false);
    }
  }

  function saveDraft() {
    const data = trimValues(values());
    if (!Object.values(data).some(Boolean)) return;
    post("/draft", "Saving draft...")
      .then(() => setStatus("Draft saved"))
      .catch(() => {
        setStatus("Draft save failed");
        setWarning(true);
      });
  }

  createEffect(() => {
    values();
    clearTimeout(saveTimer);
    saveTimer = setTimeout(saveDraft, 700);
    onCleanup(() => clearTimeout(saveTimer));
  });

  function saveBrief(event: SubmitEvent) {
    event.preventDefault();
    post("/save", "Saving brief...")
      .then(() => {
        setStatus("Saved to brief.md and out/workbench_manifest.json");
        setWarning(false);
      })
      .catch(() => {
        setStatus("Save failed");
        setWarning(true);
      });
  }

  function complete(event: MouseEvent) {
    event.preventDefault();
    if (!completionReady()) {
      setStatus("Complete the required answers before generation");
      setWarning(true);
      return;
    }
    post("/complete", "Starting generation...")
      .then((payload) => {
        setStatus(payload.status === "generation_reused" ? "Generation already running" : "Generation started");
        setWarning(false);
      })
      .catch(() => {
        setStatus("Generation start failed");
        setWarning(true);
      });
  }

  return (
    <div class="wizard-shell">
      <header class="wizard-header">
        <div>
          <p class="eyebrow">slidex Solid Workbench</p>
          <h1>Deck intake workbench</h1>
          <div class="meta">Deck: <strong>{config.deckId}</strong></div>
        </div>
        <div class="meta deck-dir">{config.deckDir}</div>
      </header>
      <nav class="stepper" aria-label="Workbench steps">
        <For each={steps}>
          {(step, index) => (
            <button
              classList={{ step: true, active: index() === stepIndex() }}
              type="button"
              onClick={() => setStepIndex(index())}
            >
              {index() + 1}. {step.title}
            </button>
          )}
        </For>
      </nav>
      <form id="deck-form" onSubmit={saveBrief}>
        <section class="step-panel">
          <h2>{activeStep().title}</h2>
          <div class="field-grid">
            <Show
              when={activeStep().fields.length > 0}
              fallback={
                <div class="review-grid">
                  <For each={requiredFields}>
                    {(key) => (
                      <div classList={{ "review-item": true, missing: missing().includes(key) }}>
                        <strong>{fieldLabels[key]}</strong>
                        <span>{missing().includes(key) ? "Needs detail" : "Ready"}</span>
                      </div>
                    )}
                  </For>
                </div>
              }
            >
              <For each={activeStep().fields}>
                {(key) => (
                  <FieldControl
                    keyName={key}
                    multiline={key !== "title" && key !== "audience"}
                    values={values()}
                    setValues={setValues}
                  />
                )}
              </For>
            </Show>
          </div>
        </section>
        <div class="actions">
          <button type="button" disabled={stepIndex() === 0} onClick={() => setStepIndex(Math.max(0, stepIndex() - 1))}>Back</button>
          <button type="button" disabled={stepIndex() >= steps.length - 1} onClick={() => setStepIndex(Math.min(steps.length - 1, stepIndex() + 1))}>Next</button>
          <button type="submit" disabled={saving()}>Save brief</button>
          <button type="button" class="primary" disabled={saving() || !completionReady()} onClick={complete}>Complete & generate</button>
          <StatusLine status={status()} warning={warning()} />
        </div>
      </form>
      <Show when={manifest()?.generationStatus}>
        <section class="generation">
          <h2>Generation</h2>
          <dl>
            <dt>Status</dt>
            <dd>{manifest()?.generationStatus}</dd>
            <Show when={manifest()?.generationLog}>
              <dt>Log</dt>
              <dd>{manifest()?.generationLog}</dd>
            </Show>
          </dl>
        </section>
      </Show>
      <section class="paths" aria-label="Deck files">
        <h2>Deck files</h2>
        <dl innerHTML={config.filePathHTML} />
      </section>
    </div>
  );
}
