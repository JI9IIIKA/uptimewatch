package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	p "uptimewatch/internal/platform"
	"uptimewatch/migrations"
	"uptimewatch/projects/uptimewatch"
)

var sharedDB *pgxpool.Pool
var sharedErr error
var dbOnce sync.Once

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedDB != nil {
		sharedDB.Close()
	}
	os.Exit(code)
}

type fixture struct {
	t      *testing.T
	app    *p.App
	db     *pgxpool.Pool
	tenant string
}

func setup(t *testing.T, name string, register func(*p.App)) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set: integration test requires PostgreSQL")
	}
	dbOnce.Do(func() { sharedDB, sharedErr = pgxpool.New(context.Background(), url) })
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	db := sharedDB
	var e error
	if e = migrations.Apply(context.Background(), db, name); e != nil {
		t.Fatal(e)
	}
	tenant := p.ID()
	tables := map[string][]string{
		"teamflow": {"task_dependencies", "task_history", "tasks"}, "bookslot": {"bookings", "resources"},
		"jobrunner": {"jobs"}, "notifyhub": {"delivery_attempts", "notifications"},
		"ordercore": {"payment_events", "order_items", "orders", "products"}, "eventscope": {"analytics_events"},
		"flagcontrol": {"flag_heads", "flags"}, "uptimewatch": {"checks", "incidents", "monitors"},
		"knowledgedesk": {"document_access", "chunks", "documents"}, "supportflow": {"ticket_audit", "support_replies", "tickets"},
	}
	t.Cleanup(func() {
		for _, table := range append(tables[name], "outbox", "idempotency", "event_receipts") {
			if _, err := db.Exec(context.Background(), "DELETE FROM "+table+" WHERE tenant=$1", tenant); err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
	})

	a := p.New(name, db, map[string]p.Principal{"admin-token-123456": {Tenant: tenant, User: "alice", Role: "admin"}, "editor-token-12345": {Tenant: tenant, User: "bob", Role: "editor"}, "viewer-token-12345": {Tenant: tenant, User: "reader", Role: "viewer"}, "other-token-123456": {Tenant: p.ID(), User: "mallory", Role: "admin"}})
	register(a)
	return &fixture{t, a, db, tenant}
}
func (f *fixture) call(method, path, token, key string, body any) (int, map[string]any) {
	f.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.app.Router.ServeHTTP(w, req)
	var result map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result
}
func (f *fixture) ok(method, path string, body any) map[string]any {
	f.t.Helper()
	code, out := f.call(method, path, "admin-token-123456", "", body)
	if code != 200 {
		f.t.Fatalf("%s %s: %d %v", method, path, code, out)
	}
	return out
}
func (f *fixture) sql(q string, args ...any) {
	f.t.Helper()
	if _, e := f.db.Exec(context.Background(), q, args...); e != nil {
		f.t.Fatal(e)
	}
}
func (f *fixture) count(q string, args ...any) int {
	f.t.Helper()
	var n int
	if e := f.db.QueryRow(context.Background(), q, args...).Scan(&n); e != nil {
		f.t.Fatal(e)
	}
	return n
}
func require(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}
func TestUptimeWatch(t *testing.T) {
	f := setup(t, "uptimewatch", uptimewatch.Register)
	out := f.ok("POST", "/monitors", map[string]any{"url": "https://example.com", "threshold": 2})
	id := out["id"].(string)
	record := func(ok bool) {
		tx, e := f.db.Begin(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(context.Background())
		if e = uptimewatch.Record(context.Background(), tx, f.tenant, id, ok, 10, 500); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	record(false)
	record(false)
	record(false)
	require(t, f.count(`SELECT count(*) FROM incidents WHERE tenant=$1 AND closed_at IS NULL`, f.tenant) == 1, "duplicate incident")
	record(true)
	require(t, f.count(`SELECT count(*) FROM incidents WHERE tenant=$1 AND closed_at IS NULL`, f.tenant) == 0, "incident not resolved")
}
