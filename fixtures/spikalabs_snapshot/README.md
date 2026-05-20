# spikalabs Snapshot Fixture

이 fixture는 실제 `decks/spikalabs` 작업공간을 직접 복사하지 않고, 회귀 테스트가
필요로 하는 snapshot 존재 여부를 고정하기 위한 자리표시자입니다. 실제 deck 자료는
사용자 작업공간에 남기고, 테스트는 `fixtures/minimal_deck`을 임시 디렉터리로 복사해
deterministic render/QA/package 흐름을 검증합니다.

