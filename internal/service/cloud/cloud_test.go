package cloud

import "testing"

func TestUnsupportedProvider(t *testing.T) {
	if _, err := New("dropbox", nil, nil); err != ErrUnsupported {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	if _, err := New("quark", nil, nil); err != ErrUnsupported {
		t.Fatalf("quark should be unsupported, got %v", err)
	}
	if IsCloudType("quark") {
		t.Fatal("quark should not be an active cloud provider")
	}
	if _, err := New("clouddrive2", nil, nil); err != ErrUnsupported {
		t.Fatalf("clouddrive2 should be unsupported, got %v", err)
	}
	if IsCloudType("clouddrive2") {
		t.Fatal("clouddrive2 should not be an active cloud provider")
	}
}
