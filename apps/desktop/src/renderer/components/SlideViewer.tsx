import { useState } from "react";
import type { ReactElement } from "react";
import type { Deck } from "./DeckManager";

// Import generated slide images
import spikaImage from "../spikalabs.png";
import virtualCompanyImage from "../virtual_company.png";

interface SlideViewerProps {
  activeDeck: Deck | null;
}

export function SlideViewer({ activeDeck }: SlideViewerProps): ReactElement {
  const [selectedSlideIdx, setSelectedSlideIdx] = useState(0);
  const [isSyncing, setIsSyncing] = useState(false);
  const [syncMessage, setSyncMessage] = useState("");

  if (!activeDeck) {
    return (
      <div className="card" style={{ padding: "40px", textAlign: "center" }}>
        <p style={{ color: "var(--text-muted)" }}>활성화된 슬라이드 덱이 없습니다. 작업공간 탭에서 먼저 덱을 선택해주세요.</p>
      </div>
    );
  }

  // Choose image based on deck
  const slideImage = activeDeck.id === "spikalabs" ? spikaImage : virtualCompanyImage;
  const slideCount = activeDeck.slidesCount;

  const handleSyncHtml = () => {
    setIsSyncing(true);
    setSyncMessage("HTML 변경점 감지 중...");
    setTimeout(() => {
      setSyncMessage("user edits와 generated_baseline.html 비교 완료.");
      setTimeout(() => {
        setSyncMessage("deck_spec.json 및 렌더링된 슬라이드 이미지 갱신 완료!");
        setIsSyncing(false);
      }, 1000);
    }, 1000);
  };

  return (
    <div className="slide-explorer">
      {/* Left panel: Slide Previews */}
      <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
        <div className="card" style={{ padding: "16px" }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "16px" }}>
            <h3 style={{ fontSize: "16px" }}>슬라이드 미리보기 (Slide {selectedSlideIdx + 1} / {slideCount})</h3>
            <div style={{ display: "flex", gap: "10px" }}>
              <button
                className="btn btn-secondary"
                onClick={handleSyncHtml}
                disabled={isSyncing}
                style={{ padding: "6px 12px", fontSize: "12px" }}
              >
                {isSyncing ? "동기화 중..." : "HTML 수정 동기화 (sync-html-edits)"}
              </button>
            </div>
          </div>

          <div className="slide-preview-large">
            <img src={slideImage} alt={`Slide ${selectedSlideIdx + 1}`} />
          </div>

          {isSyncing && (
            <div style={{
              marginTop: "12px",
              padding: "10px 14px",
              background: "var(--bg-tertiary)",
              borderLeft: "3px solid var(--accent-gold)",
              borderRadius: "4px",
              fontSize: "13px",
              color: "var(--accent-gold)"
            }}>
              ⚙️ {syncMessage}
            </div>
          )}
        </div>

        {/* Thumbnail grid */}
        <div className="slide-grid">
          {Array.from({ length: slideCount }).map((_, idx) => (
            <div
              key={idx}
              className={`slide-thumb ${selectedSlideIdx === idx ? "active" : ""}`}
              onClick={() => setSelectedSlideIdx(idx)}
            >
              <img src={slideImage} alt={`Thumbnail ${idx + 1}`} />
              <div className="slide-number">Slide {idx + 1}</div>
            </div>
          ))}
        </div>
      </div>

      {/* Right panel: QA Report & Deliverables */}
      <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
        <div className="qa-report-panel">
          <h3 style={{ fontSize: "16px", borderBottom: "1px solid var(--border-color)", paddingBottom: "10px" }}>
            QA 자동 검증 리포트
          </h3>
          
          <div style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
            <div className="qa-item" style={{ borderLeft: "4px solid var(--accent-success)" }}>
              <div style={{ flex: 1 }}>
                <strong>HTML Parse Check</strong>
                <p style={{ fontSize: "11px", color: "var(--text-muted)" }}>HTML 구조 및 닫는 태그 유효성 검사</p>
              </div>
              <span className="badge badge-success" style={{ padding: "2px 6px" }}>통과</span>
            </div>

            <div className="qa-item" style={{ borderLeft: "4px solid var(--accent-success)" }}>
              <div style={{ flex: 1 }}>
                <strong>Dimensions (1920x1080)</strong>
                <p style={{ fontSize: "11px", color: "var(--text-muted)" }}>16:9 해상도 출력 검증</p>
              </div>
              <span className="badge badge-success" style={{ padding: "2px 6px" }}>통과</span>
            </div>

            <div className="qa-item" style={{ borderLeft: "4px solid var(--accent-success)" }}>
              <div style={{ flex: 1 }}>
                <strong>Font Loader Validation</strong>
                <p style={{ fontSize: "11px", color: "var(--text-muted)" }}>Pretendard 폰트 로드 확인</p>
              </div>
              <span className="badge badge-success" style={{ padding: "2px 6px" }}>통과</span>
            </div>

            <div className="qa-item" style={{ borderLeft: "4px solid var(--accent-warning)" }}>
              <div style={{ flex: 1 }}>
                <strong>Overflow & Clipping Check</strong>
                <p style={{ fontSize: "11px", color: "var(--text-muted)" }}>텍스트 넘침 및 잘림 현상 분석</p>
              </div>
              <span className="badge badge-warning" style={{ padding: "2px 6px" }}>경고 (0건)</span>
            </div>

            <div className="qa-item" style={{ borderLeft: "4px solid var(--accent-success)" }}>
              <div style={{ flex: 1 }}>
                <strong>Claim Provenance</strong>
                <p style={{ fontSize: "11px", color: "var(--text-muted)" }}>팩트 데이터 출처 매핑</p>
              </div>
              <span className="badge badge-success" style={{ padding: "2px 6px" }}>100% 매핑</span>
            </div>
          </div>
        </div>

        <div className="card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          <h3 style={{ fontSize: "15px" }}>최종 Deliverables</h3>
          <div style={{ fontSize: "13px", display: "flex", flexDirection: "column", gap: "8px" }}>
            <div style={{ display: "flex", justifyContent: "space-between" }}>
              <span style={{ color: "var(--text-secondary)" }}>HTML 슬라이드:</span>
              <a href="#" style={{ color: "var(--accent-gold)", textDecoration: "none" }} onClick={(e) => e.preventDefault()}>
                final_deck.html ↗
              </a>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between" }}>
              <span style={{ color: "var(--text-secondary)" }}>최종 인쇄용 PDF:</span>
              <a href="#" style={{ color: "var(--accent-gold)", textDecoration: "none" }} onClick={(e) => e.preventDefault()}>
                final_deck.pdf ↗
              </a>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between" }}>
              <span style={{ color: "var(--text-secondary)" }}>QA 이미지 몽타주:</span>
              <a href="#" style={{ color: "var(--accent-gold)", textDecoration: "none" }} onClick={(e) => e.preventDefault()}>
                qa_montage.png ↗
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
