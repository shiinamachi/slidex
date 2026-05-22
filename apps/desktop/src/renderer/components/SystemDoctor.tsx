import { useState } from "react";
import type { ReactElement } from "react";

interface DoctorItem {
  name: string;
  expected: string;
  actual: string;
  status: "ok" | "warning" | "error";
  description: string;
}

export function SystemDoctor(): ReactElement {
  const [isDiagnosing, setIsDiagnosing] = useState(false);
  const [diagnostics, setDiagnostics] = useState<DoctorItem[]>([
    {
      name: "Go Runtime Version",
      expected: "go1.26.3 (pinned in .mise.toml)",
      actual: "go1.26.3",
      status: "ok",
      description: "slidex CLI 핵심 컴파일러 버전이 정확히 고정되어 있습니다."
    },
    {
      name: "Node.js Version",
      expected: "v24.16.0 (pinned in .mise.toml)",
      actual: "v24.16.0",
      status: "ok",
      description: "데스크탑 쉘 및 렌더러 빌드 환경의 런타임 버전 검증을 통과했습니다."
    },
    {
      name: "Chrome Sandbox Mode",
      expected: "Enabled (Default)",
      actual: "Enabled",
      status: "ok",
      description: "Puppeteer 렌더링 세션이 보안 샌드박스 내부에서 활성화되어 실행됩니다."
    },
    {
      name: "Codex App Server Schema",
      expected: "0.132.0 (compatible)",
      actual: "0.132.0",
      status: "ok",
      description: "프로토콜 스키마 및 호환 버전이 Local App Server와 완전히 일치합니다."
    },
    {
      name: "CLI Local Bridge Connection",
      expected: "Connected",
      actual: "Not Connected (Simulated)",
      status: "warning",
      description: "GUI 껍데기는 동작하나 CLI 실행 브릿지가 현재 에뮬레이션 상태입니다."
    }
  ]);

  const handleDiagnose = () => {
    setIsDiagnosing(true);
    setTimeout(() => {
      // Complete mock diagnosis
      setDiagnostics(prev =>
        prev.map(item =>
          item.name === "CLI Local Bridge Connection"
            ? { ...item, actual: "Connected (Simulated)", status: "ok", description: "CLI 로컬 브릿지 에뮬레이션이 안전하게 활성화되었습니다." }
            : item
        )
      );
      setIsDiagnosing(false);
    }, 1500);
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
      <div className="card" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <h2 style={{ fontSize: "20px", marginBottom: "4px" }}>System Doctor & 환경 진단</h2>
          <p style={{ fontSize: "13px", color: "var(--text-secondary)" }}>
            슬라이덱스가 사용하는 CLI 도구, 런타임 버전 핀 상태, 그리고 Chromium 브라우저 환경을 점검합니다.
          </p>
        </div>
        <button
          className="btn btn-primary"
          onClick={handleDiagnose}
          disabled={isDiagnosing}
        >
          {isDiagnosing ? "진단 중..." : "시스템 재진단 (slidex doctor)"}
        </button>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        {diagnostics.map((item, idx) => (
          <div key={idx} className="doctor-item">
            <div style={{ display: "flex", gap: "16px", alignItems: "center" }}>
              <span className={`doctor-status-dot ${
                item.status === "ok" ? "status-dot-ok" : item.status === "warning" ? "status-dot-warning" : "status-dot-error"
              }`} />
              <div className="doctor-info">
                <h4 style={{ color: "var(--text-primary)", fontWeight: "600" }}>{item.name}</h4>
                <p style={{ fontSize: "12px", color: "var(--text-secondary)", marginTop: "2px" }}>{item.description}</p>
                <div style={{ display: "flex", gap: "12px", fontSize: "11px", color: "var(--text-muted)", marginTop: "6px" }}>
                  <span>기대치: <code>{item.expected}</code></span>
                  <span>현재값: <code>{item.actual}</code></span>
                </div>
              </div>
            </div>
            
            <span className={`badge ${
              item.status === "ok" ? "badge-success" : item.status === "warning" ? "badge-warning" : "badge-error"
            }`} style={{ textTransform: "uppercase", fontSize: "11px" }}>
              {item.status === "ok" ? "정상" : item.status === "warning" ? "주의" : "오류"}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
