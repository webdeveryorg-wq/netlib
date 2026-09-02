//go:build integration

package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/koenbollen/logging"
	"github.com/poki/netlib/internal/signaling/stores"
	"go.uber.org/zap"
)

// Postgres создаёт отдельную БД для каждого теста и использует настоящие миграции.
func Postgres(t *testing.T) (context.Context, *stores.PostgresStore) {
	t.Helper()
	address, err := url.Parse(os.Getenv("NETLIB_TEST_DATABASE_URL"))
	if err != nil || address == nil || address.Host == "" || address.Path != "/netlib_test" {
		t.Fatal("для integration нужен NETLIB_TEST_DATABASE_URL с отдельной базой /netlib_test")
	}
	ctx, cancel := context.WithCancel(logging.WithLogger(context.Background(), zap.NewNop()))
	admin, err := pgx.Connect(ctx, address.String())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	name := fmt.Sprintf("netlib_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		admin.Close(ctx)
		cancel()
		t.Fatal(err)
	}
	var store *stores.PostgresStore
	t.Cleanup(func() {
		cancel()
		if store != nil {
			store.DB.Close()
		}
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("удаление тестовой БД: %v", err)
		}
		admin.Close(cleanup)
	})
	address.Path = "/" + name
	t.Setenv("DATABASE_URL", address.String())
	loaded, _, err := stores.FromEnv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store = loaded.(*stores.PostgresStore)
	return ctx, store
}
