package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// UserAccountMediaVisibility applies administrator-assigned library access.
func UserAccountMediaVisibility(ctx context.Context, repo *repository.Container, userID string) MediaVisibility {
	visibility := MediaVisibility{}
	if repo == nil {
		return visibility
	}
	if userID == "" {
		return visibility
	}

	if repo.User != nil {
		user, err := repo.User.FindByID(ctx, userID)
		if err == nil && user != nil && user.Role != "admin" {
			if userAllowed := user.DecodeAllowedLibraryIDs(); len(userAllowed) > 0 {
				visibility.AllowedLibraryIDs = userAllowed
			}
		}
	}

	return visibility
}

// UserDefaultMediaVisibility applies the same account preferences to web and
// Emby-compatible clients. A user's selection can only narrow administrator access.
func UserDefaultMediaVisibility(ctx context.Context, repo *repository.Container, userID string) MediaVisibility {
	visibility := UserAccountMediaVisibility(ctx, repo, userID)
	if repo == nil || repo.User == nil || userID == "" {
		return visibility
	}
	if user, err := repo.User.FindByID(ctx, userID); err == nil && user != nil {
		if allowed := user.DecodePlaybackSettings().AllowedLibraryIDs; len(allowed) > 0 {
			if len(visibility.AllowedLibraryIDs) > 0 {
				visibility.AllowedLibraryIDs = IntersectStrings(visibility.AllowedLibraryIDs, allowed)
			} else {
				visibility.AllowedLibraryIDs = allowed
			}
		}
	}
	return visibility
}

// IntersectStrings 计算两个字符串切片的交集。
func IntersectStrings(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	set := make(map[string]struct{}, len(b))
	for _, s := range b {
		set[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, ok := set[s]; ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return []string{"__no_access__"}
	}
	return out
}

// DecodeAllowedLibraryIDs normalises an allowed-library JSON string.
func DecodeAllowedLibraryIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	out := ids[:0]
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, strings.TrimSpace(id))
		}
	}
	return out
}

// LibraryIDAllowed reports whether libraryID is permitted by the allow-list.
// An empty AllowedLibraryIDs means unrestricted access.
func LibraryIDAllowed(visibility MediaVisibility, libraryID string) bool {
	if len(visibility.AllowedLibraryIDs) == 0 {
		return true
	}
	for _, id := range visibility.AllowedLibraryIDs {
		if id == libraryID {
			return true
		}
	}
	return false
}

// EmbyMountLibraryID is the web/Emby library id for a mounted remote view.
func EmbyMountLibraryID(mount *model.EmbyMount) string {
	if mount == nil {
		return ""
	}
	return EncodeEmbyRemoteID(mount.ID, mount.RemoteViewID)
}

// EmbyMountLibraryAllowed reports whether a mounted Emby library is allowed for
// the given visibility policy.
func EmbyMountLibraryAllowed(visibility MediaVisibility, mount *model.EmbyMount) bool {
	return LibraryIDAllowed(visibility, EmbyMountLibraryID(mount))
}

// LibraryVisibleForUser applies library access permissions to a library card/folder.
func LibraryVisibleForUser(_ context.Context, _ *repository.Container, lib model.Library, visibility MediaVisibility) bool {
	return LibraryIDAllowed(visibility, lib.ID)
}
