import { useState } from "react";
import type { ReactElement } from "react";

export interface Deck {
  id: string;
  name: string;
  path: string;
  status: "ready" | "intake_required" | "error";
  slidesCount: number;
  lastUpdated: string;
  description: string;
}

interface DeckManagerProps {
  decks: Deck[];
  activeDeck: Deck | null;
  onSelectDeck: (deck: Deck) => void;
  onCreateDeck: (name: string, description: string) => void;
}

export function DeckManager({
  decks,
  activeDeck,
  onSelectDeck,
  onCreateDeck
}: DeckManagerProps): ReactElement {
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newDeckName, setNewDeckName] = useState("");
  const [newDeckDesc, setNewDeckDesc] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newDeckName.trim()) return;
    
    // Convert to safe deck ID (e.g. "My Deck" -> "my-deck")
    onCreateDeck(newDeckName, newDeckDesc);
    setNewDeckName("");
    setNewDeckDesc("");
    setShowCreateModal(false);
  };

  const getStatusBadgeClass = (status: Deck["status"]) => {
    switch (status) {
      case "ready":
        return "badge-success";
      case "intake_required":
        return "badge-warning";
      case "error":
        return "badge-error";
      default:
        return "badge-info";
    }
  };

  const getStatusText = (status: Deck["status"]) => {
    switch (status) {
      case "ready":
        return "준비됨";
      case "intake_required":
        return "Q&A 입력 대기";
      case "error":
        return "오류 감지";
      default:
        return "대기";
    }
  };

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "20px" }}>
        <p style={{ color: "var(--text-secondary)" }}>
          현재 구성된 슬라이덱스(Slidex) 작업공간 목록입니다. 활성화할 덱을 선택하거나 새 덱을 만드세요.
        </p>
      </div>

      <div className="deck-grid">
        {decks.map((deck) => (
          <div
            key={deck.id}
            className={`card deck-card ${activeDeck?.id === deck.id ? "active" : ""}`}
            onClick={() => onSelectDeck(deck)}
            style={{ display: "flex", flexDirection: "column", justifyContent: "space-between", minHeight: "200px" }}
          >
            <div>
              <span className={`badge ${getStatusBadgeClass(deck.status)}`}>
                {getStatusText(deck.status)}
              </span>
              <h3 style={{ marginTop: "10px", fontSize: "18px" }}>{deck.name}</h3>
              <p style={{ fontSize: "13px", color: "var(--text-secondary)", marginTop: "8px", height: "40px", overflow: "hidden", textOverflow: "ellipsis" }}>
                {deck.description || "설명이 없습니다."}
              </p>
            </div>
            
            <div className="deck-meta">
              <span>슬라이드: {deck.slidesCount}개</span>
              <span>{deck.lastUpdated}</span>
            </div>
          </div>
        ))}

        <div
          className="card create-deck-card"
          onClick={() => setShowCreateModal(true)}
          style={{ cursor: "pointer", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}
        >
          <span style={{ fontSize: "32px", color: "var(--accent-gold)" }}>+</span>
          <strong>새 슬라이드 덱 추가</strong>
          <span style={{ fontSize: "12px", color: "var(--text-muted)" }}>_template 기준 생성</span>
        </div>
      </div>

      {showCreateModal && (
        <div style={{
          position: "fixed",
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: "rgba(0,0,0,0.6)",
          backdropFilter: "blur(4px)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          zIndex: 1000
        }}>
          <div className="card" style={{ width: "450px", background: "var(--bg-secondary)", padding: "28px" }}>
            <h2 style={{ marginBottom: "20px" }}>새로운 슬라이드 덱 생성</h2>
            <form onSubmit={handleSubmit}>
              <div className="form-group">
                <label>덱 이름 (영문/숫자/하이픈 권장)</label>
                <input
                  type="text"
                  placeholder="e.g. business-proposal"
                  value={newDeckName}
                  onChange={(e) => setNewDeckName(e.target.value)}
                  required
                  autoFocus
                />
              </div>
              <div className="form-group">
                <label>설명 (간략한 개요)</label>
                <textarea
                  placeholder="어떤 문서를 다루는지 설명해주세요."
                  value={newDeckDesc}
                  onChange={(e) => setNewDeckDesc(e.target.value)}
                  rows={3}
                />
              </div>
              <div className="action-row" style={{ justifyContent: "flex-end", marginTop: "24px" }}>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setShowCreateModal(false)}
                >
                  취소
                </button>
                <button type="submit" className="btn btn-primary">
                  생성하기
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
