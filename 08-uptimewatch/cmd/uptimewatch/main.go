package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"uptimewatch/internal/platform"
	"uptimewatch/migrations"
	"uptimewatch/projects/uptimewatch"
)

func main() {
	platform.Serve("uptimewatch", uptimewatch.Register, func(ctx context.Context, pool *pgxpool.Pool) error { return migrations.Apply(ctx, pool, "uptimewatch") })
}
