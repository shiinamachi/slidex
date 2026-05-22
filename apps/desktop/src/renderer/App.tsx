import { useEffect, useState } from "react";
import type { ReactElement } from "react";
import type { DesktopAppInfo, SlidexStatus } from "../shared/api";

// Import Components
import { DeckManager, type Deck } from "./components/DeckManager";
import { PipelineMonitor } from "./components/PipelineMonitor";
import { IntakePanel } from "./components/IntakePanel";
import { SlideViewer } from "./components/SlideViewer";
import { SystemDoctor } from "./components/SystemDoctor";

type Tab = "작업공간" | "빌드 파이프라인" | "Q&A Intake" | "슬라이드 미리보기" | "시스템 진단";

const INITIAL_DECKS: Deck[] = [
  {
    id: "spikalabs",
    name: "SpikaLabs IR 자료",
    path: "decks/spikalabs",
    status: "intake_required",
    slidesCount: 9,
    lastUpdated: "2026-05-22",
    description: "SpikaLabs 시리즈 A 유치를 위한 기술 및 비즈니스 실적 IR 슬라이드 덱"
  },
  {
    id: "cli-test-virtual-company",
    name: "가상 기업 기술 로드맵",
    path: "decks/cli-test-virtual-company",
    status: "ready",
    slidesCount: 6,
    lastUpdated: "2026-05-23",
    description: "정부 기술 과제 제출용 마일스톤 및 에이전틱 개발 아키텍처 로드맵 문서"
  }
];

export function App(): ReactElement {
  const [appInfo, setAppInfo] = useState<DesktopAppInfo | null>(null);
  const [status, setStatus] = useState<SlidexStatus | null>(null);
  
  // App States
  const [activeTab, setActiveTab] = useState<Tab>("작업공간");
  const [decks, setDecks] = useState<Deck[]>(INITIAL_DECKS);
  const [activeDeck, setActiveDeck] = useState<Deck | null>(INITIAL_DECKS[0]);

  useEffect(() => {
    void window.slidex.getAppInfo().then(setAppInfo);
    void window.slidex.getSlidexStatus().then(setStatus);
  }, []);

  const handleSelectDeck = (deck: Deck) => {
    setActiveDeck(deck);
  };

  const handleCreateDeck = (name: string, description: string) => {
    const id = name.toLowerCase().replace(/\s+/g, "-");
    const newDeck: Deck = {
      id,
      name,
      path: `decks/${id}`,
      status: "intake_required",
      slidesCount: 5,
      lastUpdated: new Date().toISOString().split("T")[0],
      description
    };
    setDecks((prev) => [...prev, newDeck]);
    setActiveDeck(newDeck);
  };

  const handleSaveIntake = (deckId: string) => {
    setDecks((prev) =>
      prev.map((deck) =>
        deck.id === deckId ? { ...deck, status: "ready" } : deck
      )
    );
    if (activeDeck && activeDeck.id === deckId) {
      setActiveDeck((prev) => (prev ? { ...prev, status: "ready" } : null));
    }
  };

  const handleIntakeRequired = () => {
    // If CLI process returns exit code 3 (intake needed), redirect to intake tab
    setActiveTab("Q&A Intake");
  };

  const handleBuildComplete = () => {
    // If build pipeline succeeds, redirect to slide viewer tab
    setActiveTab("슬라이드 미리보기");
  };

  const renderActiveContent = () => {
    switch (activeTab) {
      case "작업공간":
        return (
          <DeckManager
            decks={decks}
            activeDeck={activeDeck}
            onSelectDeck={handleSelectDeck}
            onCreateDeck={handleCreateDeck}
          />
        );
      case "빌드 파이프라인":
        return (
          <PipelineMonitor
            activeDeck={activeDeck}
            onIntakeRequired={handleIntakeRequired}
            onBuildComplete={handleBuildComplete}
          />
        );
      case "Q&A Intake":
        return (
          <IntakePanel
            activeDeck={activeDeck}
            onSaveIntake={handleSaveIntake}
          />
        );
      case "슬라이드 미리보기":
        return <SlideViewer activeDeck={activeDeck} />;
      case "시스템 진단":
        return <SystemDoctor />;
      default:
        return <div>화면을 찾을 수 없습니다.</div>;
    }
  };

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
          {(["작업공간", "빌드 파이프라인", "Q&A Intake", "슬라이드 미리보기", "시스템 진단"] as Tab[]).map((item) => (
            <button
              className={activeTab === item ? "active" : ""}
              key={item}
              type="button"
              onClick={() => setActiveTab(item)}
            >
              {item === "작업공간" && "📁 "}
              {item === "빌드 파이프라인" && "⚡ "}
              {item === "Q&A Intake" && "📝 "}
              {item === "슬라이드 미리보기" && "🖼️ "}
              {item === "시스템 진단" && "🩺 "}
              {item}
            </button>
          ))}
        </nav>
      </aside>

      <main className="workspace">
        <header className="workspace-header">
          <div>
            <p className="eyebrow">슬라이덱스 로컬 자동화 쉘</p>
            <h1>{activeTab}</h1>
          </div>
          <div style={{ display: "flex", gap: "10px", alignItems: "center" }}>
            {activeDeck && (
              <span className="version-chip" style={{ color: "var(--accent-gold)", borderColor: "rgba(212,175,55,0.4)" }}>
                Active: <strong>{activeDeck.name}</strong>
              </span>
            )}
            <span className="version-chip">v{appInfo?.version ?? "0.1.0"}</span>
          </div>
        </header>

        {activeTab === "작업공간" && (
          <section className="status-grid" aria-label="현재 상태">
            <div className="status-card">
              <span>App Mode</span>
              <strong>{appInfo?.mode === "production" ? "배포" : "개발"}</strong>
            </div>
            <div className="status-card">
              <span>CLI Bridge</span>
              <strong>{status?.ready ? "준비됨" : "준비됨 (시뮬레이션)"}</strong>
            </div>
            <div className="status-card">
              <span>구성된 덱 개수</span>
              <strong>{decks.length}개</strong>
            </div>
          </section>
        )}

        <section style={{ flex: 1 }}>
          {renderActiveContent()}
        </section>
      </main>
    </div>
  );
}
