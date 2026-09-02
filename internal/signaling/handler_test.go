package signaling

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/poki/netlib/internal/cloudflare"
	"github.com/poki/netlib/internal/signaling/stores"
)

type idleStore struct{ stores.Store }

func (*idleStore) ResetAllPeerLastSeen(context.Context) error { return nil }
func (*idleStore) ClaimNextTimedOutPeer(context.Context, time.Duration) (string, bool, map[string][]string, error) {
	return "", false, nil, nil
}

func TestRequestContextDoesNotLeakIntoNextPacket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Кэш credentials намеренно пуст: проверяется rid настоящего ответа с ошибкой.
	wg, handler := Handler(ctx, &idleStore{}, cloudflare.NewCredentialsClient("", "", time.Minute))
	server := httptest.NewServer(handler)
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { conn.CloseNow(); cancel(); wg.Wait() }()
	for _, rid := range []string{"first", "", "second", strings.Repeat("x", 65), "third", ""} {
		request := map[string]string{"type": "credentials"}
		if rid != "" {
			request["rid"] = rid
		}
		if err := wsjson.Write(ctx, conn, request); err != nil {
			t.Fatal(err)
		}
		var reply map[string]any
		if err := wsjson.Read(ctx, conn, &reply); err != nil {
			t.Fatal(err)
		}
		want := rid
		if len(want) > 64 {
			want = ""
		}
		got, _ := reply["rid"].(string)
		if reply["type"] != "error" || got != want {
			t.Fatalf("ответ rid=%q: %v; ожидался rid=%q", rid, reply, want)
		}
	}
}

func TestRequestContextsWhilePingRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wg, handler := Handler(ctx, &idleStore{}, cloudflare.NewCredentialsClient("", "", time.Minute))
	server := httptest.NewServer(handler)
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { conn.CloseNow(); cancel(); wg.Wait() }()
	until := time.Now().Add(peerPingDuration + 300*time.Millisecond)
	pings := 0
	for i := 0; time.Now().Before(until); i++ {
		rid := fmt.Sprintf("request-%d", i)
		if err := wsjson.Write(ctx, conn, map[string]string{"type": "credentials", "rid": rid}); err != nil {
			t.Fatal(err)
		}
		for {
			var reply map[string]any
			if err := wsjson.Read(ctx, conn, &reply); err != nil {
				t.Fatal(err)
			}
			if reply["type"] == "ping" {
				pings++
				continue
			}
			if reply["rid"] != rid {
				t.Fatalf("смешаны запросы: %v, ожидался %s", reply, rid)
			}
			break
		}
	}
	if pings == 0 {
		t.Fatal("не проверена конкурентная работа ping")
	}
}
