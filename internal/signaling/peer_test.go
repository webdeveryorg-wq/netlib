package signaling

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/poki/netlib/internal/signaling/stores"
)

type snapshotStore struct {
	stores.Store
	recipients []string
}

func (*snapshotStore) JoinLobby(context.Context, string, string, string, string) ([]string, error) {
	return []string{"host"}, nil
}
func (*snapshotStore) Subscribe(context.Context, stores.SubscriptionCallback, string, string, string) {
}
func (*snapshotStore) DoLeaderElection(context.Context, string, string) (*stores.ElectionResult, error) {
	return nil, nil
}
func (*snapshotStore) GetLobby(context.Context, string, string) (stores.Lobby, error) {
	// Следующий участник уже вошёл между JoinLobby и GetLobby.
	return stores.Lobby{Code: "room", Peers: []string{"host", "joining", "late"}}, nil
}
func (s *snapshotStore) Publish(_ context.Context, topic string, data []byte) error {
	var packet ConnectPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return err
	}
	if packet.Type == "connect" {
		s.recipients = append(s.recipients, topic)
	}
	return nil
}

func TestJoinRequestsOnlyPreviousPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store := &snapshotStore{}
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		peer := &Peer{store: store, conn: conn, ID: "joining", Game: "game"}
		done <- peer.HandleJoinPacket(ctx, JoinPacket{Lobby: "room", RequestID: "join"})
	}))
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var connections []string
	joined := false
	for {
		var packet map[string]any
		if err := wsjson.Read(ctx, conn, &packet); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				t.Fatal(err)
			}
			break
		}
		if packet["type"] == "joined" {
			joined = packet["rid"] == "join"
		}
		if packet["type"] == "connect" {
			connections = append(connections, packet["id"].(string))
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !joined || !reflect.DeepEqual(connections, []string{"host"}) || !reflect.DeepEqual(store.recipients, []string{"gameroomhost"}) {
		t.Fatalf("joined=%v connections=%v recipients=%v", joined, connections, store.recipients)
	}
}
