import { useState, useEffect, useRef } from "react";
import type { ReactElement } from "react";
import type { Deck } from "./DeckManager";

interface PipelineMonitorProps {
  activeDeck: Deck | null;
  onIntakeRequired: () => void;
  onBuildComplete: () => void;
}

interface Step {
  id: string;
  label: string;
  status: "idle" | "active" | "completed" | "error";
  command: string;
}

const INITIAL_STEPS: Step[] = [
  { id: "inspect", label: "Inspect (검사)", status: "idle", command: "slidex inspect --deck decks/{deck_id} --write" },
  { id: "intake", label: "Intake (Q&A)", status: "idle", command: "slidex intake --deck decks/{deck_id}" },
  { id: "strategy", label: "Strategy (전략)", status: "idle", command: "slidex strategy --deck decks/{deck_id}" },
  { id: "spec", label: "Spec (규격)", status: "idle", command: "slidex spec --deck decks/{deck_id}" },
  { id: "build", label: "Build (HTML)", status: "idle", command: "slidex build --deck decks/{deck_id}" },
  { id: "render", label: "Render (PNG)", status: "idle", command: "slidex render --deck decks/{deck_id}" },
  { id: "qa", label: "QA (품질 검증)", status: "idle", command: "slidex qa --deck decks/{deck_id}" },
  { id: "finalize", label: "Finalize (정리)", status: "idle", command: "slidex finalize --deck decks/{deck_id}" },
  { id: "package", label: "Package (배포)", status: "idle", command: "slidex package --deck decks/{deck_id}" }
];

export function PipelineMonitor({
  activeDeck,
  onIntakeRequired,
  onBuildComplete
}: PipelineMonitorProps): ReactElement {
  const [steps, setSteps] = useState<Step[]>(INITIAL_STEPS);
  const [isRunning, setIsRunning] = useState(false);
  const [terminalLogs, setTerminalLogs] = useState<Array<{ text: string; type: "cmd" | "info" | "success" | "error" }>>([
    { text: "System initialized. Waiting for pipeline execution...", type: "info" }
  ]);
  
  const terminalEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Scroll terminal to bottom when logs update
    terminalEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [terminalLogs]);

  useEffect(() => {
    // Reset steps when active deck changes
    if (activeDeck) {
      const updatedSteps = INITIAL_STEPS.map(step => ({
        ...step,
        status: (activeDeck.status === "ready" && step.id === "inspect" ? "completed" 
                : activeDeck.status === "intake_required" && ["inspect", "intake"].includes(step.id) ? "error" 
                : "idle") as Step["status"]
      }));
      setSteps(updatedSteps);
      setTerminalLogs([
        { text: `Active deck changed to [${activeDeck.name}]. Paths resolved.`, type: "info" },
        { text: `Current Status: ${activeDeck.status === "intake_required" ? "Q&A Response Needed" : "Ready"}`, type: activeDeck.status === "intake_required" ? "error" : "success" }
      ]);
    }
  }, [activeDeck]);

  const addLog = (text: string, type: "cmd" | "info" | "success" | "error" = "info") => {
    setTerminalLogs((prev) => [...prev, { text, type }]);
  };

  const simulateStepExecution = async (stepIndex: number): Promise<boolean> => {
    const step = steps[stepIndex];
    const deckId = activeDeck?.id ?? "active-deck";
    const actualCommand = step.command.replace("{deck_id}", deckId);

    // Set step to active
    setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "active" } : s));
    addLog(`$ ${actualCommand}`, "cmd");
    await new Promise(resolve => setTimeout(resolve, 800));

    if (step.id === "inspect") {
      addLog(`[inspect] Resolving deck path under decks/${deckId}...`, "info");
      addLog(`[inspect] Success. brief.md templates and design rules found.`, "success");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "intake") {
      addLog(`[intake] Checking configuration claims and source inventory...`, "info");
      if (activeDeck?.status === "intake_required") {
        addLog(`[intake] Error: Essential intake questions not answered in brief.md.`, "error");
        addLog(`[intake] Exit code: 3. Process suspended.`, "error");
        setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "error" } : s));
        onIntakeRequired();
        return false;
      } else {
        addLog(`[intake] All necessary audience/purpose claims verified.`, "success");
        setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
        return true;
      }
    }

    if (step.id === "strategy") {
      addLog(`[strategy] Analyzing sources and compiling strategy...`, "info");
      addLog(`[strategy] Created out/strategy.md. Story arc resolved.`, "success");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "spec") {
      addLog(`[spec] Validating schema constraints for deck_spec.json...`, "info");
      addLog(`[spec] Generated out/deck_spec.json successfully.`, "success");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "build") {
      addLog(`[build] Compiling static HTML/CSS files...`, "info");
      addLog(`[build] Created out/final_deck.html (Static Widescreen Slide Deck).`, "success");
      addLog(`[build] Saved baseline to out/final_deck.generated_baseline.html.`, "info");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "render") {
      addLog(`[render] Launching Puppeteer browser with Chrome Sandbox...`, "info");
      addLog(`[render] Rendering 9 slides to out/rendered_slides/*.png...`, "info");
      addLog(`[render] Saved out/final_deck.pdf (paginated).`, "success");
      addLog(`[render] Generated out/render_manifest.json.`, "info");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "qa") {
      addLog(`[qa] Running automated validation...`, "info");
      addLog(`[qa] - Checking slide dimensions (1920x1080)... OK`, "success");
      addLog(`[qa] - Font loader check ('pretendard')... OK`, "success");
      addLog(`[qa] - Text overflow & clipping analysis... OK (0 warnings)`, "success");
      addLog(`[qa] Created out/qa_montage.png and out/qa_report.md.`, "success");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "finalize") {
      addLog(`[finalize] Cleaning temporary cache files...`, "info");
      addLog(`[finalize] Synthesizing notes.md.`, "success");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      return true;
    }

    if (step.id === "package") {
      addLog(`[package] Bundling final deliverables to zip package...`, "info");
      addLog(`[package] Saved packages/final_deck_${deckId}.zip`, "success");
      addLog(`[package] Pipeline completed successfully. Output ready for review.`, "success");
      setSteps(prev => prev.map((s, idx) => idx === stepIndex ? { ...s, status: "completed" } : s));
      onBuildComplete();
      return true;
    }

    return false;
  };

  const handleRunPipeline = async () => {
    if (!activeDeck) {
      addLog("Error: No active deck selected.", "error");
      return;
    }

    setIsRunning(true);
    addLog(`Starting full pipeline execution for: ${activeDeck.name}...`, "info");

    // Reset step status
    setSteps(prev => prev.map(s => ({ ...s, status: "idle" })));

    for (let i = 0; i < steps.length; i++) {
      const success = await simulateStepExecution(i);
      if (!success) {
        setIsRunning(false);
        return; // Stopped on error (e.g. intake suspended)
      }
    }

    setIsRunning(false);
  };

  const handleSingleStep = async (stepId: string) => {
    if (!activeDeck) return;
    const index = steps.findIndex(s => s.id === stepId);
    if (index === -1) return;

    setIsRunning(true);
    await simulateStepExecution(index);
    setIsRunning(false);
  };

  return (
    <div className="pipeline-layout">
      <div className="card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <div>
            <h2 style={{ fontSize: "20px" }}>빌드 파이프라인 제어</h2>
            <p style={{ fontSize: "13px", color: "var(--text-secondary)", marginTop: "4px" }}>
              선택된 덱: <strong>{activeDeck?.name ?? "없음"}</strong>
            </p>
          </div>
          <button
            className="btn btn-primary"
            onClick={handleRunPipeline}
            disabled={isRunning || !activeDeck}
            style={{ opacity: !activeDeck || isRunning ? 0.6 : 1 }}
          >
            {isRunning ? "실행 중..." : "전체 빌드 실행 (slidex run)"}
          </button>
        </div>

        {/* Stepper */}
        <div className="stepper-container">
          {steps.map((step, idx) => {
            const isCompleted = step.status === "completed";
            const isActive = step.status === "active";
            const isError = step.status === "error";
            
            let statusClass = "";
            if (isCompleted) statusClass = "completed";
            else if (isActive) statusClass = "active";
            else if (isError) statusClass = "error";

            return (
              <div key={step.id} className={`step-node ${statusClass}`}>
                <div
                  className="step-dot"
                  onClick={() => !isRunning && activeDeck && handleSingleStep(step.id)}
                  style={{ cursor: isRunning ? "not-allowed" : "pointer" }}
                  title="이 단계만 개별 실행"
                >
                  {isCompleted ? "✓" : isError ? "✗" : idx + 1}
                </div>
                <div className="step-label">{step.label.split(" ")[0]}</div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Terminal View */}
      <div className="terminal">
        <div className="terminal-header">
          <div className="terminal-buttons">
            <span className="terminal-btn term-close"></span>
            <span className="terminal-btn term-min"></span>
            <span className="terminal-btn term-max"></span>
          </div>
          <span className="terminal-title">slidex CLI Console</span>
          <span style={{ fontSize: "11px", color: "var(--text-muted)" }}>
            {activeDeck ? `decks/${activeDeck.id}` : "no deck"}
          </span>
        </div>
        <div className="terminal-body">
          {terminalLogs.map((log, index) => (
            <div key={index} className={`terminal-line ${log.type}`}>
              {log.text}
            </div>
          ))}
          <div ref={terminalEndRef} />
        </div>
      </div>

      <div style={{ display: "flex", gap: "10px", marginTop: "10px" }}>
        <button
          className="btn btn-secondary"
          onClick={() => {
            setTerminalLogs([
              { text: "Console logs cleared. Ready.", type: "info" }
            ]);
          }}
        >
          로그 비우기
        </button>
      </div>
    </div>
  );
}
