package database

import (
	"testing"

	"github.com/glebarez/sqlite"
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
			in:   "sqlite://data/nook.db",
			want: "sqlite://data/nook.db",
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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	_ = db.Create(&model.User{Username: "testuser", PasswordHash: "h", Role: "user"}).Error

	cfg := &config.Config{}
	cfg.Database.Type = "sqlite"
	cfg.Database.DBPath = "./data/nook.db"

	st := InspectDatabaseStatus(db, cfg)
	if st == nil {
		t.Fatal("expected non-nil DatabaseStatus")
	}
	if st.Type != "sqlite" {
		t.Fatalf("expected sqlite, got %s", st.Type)
	}
	if st.DBPath != "./data/nook.db" {
		t.Fatalf("expected db_path, got %s", st.DBPath)
	}
	if st.TableCounts["users"] != 1 {
		t.Fatalf("expected 1 user, got %d", st.TableCounts["users"])
	}
}
