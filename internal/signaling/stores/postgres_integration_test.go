//go:build integration

package stores_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/poki/netlib/internal/signaling/stores"
	"github.com/poki/netlib/internal/testutil"
)

const testGame = "00000000-0000-4000-8000-000000000001"

func createPeer(t *testing.T, ctx context.Context, s *stores.PostgresStore, id string) {
	t.Helper()
	if err := s.CreatePeer(ctx, id, "test-secret", testGame); err != nil {
		t.Fatal(err)
	}
}

func createLobby(t *testing.T, ctx context.Context, s *stores.PostgresStore, code, host string, options stores.LobbyOptions) {
	t.Helper()
	// HandleCreatePacket задаёт эти значения перед вызовом store.
	if options.Public == nil {
		value := false
		options.Public = &value
	}
	if options.CanUpdateBy == nil {
		value := stores.CanUpdateByCreator
		options.CanUpdateBy = &value
	}
	if options.MaxPlayers == nil {
		value := 4
		options.MaxPlayers = &value
	}
	if err := s.CreateLobby(ctx, testGame, code, host, options); err != nil {
		t.Fatal(err)
	}
}

func TestJoinLobbyReturnsConcurrentSnapshots(t *testing.T) {
	ctx, s := testutil.Postgres(t)
	for _, id := range []string{"host", "first", "second", "third"} {
		createPeer(t, ctx, s, id)
	}
	createLobby(t, ctx, s, "room", "host", stores.LobbyOptions{})
	type result struct {
		id    string
		peers []string
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 3)
	for _, id := range []string{"first", "second", "third"} {
		go func(id string) {
			<-start
			peers, err := s.JoinLobby(ctx, testGame, "room", id, "")
			results <- result{id, peers, err}
		}(id)
	}
	close(start)
	snapshots := make(map[string][]string)
	for range 3 {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if slices.Contains(r.peers, r.id) || !slices.Contains(r.peers, "host") {
			t.Fatalf("снимок %s: %v", r.id, r.peers)
		}
		snapshots[r.id] = r.peers
	}
	// Для каждой пары только более поздний вход должен инициировать соединение.
	for a, beforeA := range snapshots {
		for b, beforeB := range snapshots {
			if a != b && slices.Contains(beforeA, b) == slices.Contains(beforeB, a) {
				t.Fatalf("неоднозначный порядок входа %s/%s: %v / %v", a, b, beforeA, beforeB)
			}
		}
	}
	lobby, err := s.GetLobby(ctx, testGame, "room")
	if err != nil || len(lobby.Peers) != 4 {
		t.Fatalf("состав: %+v, %v", lobby, err)
	}
}

func TestLobbyValidationAndLifecycle(t *testing.T) {
	ctx, s := testutil.Postgres(t)
	for _, id := range []string{"host", "guest", "extra"} {
		createPeer(t, ctx, s, id)
	}
	password, permission, public, limit := "secret", stores.CanUpdateByCreator, true, 2
	data := map[string]any{"status": "waiting"}
	createLobby(t, ctx, s, "room", "host", stores.LobbyOptions{Password: &password, CanUpdateBy: &permission, Public: &public, MaxPlayers: &limit, CustomData: &data})
	for _, tc := range []struct {
		name, lobby, peer, password string
		want                        error
	}{
		{"password", "room", "guest", "wrong", stores.ErrInvalidPassword},
		{"missing", "missing", "guest", "secret", stores.ErrNotFound},
		{"long-peer", "room", strings.Repeat("x", 21), "secret", stores.ErrInvalidPeerID},
		{"duplicate", "room", "host", "secret", stores.ErrAlreadyInLobby},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := s.JoinLobby(ctx, testGame, tc.lobby, tc.peer, tc.password)
			if !errors.Is(err, tc.want) || before != nil {
				t.Fatalf("результат %v, %v; ожидалось %v", before, err, tc.want)
			}
			lobby, err := s.GetLobby(ctx, testGame, "room")
			if err != nil || !slices.Equal(lobby.Peers, []string{"host"}) {
				t.Fatalf("отказ изменил состав: %+v %v", lobby, err)
			}
		})
	}
	if _, err := s.JoinLobby(ctx, testGame, "room", "guest", password); err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinLobby(ctx, testGame, "room", "extra", password); !errors.Is(err, stores.ErrLobbyIsFull) {
		t.Fatalf("заполненная комната: %v", err)
	}
	updated := map[string]any{"status": "playing"}
	if err := s.UpdateLobby(ctx, testGame, "room", "guest", stores.LobbyOptions{CustomData: &updated}); err == nil {
		t.Fatal("гость изменил настройки хозяина")
	}
	if err := s.UpdateLobby(ctx, testGame, "room", "host", stores.LobbyOptions{CustomData: &updated}); err != nil {
		t.Fatal(err)
	}
	lobbies, err := s.ListLobbies(ctx, testGame, nil, nil, nil, "", "", 10)
	if err != nil || len(lobbies) != 1 || lobbies[0].CustomData["status"] != "playing" || lobbies[0].PlayerCount != 2 {
		t.Fatalf("список: %+v %v", lobbies, err)
	}
	if err := s.LeaveLobby(ctx, testGame, "room", "guest"); err != nil {
		t.Fatal(err)
	}
	before, err := s.JoinLobby(ctx, testGame, "room", "guest", password)
	if err != nil || !slices.Equal(before, []string{"host"}) {
		t.Fatalf("повторный вход: %v %v", before, err)
	}
}

func TestLeaderElectionUsesExistingConnectedPeers(t *testing.T) {
	for _, state := range []string{"connected", "disconnected", "deleted", "no-candidates", "foreign-game"} {
		t.Run(state, func(t *testing.T) {
			ctx, s := testutil.Postgres(t)
			createPeer(t, ctx, s, "host")
			createPeer(t, ctx, s, "guest")
			createLobby(t, ctx, s, "room", "host", stores.LobbyOptions{})
			if _, err := s.JoinLobby(ctx, testGame, "room", "guest", ""); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "disconnected", "no-candidates", "foreign-game":
				if err := s.MarkPeerAsDisconnected(ctx, "host"); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if _, err := s.DB.Exec(ctx, "DELETE FROM peers WHERE peer = 'host'"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "no-candidates" {
				if err := s.MarkPeerAsDisconnected(ctx, "guest"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "foreign-game" {
				if _, err := s.DB.Exec(ctx, "UPDATE peers SET game = '00000000-0000-4000-8000-000000000002' WHERE peer = 'guest'"); err != nil {
					t.Fatal(err)
				}
			}
			result, err := s.DoLeaderElection(ctx, testGame, "room")
			if err != nil {
				t.Fatal(err)
			}
			if state == "connected" {
				if result != nil {
					t.Fatalf("действующий лидер заменён: %+v", result)
				}
				return
			}
			want := "guest"
			if state == "no-candidates" || state == "foreign-game" {
				want = ""
			}
			if result == nil || result.Leader != want || result.Term != 2 {
				t.Fatalf("выбор: %+v, ожидался %q/term=2", result, want)
			}
			lobby, err := s.GetLobby(ctx, testGame, "room")
			if err != nil || lobby.Leader != want || lobby.Term != 2 {
				t.Fatalf("сохранённый лидер: %+v %v", lobby, err)
			}
		})
	}
}

func TestLeaderElectionDoesNotBlockAnotherLobby(t *testing.T) {
	ctx, s := testutil.Postgres(t)
	for _, id := range []string{"host", "other", "newcomer"} {
		createPeer(t, ctx, s, id)
	}
	createLobby(t, ctx, s, "locked", "host", stores.LobbyOptions{})
	createLobby(t, ctx, s, "other-room", "other", stores.LobbyOptions{})
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT 1 FROM lobbies WHERE code = 'locked' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	electionCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	done := make(chan error, 1)
	go func() { _, err := s.DoLeaderElection(electionCtx, testGame, "locked"); done <- err }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting int
		err := s.DB.QueryRow(electionCtx, "SELECT count(*) FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND wait_event_type = 'Lock' AND query LIKE '%SELECT leader, term, peers%'").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("выбор не дошёл до блокировки комнаты")
		}
		time.Sleep(10 * time.Millisecond)
	}
	otherCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := s.MarkPeerAsActive(otherCtx, "other"); err != nil {
		t.Fatalf("выбор в одной комнате блокирует другую: %v", err)
	}
	if _, err := s.JoinLobby(otherCtx, testGame, "other-room", "newcomer", ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutElectionAndJoinCanCompleteTogether(t *testing.T) {
	ctx, s := testutil.Postgres(t)
	ctx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	for round := range 5 {
		host, guest, next, room := fmt.Sprintf("old%d", round), fmt.Sprintf("live%d", round), fmt.Sprintf("new%d", round), fmt.Sprintf("room%d", round)
		for _, id := range []string{host, guest, next} {
			createPeer(t, ctx, s, id)
		}
		createLobby(t, ctx, s, room, host, stores.LobbyOptions{})
		if _, err := s.JoinLobby(ctx, testGame, room, guest, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(ctx, "UPDATE peers SET last_seen = CURRENT_TIMESTAMP - interval '10 minutes' WHERE peer = $1", host); err != nil {
			t.Fatal(err)
		}
		start, results := make(chan struct{}), make(chan error, 3)
		go func() { <-start; _, _, _, err := s.ClaimNextTimedOutPeer(ctx, time.Minute); results <- err }()
		go func() { <-start; _, err := s.DoLeaderElection(ctx, testGame, room); results <- err }()
		go func() { <-start; _, err := s.JoinLobby(ctx, testGame, room, next, ""); results <- err }()
		close(start)
		for range 3 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.DoLeaderElection(ctx, testGame, room); err != nil {
			t.Fatal(err)
		}
		lobby, err := s.GetLobby(ctx, testGame, room)
		if err != nil || slices.Contains(lobby.Peers, host) || len(lobby.Peers) != 2 || (lobby.Leader != guest && lobby.Leader != next) {
			t.Fatalf("итог конкуренции: %+v %v", lobby, err)
		}
	}
}
