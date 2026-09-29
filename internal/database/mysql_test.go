package database

import (
	"database/sql"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestConfigurePool(t *testing.T) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = "127.0.0.1:1" // never dialed: no Ping is issued
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatalf("new connector: %v", err)
	}

	for _, k := range []int{1, 4, 50} {
		db := sql.OpenDB(connector)
		configurePool(db, k)
		if got := db.Stats().MaxOpenConnections; got < k+1 {
			t.Errorf("workers=%d: MaxOpenConnections=%d, want >= %d", k, got, k+1)
		}
		_ = db.Close()
	}
}
