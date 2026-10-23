package service

import "strings"

// Emby MediaStream fields and enums, not Jellyfin's VideoRangeType/DvProfile:
// https://dev.emby.media/reference/pluginapi/MediaBrowser.Model.Entities.MediaStream.html
// https://dev.emby.media/reference/pluginapi/MediaBrowser.Model.Entities.ExtendedVideoSubTypes.html
func appendEmbyVideoMetadata(stream map[string]any, s ProbeStream) {
	for key, value := range map[string]string{
		"CodecTag": s.CodecTag, "ColorSpace": s.ColorSpace,
		"ColorTransfer": s.ColorTransfer, "ColorPrimaries": s.ColorPrimaries,
	} {
		if value != "" {
			stream[key] = value
		}
	}
	if depth := probeBitDepth(s.BitDepth, 0, s.Type, s.PixFmt); depth > 0 {
		stream["BitDepth"] = depth
	}
	kind, subtype, description, videoRange := embyExtendedVideoType(s)
	if kind == "" {
		// Legacy or insufficient metadata is unknown, never assumed SDR/HDR
		// from a filename, HEVC Main 10 profile, or pixel format alone.
		return
	}
	stream["ExtendedVideoType"] = kind
	stream["ExtendedVideoSubType"] = subtype
	if description != "" {
		stream["ExtendedVideoSubTypeDescription"] = description
	}
	stream["VideoRange"] = videoRange
}

func embyExtendedVideoType(s ProbeStream) (kind, subtype, description, videoRange string) {
	hdr10Plus := false
	for _, side := range s.SideDataList {
		if side.Type == "DOVI configuration record" && side.DVProfile != nil && *side.DVProfile >= 0 && *side.DVProfile <= 127 && side.RPUPresentFlag != nil && *side.RPUPresentFlag == 1 {
			subtype, description = embyDoviSubType(side)
			return "DolbyVision", subtype, description, "Dolby Vision"
		}
		if side.Type == "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)" {
			hdr10Plus = true
		}
	}
	if hdr10Plus {
		return "Hdr10Plus", "Hdr10Plus0", "HDR 10+", "HDR 10+"
	}
	switch strings.ToLower(strings.TrimSpace(s.ColorTransfer)) {
	case "smpte2084":
		return "Hdr10", "Hdr10", "HDR 10", "HDR 10"
	case "arib-std-b67":
		return "HyperLogGamma", "HyperLogGamma", "HLG", "HLG"
	case "bt709", "smpte170m", "smpte240m", "bt470m", "bt470bg", "gamma22", "gamma28", "iec61966-2-1", "bt2020-10", "bt2020-12":
		return "None", "None", "", "SDR"
	}
	return "", "", "", ""
}

// A DV subtype describes the profile/base-layer compatibility, not dv_level.
// In particular, Profile 8 Level 6 with compatibility ID 1 is DoviProfile81.
func embyDoviSubType(side ProbeVideoSideData) (string, string) {
	profile := *side.DVProfile
	compat := -1
	if side.DVBLSignalCompatibilityID != nil {
		compat = *side.DVBLSignalCompatibilityID
	}
	switch profile {
	case 1:
		return "DoviProfile10", "Profile 1.0"
	case 3:
		return "DoviProfile30", "Profile 3.0"
	case 5:
		return "DoviProfile50", "Profile 5.0"
	case 7:
		return "DoviProfile76", "Profile 7.6 (Bluray)"
	case 0, 2, 4, 9:
		if compat == 2 {
			switch profile {
			case 0:
				return "DoviProfile02", "Profile 0.2 (SDR)"
			case 2:
				return "DoviProfile22", "Profile 2.2 (SDR)"
			case 4:
				return "DoviProfile42", "Profile 4.2 (SDR)"
			case 9:
				return "DoviProfile92", "Profile 9.2 (SDR)"
			}
		}
	case 6:
		if compat == 1 {
			return "DoviProfile61", "Profile 6.1 (HDR10 compatible)"
		}
	case 8:
		switch compat {
		case 1:
			return "DoviProfile81", "Profile 8.1 (HDR10 compatible)"
		case 2:
			return "DoviProfile82", "Profile 8.2 (SDR)"
		case 3:
			return "DoviProfile83", "Profile 8.3"
		case 4:
			return "DoviProfile84", "Profile 8.4 (HLG compatible)"
		case 5:
			return "DoviProfile85", "Profile 8.5"
		}
	}
	// Keep DolbyVision as the main type for newer/unknown profiles without
	// inventing an enum value that existing Emby clients cannot deserialize.
	return "None", ""
}
