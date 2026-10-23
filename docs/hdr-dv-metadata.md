# HDR / Dolby Vision metadata

Nook retains the HDR/DV information already returned by its existing single
`ffprobe -show_format -show_streams -show_chapters` invocation. It does not add
`-show_frames`, a second ffprobe invocation, or any playback-time probing.
HDR10+ is advertised only if its explicit SMPTE2094-40 marker is present in the
stream output. Its absence does not prove that a PQ source lacks HDR10+.

The version-2 probe JSON retains color transfer/primaries/space/range, codec tag,
bit depth, Dolby Vision configuration (including level and base-layer
compatibility), mastering-display and content-light metadata, and an HDR10+
presence marker. Unknown raw side-data and signed source URLs are not stored.

Emby item details, embedded media sources and PlaybackInfo use the same mapping:

| Detected information | ExtendedVideoType | ExtendedVideoSubType |
| --- | --- | --- |
| PQ transfer | Hdr10 | Hdr10 |
| Explicit HDR10+ marker | Hdr10Plus | Hdr10Plus0 |
| HLG transfer | HyperLogGamma | HyperLogGamma |
| DV configuration with RPU flag | DolbyVision | Official DoviProfile… value, or None if unknown |
| Known SDR transfer | None | None |
| Insufficient or legacy metadata | Omitted | Omitted |

Dolby Vision takes precedence over its HDR-compatible base layer. DV profile 8
subtypes use **base-layer compatibility ID**, not `dv_level`; for example,
profile 8, compatibility 1, level 6 is `DoviProfile81`. Bit depth or filenames
alone never imply HDR. `VideoRange` is supplied as a display/compatibility
string, together with the official enum fields and available `BitDepth`,
`CodecTag`, `ColorSpace`, `ColorTransfer`, and `ColorPrimaries`.

Existing probe caches remain readable and are not automatically invalidated.
Old films need an explicit re-probe to obtain the newly retained metadata;
upgrading does not start a whole-library or whole-server re-probe. A failed
re-probe keeps the last successful payload. Administrators should select one
library explicitly when validating old films, rather than invalidating caches
across the server.

Validation against 108 real stream-probe results found 42 Dolby Vision sources
(13 profile 5, 20 profile 8.1, 9 profile 7), 25 PQ/HDR10 sources, 13 sources with
explicit SDR transfer characteristics, and 28 with insufficient range metadata.
All samples parsed and round-tripped through the new payload successfully.
No frame probing was added; the PQ count does not rule out HDR10+ on those
sources. Cache backfill was limited to the selected library after a backup.

Official definitions:

- [MediaStream](https://dev.emby.media/reference/pluginapi/MediaBrowser.Model.Entities.MediaStream.html)
- [ExtendedVideoTypes](https://dev.emby.media/reference/pluginapi/MediaBrowser.Model.Entities.ExtendedVideoTypes.html)
- [ExtendedVideoSubTypes](https://dev.emby.media/reference/pluginapi/MediaBrowser.Model.Entities.ExtendedVideoSubTypes.html)
- [FFmpeg Dolby Vision metadata](https://ffmpeg.org/doxygen/trunk/dovi__meta_8h_source.html)
