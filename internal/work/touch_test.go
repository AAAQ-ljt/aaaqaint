package work

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/wahhheee/aint/internal/store"
)

// Touch 把作品的 updated_at 推到当前时间，书架按它倒序排列。
func TestTouchMovesWorkToFront(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct {
		id string
		at int64
	}{{"older", 1_000}, {"newer", 2_000}} {
		err := db.Tx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO works (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)", w.id, w.id, w.at, w.at)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	rows, err := listWorks(ctx, db.Read())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "newer" {
		t.Fatalf("初始顺序应为 newer 在前：%+v", rows)
	}

	s := NewService(db, store.NewBlobs(filepath.Join(dir, "blobs")))
	if err := s.Touch(ctx, "older"); err != nil {
		t.Fatal(err)
	}

	rows, err = listWorks(ctx, db.Read())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "older" {
		t.Fatalf("Touch 后应把 older 排到最前：%+v", rows)
	}
}
