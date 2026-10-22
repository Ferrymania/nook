package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestDevicePayloadDisplayName(t *testing.T) {
	for _, tt := range []struct {
		name, raw, want string
	}{
		{"space", "Apple%20TV", "Apple TV"},
		{"chinese", "%E5%AE%A2%E5%8E%85%20TV", "客厅 TV"},
		{"emoji", "%F0%9F%93%BA%20TV", "📺 TV"},
		{"plain", "客厅 Apple TV", "客厅 Apple TV"},
		{"plus", "Apple+TV%20Pro", "Apple+TV Pro"},
		{"literal_percent", "Battery 100%", "Battery 100%"},
		{"malformed_escape", "Apple%20TV%GG", "Apple%20TV%GG"},
		{"truncated_escape", "Apple%2", "Apple%2"},
		{"invalid_utf8", "Apple%FFTV", "Apple%FFTV"},
		{"decode_once", "Apple%2520TV", "Apple%20TV"},
		{"empty", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			device := model.UserDevice{
				Base: model.Base{ID: "row%20id"}, DeviceID: "device%20id",
				DeviceName: tt.raw, Client: "Lenna", Fingerprint: "original-fingerprint",
			}
			got := toDevicePayload(device)
			if got.DeviceName != tt.want {
				t.Errorf("device name = %q, want %q", got.DeviceName, tt.want)
			}
			if got.ID != device.ID || got.DeviceID != device.DeviceID || got.Client != device.Client {
				t.Errorf("device identity changed: %+v", got)
			}
		})
	}
}

func TestDeviceListsDecodeNamesWithoutChangingStoredDevice(t *testing.T) {
	router, svc := newDeviceTestEnv(t)
	seedDeviceUsers(t, svc)
	ctx := context.Background()
	// Seed a pre-existing encoded name without a realtime session. Old records
	// must display correctly without another login or a database rewrite.
	device := model.UserDevice{
		UserID: "user-a", DeviceID: "apple%20tv-id", DeviceName: "Apple%20TV",
		Client: "Lenna", Fingerprint: "original-fingerprint",
	}
	if err := svc.Repo.UserDevice.Create(ctx, &device); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct {
		path, userID, role string
	}{
		{"/api/me/devices", "user-a", "user"},
		{"/api/admin/users/user-a/devices", "admin-1", "admin"},
	} {
		t.Run(route.path, func(t *testing.T) {
			w := doJSON(t, router, http.MethodGet, route.path, route.userID, route.role)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
			var payload deviceListPayload
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Devices) != 1 {
				t.Fatalf("devices = %+v, want one device", payload.Devices)
			}
			got := payload.Devices[0]
			if got.DeviceName != "Apple TV" || got.DeviceID != device.DeviceID || got.ID != device.ID {
				t.Fatalf("device = %+v, want decoded name with original identity", got)
			}
		})
	}
	stored, err := svc.Repo.UserDevice.ListByUser(ctx, device.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].DeviceName != device.DeviceName ||
		stored[0].DeviceID != device.DeviceID || stored[0].Fingerprint != device.Fingerprint {
		t.Fatalf("stored device changed: %+v", stored)
	}
}
