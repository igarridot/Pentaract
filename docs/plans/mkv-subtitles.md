# Plan: subtitles for MKV playback in the web UI

Status: **not started** (decided on 2026-10-17 to ship MKV playback without subtitles first).

This document is written for whoever picks the work up next, human or Claude Code. It states what exists, what the constraints are, and a step-by-step plan with tests. Read `CLAUDE.md` first for the project rules (every change needs Go and JS tests).

## Where we are

MKV files play in the web UI through an on-the-fly HLS pipeline (`internal/service/hls.go`). One ffmpeg process per (user, file, audio track) reads the file back from the server over loopback and writes fragmented MP4 segments. Today the ffmpeg command line built by `hlsFFmpegArgs` passes `-sn`, so every subtitle track is dropped.

What already exists and helps:

- `internal/media/matroska.go` parses the Matroska header and lists subtitle tracks with their codec (`subrip`, `ass`, `ssa`, `webvtt`, `pgs`, `vobsub`, `dvbsub`), language, name and default flag. `GET /storages/{id}/files/media_info/{path}` returns them, so the UI already knows which subtitle tracks a file has.
- `media.Plan` (`internal/media/plan.go`) decides what ffmpeg copies or transcodes. Subtitle choice belongs there, next to the audio choice.
- The HLS session key is `(user, file, audio track)`. A subtitle choice that changes the ffmpeg command must become part of that key, exactly like `audio` does, so two choices never share a session directory.
- `ui/src/components/HlsPlayer.jsx` has the audio track selector and `ui/src/common/hls_playback.js` the pure helpers (`audioTracks`, `audioTrackLabel`, `describePlayback`). Subtitles follow the same shape.

## The constraint that shapes everything

Subtitle packets are interleaved with video inside the Matroska clusters. Extracting a whole subtitle track means reading the whole file, which for an 11 GB BDRemux means downloading every chunk from Telegram. **Any design that extracts the full subtitle track up front is wrong for this project.** Subtitles must come out of the same ffmpeg session that produces the video segments, window by window, so they cost no extra Telegram reads.

## Subtitle types and what the browser can do

| Codec in MKV | Nature | Browser can render | Realistic path |
|---|---|---|---|
| `subrip` (SRT), `webvtt` | text | yes, as WebVTT | convert with ffmpeg (`-c:s webvtt`) |
| `ass`, `ssa` | text with styling | only the plain text as WebVTT; styles and positions need a renderer | WebVTT first (loses styling); later `jassub` / `SubtitlesOctopus` (libass in WebAssembly) if styling matters, which also needs the font attachments |
| `pgs` (Blu-ray) | bitmaps | no | burn into the video (forces full video transcode) or a JavaScript PGS renderer fed with `.sup` data |
| `vobsub`, `dvbsub` | bitmaps | no | same as PGS |

The user's BDRemux files carry PGS, so bitmap subtitles are the case that matters most and also the hardest one.

## Plan

### Phase 1: text subtitles as an HLS subtitle rendition

Goal: SRT/ASS/SSA/WebVTT tracks become selectable in the player and render through the browser's own `<track>` machinery, with no extra Telegram reads.

Backend:

1. `media.Plan` gets `Subtitle *Track` and `SubtitleIndex int` (position among the file's subtitle tracks, for `-map 0:s:N`). `BuildPlan(info, audio, subtitle)` validates the requested track number the way it does for audio (`ErrSubtitleTrackNotFound`). Only text codecs are eligible in this phase; a bitmap track must be rejected with a clear error.
2. `hlsSessionKey` gains `subtitle uint64`. The HTTP routes take `?subtitle=N`, parsed like `parseAudioTrack` in `internal/handler/media_handler.go`. The session is restarted when the choice changes, exactly like audio.
3. `hlsFFmpegArgs` maps the subtitle stream and converts it: `-map 0:s:N -c:s webvtt`, and drops `-sn` when a subtitle is chosen. ffmpeg's hls muxer writes subtitle segments next to the video ones when a subtitle stream is mapped: it emits `<name>_vtt.m3u8` plus `segNNNNN.vtt` files cut on the same `-hls_time` windows and honouring `-start_number`. Verify the exact output names with the ffmpeg in the Docker image before relying on them (run the real-ffmpeg integration test, `internal/service/hls_ffmpeg_test.go`, with a fixture that has an SRT track; `internal/media/testdata/sample.mkv` can be regenerated with `-c:s srt` added).
4. `BuildHLSPlaylist` becomes a master playlist plus two media playlists when a subtitle is chosen:
   - `index.m3u8` (master): `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="<label>",LANGUAGE="<lang>",DEFAULT=YES,AUTOSELECT=YES,URI="subs.m3u8?<query>"` and `#EXT-X-STREAM-INF:BANDWIDTH=...,SUBTITLES="subs"` pointing at `video.m3u8?<query>`.
   - `video.m3u8`: what `index.m3u8` is today.
   - `subs.m3u8`: same segment list with `segNNNNN.vtt` entries and no `EXT-X-MAP`.
   Keep `index.m3u8` unchanged when no subtitle is requested, so existing behaviour and tests stay intact.
5. `Resource` accepts `segNNNNN.vtt` (extend `hlsSegmentNameRe` or add a second pattern) and serves it with `Content-Type: text/vtt`. The wait logic is the same: the vtt for window N appears when ffmpeg finishes window N.
6. `media_info` adds `subtitle_tracks_playable` or marks each subtitle track with `playable: true/false` so the UI only offers what Phase 1 supports.

Frontend:

7. `hls_playback.js`: `subtitleTracks(info)`, `playableSubtitleTracks(info)`, `subtitleTrackLabel(track)`; `describePlayback` mentions the chosen subtitle. Node tests for each.
8. `HlsPlayer.jsx`: a "Subtitles" selector (Off + playable tracks). The choice is part of the playlist URL (`API.files.hlsPlaylistUrl(storageId, path, audio, subtitle)`). hls.js exposes the rendition through `hls.subtitleTracks`; set `hls.subtitleTrack = 0` and `hls.subtitleDisplay = true` once `Hls.Events.SUBTITLE_TRACKS_UPDATED` fires. On native HLS (iOS Safari) the browser shows the rendition in its own menu.
9. API client tests in `ui/src/api/index.test.js` for the new query parameter.

Acceptance: an MKV with an SRT track plays in Chrome with subtitles on after choosing them, seeking keeps subtitles in sync, and the backend log shows no whole-file read (no range requests beyond the segments' windows). Go and JS tests cover the plan, the args, the playlists, the resource names and the UI helpers.

### Phase 2: bitmap subtitles (PGS, VobSub)

Pick one of these, in order of preference:

- **A. Burn-in on demand.** Add a `burn_subtitle=N` option that switches the video to transcode (`-filter_complex "[0:v:0][0:s:N]overlay"`, `-c:v libx264`) in the same session model. Simple, correct, but recodes the video: slow on a weak CPU and lower quality. Make it an explicit user choice in the UI with a warning. This is the fallback that always works.
- **B. Client-side PGS renderer.** Extract the PGS stream per session (`-map 0:s:N -c:s copy -f sup` into per-window files, or one `.sup` stream alongside the segments) and draw it on a canvas over the video with a JavaScript PGS decoder (search npm for a maintained one; `libpgs` was a candidate when this was written). Keeps video copy intact, but the renderer has to be synced to `video.currentTime` and to seeks, and segment-aligned `.sup` output from ffmpeg needs verification.
- **C. Server-side OCR to text.** Tesseract on the PGS bitmaps, then Phase 1. Heavy dependency, slow, error-prone. Only if A and B are both unacceptable.

### Phase 3: styled ASS

If users care about ASS styling (anime fansubs), render with libass in WebAssembly (`jassub`) fed with the ASS track and the font attachments from the Matroska `Attachments` element. Needs: an endpoint that extracts attachments (they sit after the Tracks element, within the header read) and a renderer wired like Phase 2B.

## Non-goals and gotchas

- Do not extract subtitles outside the HLS session: it re-downloads the whole file from Telegram.
- Keep Kodi and other external players on the native `?inline=1` stream; nothing here touches them. Kodi already handles every subtitle type by itself.
- The chunk range path and `hls.js` timeouts are tuned for Telegram's latency (see `buildHlsConfig`); subtitle requests ride the same loader policy.
- `-sn` and `-dn` are in `hlsFFmpegArgs`; drop `-sn` only when a subtitle is mapped, otherwise ffmpeg may fail on a file whose subtitle codec it cannot encode.
