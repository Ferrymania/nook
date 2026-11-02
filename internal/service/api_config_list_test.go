package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func TestAPIConfigRejectsPlaintextStoredKey(t *testing.T) {
	t.Parallel()
	repos := repository.New(newServiceTestDB(t))
	cipher := NewCryptoService("test-secret", zap.NewNop())
	svc := NewAPIConfigService(zap.NewNop(), repos, cipher)
	connection := NewApiConfigService(nil, zap.NewNop(), repos, cipher)
	const plaintext = "plaintext-api-key-fixture"
	if err := repos.DB.Create(&model.APIConfig{Provider: "tmdb", APIKey: plaintext}).Error; err != nil {
		t.Fatal(err)
	}
	for name, read := range map[string]func() error{
		"resolve":         func() error { _, err := svc.Resolve(t.Context(), "tmdb"); return err },
		"get":             func() error { _, err := svc.Get(t.Context(), "tmdb"); return err },
		"list":            func() error { _, err := svc.List(t.Context()); return err },
		"connection get":  func() error { _, err := connection.GetByProvider(t.Context(), "tmdb"); return err },
		"connection list": func() error { _, err := connection.List(t.Context()); return err },
	} {
		t.Run(name, func(t *testing.T) {
			err := read()
			if !errors.Is(err, helper.ErrSecretNotEncrypted) {
				t.Fatalf("read error = %v, want unencrypted credential error", err)
			}
			if strings.Contains(err.Error(), plaintext) {
				t.Fatal("credential leaked in error")
			}
		})
	}
	replacement := "replacement-key"
	if _, err := svc.Update(t.Context(), "tmdb", APIConfigPatch{APIKey: &replacement}); err != nil {
		t.Fatal(err)
	}
	resolved, err := svc.Resolve(t.Context(), "tmdb")
	if err != nil || resolved.APIKey != replacement {
		t.Fatalf("replacement key must be encrypted and readable: %v", err)
	}
}

func TestAPIConfigListHidesScraperSourcesAndPrioritizesConfigured(t *testing.T) {
	db := newServiceTestDB(t)
	svc := NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()))
	ctx := t.Context()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	var scraperDefaults int64
	if err := db.Model(&model.APIConfig{}).Where("provider IN ?", []string{"adult", "douban", "retired_source"}).Count(&scraperDefaults).Error; err != nil {
		t.Fatal(err)
	}
	if scraperDefaults != 0 {
		t.Fatal("scraper providers must not be seeded")
	}
	for _, provider := range []string{"tmdb", "bangumi", "adult", "douban", "retired_source"} {
		key := "configured-" + provider + "-key"
		if _, err := svc.Update(ctx, provider, APIConfigPatch{APIKey: &key}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	providers := make([]string, 0, len(items))
	for _, item := range items {
		providers = append(providers, item.Provider)
		if item.HasKey && (item.MaskedKey == "" || item.MaskedKey == "configured-"+item.Provider+"-key") {
			t.Fatalf("configured key must remain masked: %s", item.Provider)
		}
	}
	want := []string{"bangumi", "tmdb", "fanart", "openai", "thetvdb"}
	if !reflect.DeepEqual(providers, want) || !items[0].HasKey || !items[1].HasKey {
		t.Fatalf("provider order = %v, want %v with configured keys first", providers, want)
	}
	if err := svc.Delete(ctx, "bangumi"); err != nil {
		t.Fatal(err)
	}
	items, err = svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Provider != "tmdb" || !items[0].HasKey || items[1].Provider != "bangumi" || items[1].HasKey {
		t.Fatal("clearing a key must return its provider to the unconfigured section")
	}
}
