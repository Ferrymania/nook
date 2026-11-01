package database

import (
	"context"
	"testing"

	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func TestMaskDSN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			in:   "postgres://admin:secret123@localhost:5432/nook?sslmode=disable",
			want: "postgres://admin:******@localhost:5432/nook?sslmode=disable",
		},
		{
			in:   "host=localhost port=5432 user=admin password=secret dbname=nook sslmode=disable",
			want: "host=localhost port=5432 user=admin password=****** dbname=nook sslmode=disable",
		},
		{
			in:   "postgres://localhost/nook",
			want: "postgres://localhost/nook",
		},
		{
			in:   "",
			want: "",
		},
	}

	for _, c := range cases {
		got := MaskDSN(c.in)
		if got != c.want {
			t.Errorf("MaskDSN(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInspectDatabaseStatus(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Media{}, &model.PlaybackHistory{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Username: "testuser", PasswordHash: "h", Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	deleted := &model.User{Username: "deleted-user", PasswordHash: "h", Role: "user"}
	if err := db.Create(deleted).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(deleted).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Database.Type = "postgres"
	cfg.Database.DSN = "postgres://admin:secret@localhost/nook"

	st := InspectDatabaseStatus(context.Background(), db, cfg)
	if st == nil {
		t.Fatal("expected non-nil DatabaseStatus")
	}
	if st.Type != "postgres" {
		t.Fatalf("expected postgres, got %s", st.Type)
	}
	if st.TableCounts["users"] != 2 {
		t.Fatalf("expected the same raw row count including soft-deleted users, got %d", st.TableCounts["users"])
	}
	if !st.Connected || st.Error != "" || st.CountsError != "" {
		t.Fatalf("expected a healthy database, got %+v", st)
	}
	if len(st.TableCounts) != 3 || st.TableCounts["media"] != 0 || st.TableCounts["playback_histories"] != 0 {
		t.Fatalf("expected all three core counts including real zeroes, got %v", st.TableCounts)
	}
	if st.DSN != "postgres://admin:******@localhost/nook" {
		t.Fatalf("expected a masked connection address, got %q", st.DSN)
	}
}

func TestInspectDatabaseStatusUnavailable(t *testing.T) {
	st := InspectDatabaseStatus(context.Background(), nil, nil)
	if st.Connected || st.Error == "" || len(st.TableCounts) != 0 {
		t.Fatalf("an unavailable database must not report healthy or fabricated counts: %+v", st)
	}
}

func TestInspectDatabaseStatusFailedCount(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	st := InspectDatabaseStatus(context.Background(), db, nil)
	if !st.Connected || st.CountsError == "" {
		t.Fatalf("expected connected with a statistics error: %+v", st)
	}
	if _, ok := st.TableCounts["playback_histories"]; ok {
		t.Fatal("a failed count must not be reported as zero")
	}
	if count, ok := st.TableCounts["users"]; !ok || count != 0 {
		t.Fatalf("a successfully counted empty table should report zero: %v", st.TableCounts)
	}
}

func TestInspectDatabaseStatusClosedPool(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	st := InspectDatabaseStatus(context.Background(), db, nil)
	if st.Connected || st.Error == "" || len(st.TableCounts) != 0 {
		t.Fatalf("a closed pool must report disconnected without counts: %+v", st)
	}
}

func TestInspectDatabaseStatusCanceled(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st := InspectDatabaseStatus(ctx, db, nil)
	if st.Connected || len(st.TableCounts) != 0 {
		t.Fatalf("a canceled request must not report a successful probe: %+v", st)
	}
}
