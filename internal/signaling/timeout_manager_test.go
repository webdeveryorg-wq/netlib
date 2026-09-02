package signaling

import (
	"context"
	"errors"
	"testing"

	"github.com/poki/netlib/internal/signaling/stores"
)

type disconnectStore struct {
	stores.Store
	marked, elections int
	markError         error
}

func (s *disconnectStore) MarkPeerAsDisconnected(context.Context, string) error {
	s.marked++
	return s.markError
}
func (s *disconnectStore) DoLeaderElection(context.Context, string, string) (*stores.ElectionResult, error) {
	s.elections++
	return nil, nil
}

func TestDisconnectedElectsOnlyForKnownLobby(t *testing.T) {
	for _, tc := range []struct {
		name, id, lobby   string
		failed            bool
		marked, elections int
	}{
		{"unintroduced", "", "", false, 0, 0},
		{"before-join", "peer", "", false, 1, 0},
		{"in-lobby", "peer", "room", false, 1, 1},
		{"mark-failed", "peer", "room", true, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &disconnectStore{}
			if tc.failed {
				s.markError = errors.New("test failure")
			}
			manager := &TimeoutManager{Store: s}
			manager.Disconnected(context.Background(), &Peer{ID: tc.id, Game: "game", Lobby: tc.lobby})
			if s.marked != tc.marked || s.elections != tc.elections {
				t.Fatalf("marked/elections = %d/%d; ожидалось %d/%d", s.marked, s.elections, tc.marked, tc.elections)
			}
		})
	}
}
