// Package facefinder positions poster crops using Pigo's local face classifier.
// The embedded cascade and classifier are distributed under the accompanying MIT license.
package facefinder

import (
	_ "embed"
	"image"

	"github.com/disintegration/imaging"
	pigo "github.com/esimov/pigo/core"
)

//go:embed cascade
var cascade []byte
var classifier, classifierErr = pigo.NewPigo().Unpack(cascade)

// Axis returns the dominant face position on the dimension being cropped.
func Axis(img image.Image, ratio float64, advanced bool) (float64, bool) {
	if classifierErr != nil || img.Bounds().Empty() {
		return 0, false
	}
	img = imaging.Fit(img, 1280, 1280, imaging.Linear)
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	params := pigo.CascadeParams{
		MinSize: 20, MaxSize: min(width, height), ShiftFactor: 0.1, ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: pigo.RgbToGrayscale(img), Rows: height, Cols: width, Dim: width},
	}
	angles := []float64{0}
	if advanced {
		angles = append(angles, 0.02, 0.98)
	}
	var faces []pigo.Detection
	for _, angle := range angles {
		faces = append(faces, classifier.RunCascade(params, angle)...)
	}
	faces = classifier.ClusterDetections(faces, 0.2)
	var best pigo.Detection
	var weight float64
	for _, face := range faces {
		if face.Q < 5 {
			continue
		}
		if score := float64(face.Q) * float64(face.Scale); score > weight {
			weight = score
			best = face
		}
	}
	if weight == 0 {
		return 0, false
	}
	if float64(height)*ratio < float64(width) {
		return float64(best.Col) / float64(width), true
	}
	return float64(best.Row) / float64(height), true
}
