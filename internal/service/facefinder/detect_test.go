package facefinder

import (
	"image"
	_ "image/jpeg"
	"os"
	"testing"
)

// sample.jpg is the upstream Pigo fixture; see the accompanying MIT license.
func TestAxisUsesEmbeddedClassifier(t *testing.T) {
	f, err := os.Open("testdata/sample.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	pos, ok := Axis(img, 2.0/3.0, true)
	if !ok || pos <= 0 || pos >= 1 {
		t.Fatalf("face axis = %v, %v", pos, ok)
	}
	if _, ok := Axis(image.NewRGBA(image.Rect(0, 0, 400, 200)), 2.0/3.0, true); ok {
		t.Fatal("blank image must use the fallback crop")
	}
}
