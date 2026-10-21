package postgres_test

import (
	"context"
	"testing"

	"github.com/fiston-user/murmur/internal/platform/testdb"
)

// Every connection of a pool that Connect opened asks Postgres for one plan per statement.
func TestConnectAsksForOnePlan(t *testing.T) {
	db := testdb.New(t) // testdb opens its pool with Connect
	var mode string
	if err := db.QueryRow(context.Background(), `show plan_cache_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "force_generic_plan" {
		t.Errorf("plan_cache_mode is %q, want force_generic_plan", mode)
	}
}
