package service

import (
	"regexp"
	"strconv"
	"strings"
)

// ProbeVideoSideData retains only HDR/DV technical metadata from -show_streams.
// Do not retain arbitrary raw side data: ffprobe output can contain source URLs.
type ProbeVideoSideData struct {
	Type                      string               `json:"side_data_type"`
	DVVersionMajor            *int                 `json:"dv_version_major,omitempty"`
	DVVersionMinor            *int                 `json:"dv_version_minor,omitempty"`
	DVProfile                 *int                 `json:"dv_profile,omitempty"`
	DVLevel                   *int                 `json:"dv_level,omitempty"`
	RPUPresentFlag            *int                 `json:"rpu_present_flag,omitempty"`
	ELPresentFlag             *int                 `json:"el_present_flag,omitempty"`
	BLPresentFlag             *int                 `json:"bl_present_flag,omitempty"`
	DVBLSignalCompatibilityID *int                 `json:"dv_bl_signal_compatibility_id,omitempty"`
	DVMDCompression           ProbeDVMDCompression `json:"dv_md_compression,omitempty"`
	RedX                      string               `json:"red_x,omitempty"`
	RedY                      string               `json:"red_y,omitempty"`
	GreenX                    string               `json:"green_x,omitempty"`
	GreenY                    string               `json:"green_y,omitempty"`
	BlueX                     string               `json:"blue_x,omitempty"`
	BlueY                     string               `json:"blue_y,omitempty"`
	WhitePointX               string               `json:"white_point_x,omitempty"`
	WhitePointY               string               `json:"white_point_y,omitempty"`
	MinLuminance              string               `json:"min_luminance,omitempty"`
	MaxLuminance              string               `json:"max_luminance,omitempty"`
	MaxContent                *int                 `json:"max_content,omitempty"`
	MaxAverage                *int                 `json:"max_average,omitempty"`
}

// ffprobe versions expose dv_md_compression either as a numeric enum or as
// "none"/"limited"/"extended". Unknown optional values must not fail a probe.
type ProbeDVMDCompression string

func (c *ProbeDVMDCompression) UnmarshalJSON(data []byte) error {
	switch strings.Trim(string(data), `"`) {
	case "0", "none":
		*c = "none"
	case "1", "limited":
		*c = "limited"
	case "2", "reserved":
		*c = "reserved"
	case "3", "extended":
		*c = "extended"
	default:
		*c = ""
	}
	return nil
}

func retainedProbeVideoSideData(raw []ProbeVideoSideData) []ProbeVideoSideData {
	var out []ProbeVideoSideData
	for _, s := range raw {
		switch s.Type {
		case "DOVI configuration record":
			out = append(out, ProbeVideoSideData{
				Type: s.Type, DVVersionMajor: s.DVVersionMajor, DVVersionMinor: s.DVVersionMinor,
				DVProfile: s.DVProfile, DVLevel: s.DVLevel, RPUPresentFlag: s.RPUPresentFlag,
				ELPresentFlag: s.ELPresentFlag, BLPresentFlag: s.BLPresentFlag,
				DVBLSignalCompatibilityID: s.DVBLSignalCompatibilityID, DVMDCompression: s.DVMDCompression,
			})
		case "Mastering display metadata":
			out = append(out, ProbeVideoSideData{
				Type: s.Type, RedX: s.RedX, RedY: s.RedY, GreenX: s.GreenX, GreenY: s.GreenY,
				BlueX: s.BlueX, BlueY: s.BlueY, WhitePointX: s.WhitePointX, WhitePointY: s.WhitePointY,
				MinLuminance: s.MinLuminance, MaxLuminance: s.MaxLuminance,
			})
		case "Content light level metadata":
			out = append(out, ProbeVideoSideData{Type: s.Type, MaxContent: s.MaxContent, MaxAverage: s.MaxAverage})
		case "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)":
			// The stream-level marker is enough for identification. No frame scan
			// or raw dynamic-metadata payload is requested or persisted.
			out = append(out, ProbeVideoSideData{Type: s.Type})
		}
	}
	return out
}

var probePixelBitDepth = regexp.MustCompile(`(?:p|gray)(9|10|12|14|16)(?:le|be)$`)

func probeBitDepth(raw, sample int, kind, pixelFormat string) int {
	for _, depth := range []int{raw, sample} {
		if depth > 0 && depth <= 64 {
			return depth
		}
	}
	if kind != "video" {
		return 0
	}
	pixelFormat = strings.ToLower(strings.TrimSpace(pixelFormat))
	if match := probePixelBitDepth.FindStringSubmatch(pixelFormat); len(match) == 2 {
		depth, _ := strconv.Atoi(match[1])
		return depth
	}
	switch pixelFormat {
	case "p010le", "p010be", "p210le", "p210be":
		return 10
	case "p016le", "p016be", "p216le", "p216be":
		return 16
	case "yuv420p", "yuv422p", "yuv444p", "yuvj420p", "yuvj422p", "yuvj444p", "nv12", "nv21", "gbrp", "rgb24", "bgr24", "rgba", "bgra", "argb", "abgr", "gray":
		return 8
	}
	return 0
}
