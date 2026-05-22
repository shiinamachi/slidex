import { useState, useEffect } from "react";
import type { ReactElement } from "react";
import type { Deck } from "./DeckManager";

interface IntakePanelProps {
  activeDeck: Deck | null;
  onSaveIntake: (deckId: string) => void;
}

export function IntakePanel({ activeDeck, onSaveIntake }: IntakePanelProps): ReactElement {
  const [docType, setDocType] = useState("proposal");
  const [audience, setAudience] = useState("");
  const [objective, setObjective] = useState("");
  const [claims, setClaims] = useState("");
  const [brandColor, setBrandColor] = useState("#d4af37");
  const [isSaved, setIsSaved] = useState(false);

  useEffect(() => {
    if (activeDeck) {
      setIsSaved(activeDeck.status === "ready");
      if (activeDeck.id === "spikalabs") {
        setDocType("ir");
        setAudience("VC 및 엔젤 투자자");
        setObjective("시리즈 A 투자 유치를 위한 핵심 사업 모델 및 성장 지표 전달");
        setClaims("1. 월간 활성 사용자(MAU) 50만 돌파 (자사 서비스 로그 기준)\n2. 전년 대비 매출액 180% 성장 (2025년도 감사보고서 기준)\n3. 3건의 핵심 특허 보유 (특허청 등록 원부 기준)");
        setBrandColor("#1e3a8a");
      } else if (activeDeck.id === "cli-test-virtual-company") {
        setDocType("government");
        setAudience("창업진흥원 정부 지원 사업 평가위원");
        setObjective("2026년도 기술창업 패키지 선정을 위한 혁신 기술 및 로드맵 입증");
        setClaims("1. 독자적인 AI 추론 가속화 알고리즘 개발 완료 (벤치마크 테스트 결과서 첨부)\n2. 창업팀 전원 석박사 및 대기업 연구소 출신");
        setBrandColor("#10b981");
      } else {
        // Reset for new decks
        setDocType("proposal");
        setAudience("");
        setObjective("");
        setClaims("");
        setBrandColor("#d4af37");
      }
    }
  }, [activeDeck]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!activeDeck) return;
    
    // Simulate updating active deck status and saving info
    setIsSaved(true);
    onSaveIntake(activeDeck.id);
  };

  if (!activeDeck) {
    return (
      <div className="card" style={{ padding: "40px", textAlign: "center" }}>
        <p style={{ color: "var(--text-muted)" }}>활성화된 슬라이드 덱이 없습니다. 작업공간 탭에서 먼저 덱을 선택해주세요.</p>
      </div>
    );
  }

  return (
    <div className="intake-container">
      <div className="card">
        <h2 style={{ fontSize: "20px", marginBottom: "8px" }}>Q&A Intake & brief.md 설정</h2>
        <p style={{ fontSize: "13px", color: "var(--text-secondary)" }}>
          슬라이덱스 AI는 brief.md를 분석하여 기획서 스펙과 슬라이드 골격을 결정합니다. 아래 정보를 채우면 AI 엔진이 즉시 기획 전략 수립을 재개합니다.
        </p>
      </div>

      <form onSubmit={handleSubmit} className="card" style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "20px" }}>
          <div className="form-group">
            <label>문서 유형 (Document Type)</label>
            <select value={docType} onChange={(e) => setDocType(e.target.value)}>
              <option value="ir">IR / 투자유치용 회사소개서</option>
              <option value="government">정부 지원 사업 사업계획서</option>
              <option value="proposal">비즈니스 제안서 (Proposal)</option>
              <option value="report">임원 보고 / 전략 보고서 (Executive Report)</option>
            </select>
          </div>

          <div className="form-group">
            <label>브랜드 핵심 컬러 (Primary Brand Color)</label>
            <div style={{ display: "flex", gap: "10px" }}>
              <input
                type="color"
                value={brandColor}
                onChange={(e) => setBrandColor(e.target.value)}
                style={{ width: "45px", height: "45px", padding: "2px", cursor: "pointer" }}
              />
              <input
                type="text"
                value={brandColor}
                onChange={(e) => setBrandColor(e.target.value)}
                style={{ flex: 1 }}
                placeholder="#hex_code"
              />
            </div>
          </div>
        </div>

        <div className="form-group">
          <label>주요 대상 청중 (Target Audience)</label>
          <input
            type="text"
            placeholder="예: 벤처캐피탈 투자 심사역, 대기업 전략 제휴 의사결정권자"
            value={audience}
            onChange={(e) => setAudience(e.target.value)}
            required
          />
        </div>

        <div className="form-group">
          <label>문서 작성 목적 및 기대 효과 (Objective)</label>
          <textarea
            placeholder="예: 우리 솔루션의 우수성과 시장 검증 지표를 제시하여 파트너십 구축 및 POC 승인 획득"
            value={objective}
            onChange={(e) => setObjective(e.target.value)}
            rows={3}
            required
          />
        </div>

        <div className="form-group">
          <label>핵심 팩트 및 근거 데이터 목록 (Claims & Sources)</label>
          <p style={{ fontSize: "11px", color: "var(--text-muted)", marginBottom: "4px" }}>
            * 비즈니스 QA 규칙에 의거하여, 검증되지 않은 무리한 팩트나 허위 지표는 AI에 의해 필터링됩니다. 출처와 함께 적어주세요.
          </p>
          <textarea
            placeholder="예: 
1. 2025년 매출 50억 돌파 (자사 재무제표 기준)
2. 고객사 이탈률 1.2% 미만 유지 (CRM 로그 기준)"
            value={claims}
            onChange={(e) => setClaims(e.target.value)}
            rows={5}
            required
          />
        </div>

        <div className="action-row" style={{ justifyContent: "flex-end", marginTop: "10px" }}>
          {isSaved && (
            <span style={{ display: "flex", alignItems: "center", color: "var(--accent-success)", fontSize: "13px", fontWeight: "600" }}>
              ✓ 모든 Q&A 정보가 brief.md에 정상 기록되었습니다!
            </span>
          )}
          <button type="submit" className="btn btn-primary">
            Brief.md 저장 및 AI 파이프라인 동기화
          </button>
        </div>
      </form>
    </div>
  );
}
