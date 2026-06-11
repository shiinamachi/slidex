(function () {
  "use strict";

  const boot = window.__SLIDEX_WORKBENCH__;
  const root = document.getElementById("slidex-react-root");
  if (!boot || !root || !window.React || !window.ReactDOM) return;

  const h = React.createElement;
  const requiredFields = ["title", "audience", "decisionGoal", "sourceNotes", "keyMessages", "outputExpectations"];
  const fieldLabels = {
    initialRequest: "요청 원문",
    title: "문서 제목",
    audience: "핵심 청중",
    decisionGoal: "결정 목표",
    sourceNotes: "근거 자료와 확정된 사실",
    keyMessages: "핵심 메시지",
    requiredClaims: "검증 필요 주장",
    constraints: "제외/주의사항",
    outputExpectations: "출력 기대"
  };
  const steps = [
    { id: "request", title: "요청 정리", fields: ["initialRequest", "title", "audience"] },
    { id: "goal", title: "목표", fields: ["decisionGoal", "outputExpectations"] },
    { id: "evidence", title: "근거", fields: ["sourceNotes", "requiredClaims"] },
    { id: "message", title: "메시지", fields: ["keyMessages", "constraints"] },
    { id: "review", title: "검토", fields: [] }
  ];
  const emptyInput = {
    initialRequest: boot.initialRequest || "",
    title: boot.title || "",
    audience: boot.audience || "",
    decisionGoal: boot.decisionGoal || "",
    sourceNotes: boot.sourceNotes || "",
    keyMessages: boot.keyMessages || "",
    requiredClaims: boot.requiredClaims || "",
    constraints: boot.constraints || "",
    outputExpectations: boot.outputExpectations || ""
  };

  function trimValues(values) {
    const out = {};
    Object.keys(emptyInput).forEach((key) => {
      out[key] = String(values[key] || "").trim();
    });
    return out;
  }

  function missingRequired(values) {
    const trimmed = trimValues(values);
    return requiredFields.filter((key) => trimmed[key].length < 8);
  }

  function fieldControl(values, setValues, key, multiline, placeholder) {
    const props = {
      name: key,
      value: values[key] || "",
      placeholder,
      onChange: (event) => setValues((current) => ({ ...current, [key]: event.target.value })),
      spellCheck: true
    };
    return h("label", { className: multiline ? "field field-wide" : "field", key },
      h("span", null, fieldLabels[key]),
      multiline ? h("textarea", props) : h("input", { ...props, autoComplete: "off" })
    );
  }

  function StatusLine({ status, warning }) {
    return h("output", { className: warning ? "status warn" : "status" }, status || "");
  }

  function App() {
    const [values, setValues] = React.useState(emptyInput);
    const [stepIndex, setStepIndex] = React.useState(0);
    const [status, setStatus] = React.useState("Wizard ready");
    const [warning, setWarning] = React.useState(false);
    const [saving, setSaving] = React.useState(false);
    const [manifest, setManifest] = React.useState(null);
    const saveTimer = React.useRef(null);
    const activeStep = steps[stepIndex];
    const missing = missingRequired(values);
    const completionReady = missing.length === 0;

    React.useEffect(() => {
      let cancelled = false;
      fetch(boot.apiBase + "/draft", { headers: { "X-Slidex-Workbench-Token": boot.token } })
        .then((response) => response.ok ? response.json() : null)
        .then((payload) => {
          if (cancelled || !payload || !payload.draft || !payload.draft.input) return;
          setValues((current) => ({ ...current, ...payload.draft.input }));
          setStatus("Recovered draft from out/workbench_draft.json");
        })
        .catch(() => {});
      return () => { cancelled = true; };
    }, []);

    React.useEffect(() => {
      if (!manifest || manifest.generationStatus !== "running") return;
      const timer = setInterval(() => {
        fetch(boot.apiBase + "/session", { headers: { "X-Slidex-Workbench-Token": boot.token } })
          .then((response) => response.ok ? response.json() : null)
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
          .catch(() => {});
      }, 2500);
      return () => clearInterval(timer);
    }, [manifest && manifest.generationStatus]);

    function post(path, nextStatus) {
      const data = trimValues(values);
      setSaving(true);
      setWarning(false);
      setStatus(nextStatus);
      return fetch(boot.apiBase + path, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Slidex-Workbench-Token": boot.token
        },
        body: JSON.stringify(data)
      }).then(async (response) => {
        const payload = response.headers.get("content-type") && response.headers.get("content-type").includes("application/json")
          ? await response.json()
          : {};
        if (!response.ok) {
          throw new Error(payload.error || response.statusText || "request failed");
        }
        if (payload.manifest) setManifest(payload.manifest);
        return payload;
      }).finally(() => setSaving(false));
    }

    function saveDraft() {
      const data = trimValues(values);
      if (!Object.values(data).some(Boolean)) return;
      post("/draft", "Saving draft...")
        .then(() => setStatus("Draft saved"))
        .catch(() => {
          setStatus("Draft save failed");
          setWarning(true);
        });
    }

    React.useEffect(() => {
      clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(saveDraft, 700);
      return () => clearTimeout(saveTimer.current);
    }, [values]);

    function saveBrief(event) {
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

    function complete(event) {
      event.preventDefault();
      if (!completionReady) {
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

    const stepFields = activeStep.fields.length > 0
      ? activeStep.fields.map((key) => fieldControl(values, setValues, key, key !== "title" && key !== "audience", {
          initialRequest: "플러그인 호출 때 남긴 요청을 그대로 보존합니다.",
          title: "예: 2026 파트너십 제안서",
          audience: "예: 전략 제휴 심사위원, 임원진, 조달 담당자",
          decisionGoal: "예: 3개월 파일럿 승인 여부 결정",
          sourceNotes: "확정된 사실, 사용 가능한 자료, 출처 상태를 적어주세요.",
          keyMessages: "반드시 전달해야 하는 3-5개 메시지를 적어주세요.",
          requiredClaims: "수치, 성과, 인증, 보안 주장을 검증 상태와 함께 적어주세요.",
          constraints: "제외할 표현, 민감 정보, 톤, 법무/보안 제약을 적어주세요.",
          outputExpectations: "슬라이드 수, 형식, 언어, 디자인 방향, PDF 기대치를 적어주세요."
        }[key]))
      : [
          h("div", { className: "review-grid", key: "review" },
            requiredFields.map((key) => h("div", { className: missing.includes(key) ? "review-item missing" : "review-item", key },
              h("strong", null, fieldLabels[key]),
              h("span", null, missing.includes(key) ? "Needs detail" : "Ready")
            ))
          )
        ];

    return h("div", { className: "wizard-shell" },
      h("header", { className: "wizard-header" },
        h("div", null,
          h("p", { className: "eyebrow" }, "slidex React Wizard"),
          h("h1", null, "Deck intake workbench"),
          h("div", { className: "meta" }, "Deck: ", h("strong", null, boot.deckId))
        ),
        h("div", { className: "meta deck-dir" }, boot.deckDir)
      ),
      h("nav", { className: "stepper", "aria-label": "Wizard steps" },
        steps.map((step, index) => h("button", {
          className: index === stepIndex ? "step active" : "step",
          type: "button",
          onClick: () => setStepIndex(index)
        }, String(index + 1), ". ", step.title))
      ),
      h("form", { id: "deck-form", onSubmit: saveBrief },
        h("section", { className: "step-panel" },
          h("h2", null, activeStep.title),
          h("div", { className: "field-grid" }, stepFields)
        ),
        h("div", { className: "actions" },
          h("button", { type: "button", disabled: stepIndex === 0, onClick: () => setStepIndex(Math.max(0, stepIndex - 1)) }, "Back"),
          h("button", { type: "button", disabled: stepIndex >= steps.length - 1, onClick: () => setStepIndex(Math.min(steps.length - 1, stepIndex + 1)) }, "Next"),
          h("button", { type: "submit", disabled: saving }, "Save brief"),
          h("button", { type: "button", className: "primary", disabled: saving || !completionReady, onClick: complete }, "Complete & generate"),
          h(StatusLine, { status, warning })
        )
      ),
      manifest && manifest.generationStatus ? h("section", { className: "generation" },
        h("h2", null, "Generation"),
        h("dl", null,
          h("dt", null, "Status"), h("dd", null, manifest.generationStatus),
          manifest.generationLog ? [h("dt", { key: "dt" }, "Log"), h("dd", { key: "dd" }, manifest.generationLog)] : null
        )
      ) : null,
      h("section", { className: "paths", "aria-label": "Deck files" },
        h("h2", null, "Deck files"),
        h("dl", { dangerouslySetInnerHTML: { __html: boot.filePathHTML } })
      )
    );
  }

  ReactDOM.createRoot(root).render(h(App));
})();
