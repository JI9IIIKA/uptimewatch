package uptimewatch

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"time"
	p "uptimewatch/internal/platform"
)

func Register(a *p.App) {
	a.Handle("POST", "/monitors", func(r *p.Request) (any, error) {
		var b struct {
			URL       string `json:"url"`
			Interval  int    `json:"interval_seconds"`
			Threshold int    `json:"threshold"`
		}
		if e := r.Decode(&b); e != nil {
			return nil, e
		}
		if e := p.ValidateURL(b.URL); e != nil {
			return nil, e
		}
		if b.Interval == 0 {
			b.Interval = 60
		}
		if b.Threshold == 0 {
			b.Threshold = 3
		}
		return r.One(`INSERT INTO monitors(tenant,id,url,interval_seconds,threshold) VALUES($1,$2,$3,$4,$5) RETURNING *`, r.Tenant, p.ID(), b.URL, b.Interval, b.Threshold)
	})
	a.Handle("GET", "/monitors", func(r *p.Request) (any, error) {
		return r.All(`SELECT * FROM monitors WHERE tenant=$1 ORDER BY id LIMIT 100`, r.Tenant)
	})
	a.Handle("PATCH", "/monitors/{id}", func(r *p.Request) (any, error) {
		var b struct {
			Paused bool `json:"paused"`
		}
		if e := r.Decode(&b); e != nil {
			return nil, e
		}
		return r.One(`UPDATE monitors SET paused=$3 WHERE tenant=$1 AND id=$2 RETURNING *`, r.Tenant, r.Param("id"), b.Paused)
	})
	a.Handle("GET", "/monitors/{id}/checks", func(r *p.Request) (any, error) {
		return r.All(`SELECT * FROM checks WHERE tenant=$1 AND monitor_id=$2 ORDER BY created_at DESC LIMIT 100`, r.Tenant, r.Param("id"))
	})
	a.Handle("GET", "/incidents", func(r *p.Request) (any, error) {
		return r.All(`SELECT * FROM incidents WHERE tenant=$1 ORDER BY opened_at DESC LIMIT 100`, r.Tenant)
	})
	a.Handle("GET", "/status", func(r *p.Request) (any, error) {
		return r.All(`SELECT m.id,m.url,m.paused,count(c.id) AS checks,count(c.id) FILTER(WHERE c.ok) AS successful,COALESCE(round(100.0*count(c.id) FILTER(WHERE c.ok)/NULLIF(count(c.id),0),2),0) AS availability_percent FROM monitors m LEFT JOIN checks c ON c.tenant=m.tenant AND c.monitor_id=m.id AND c.created_at>now()-interval '24 hours' WHERE m.tenant=$1 GROUP BY m.id,m.url,m.paused ORDER BY m.id`, r.Tenant)
	})
	a.Workers = append(a.Workers, Check)
}
func Record(ctx context.Context, tx pgx.Tx, tenant, id string, ok bool, latency int64, status int) error {
	if _, e := tx.Exec(ctx, `INSERT INTO checks(tenant,monitor_id,ok,latency_ms,status_code) VALUES($1,$2,$3,$4,$5)`, tenant, id, ok, latency, status); e != nil {
		return e
	}
	var failures, threshold int
	if e := tx.QueryRow(ctx, `UPDATE monitors SET failures=CASE WHEN $3 THEN 0 ELSE failures+1 END,next_at=now()+make_interval(secs=>interval_seconds) WHERE tenant=$1 AND id=$2 RETURNING failures,threshold`, tenant, id, ok).Scan(&failures, &threshold); e != nil {
		return e
	}
	if ok {
		_, e := tx.Exec(ctx, `UPDATE incidents SET closed_at=now() WHERE tenant=$1 AND monitor_id=$2 AND closed_at IS NULL`, tenant, id)
		return e
	}
	if failures >= threshold {
		_, e := tx.Exec(ctx, `INSERT INTO incidents(id,tenant,monitor_id) VALUES($1,$2,$3) ON CONFLICT(tenant,monitor_id) WHERE closed_at IS NULL DO NOTHING`, p.ID(), tenant, id)
		return e
	}
	return nil
}
func Check(ctx context.Context, db *pgxpool.Pool) error {
	ctx, c := context.WithTimeout(ctx, 8*time.Second)
	defer c()
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var tenant, id, url string
	e = tx.QueryRow(ctx, `SELECT tenant,id,url FROM monitors WHERE next_at<=now() AND NOT paused ORDER BY next_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&tenant, &id, &url)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	start := time.Now()
	status := 0
	if p.ValidateURL(url) == nil {
		req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
		if e != nil {
			return e
		}
		resp, e := p.SafeClient().Do(req)
		if e == nil {
			status = resp.StatusCode
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
		}
	}
	if e = Record(ctx, tx, tenant, id, status >= 200 && status < 300, time.Since(start).Milliseconds(), status); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
