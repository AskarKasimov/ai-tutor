# VoxCPM Performance Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development to implement and review the tasks below. Each subsystem owns its files; coordinate through the stream protocol in the spec.

**Goal:** Deliver optional compiled inference, reusable voice features and streaming speech from VoxCPM to the browser.

**Architecture:** A framed NDJSON PCM protocol connects the existing TTS service, authenticated backend and AudioWorklet player. Existing WAV clients and S3 cache remain supported. Performance rollout follows measurements, not assumptions.

**Tech Stack:** Python/FastAPI/PyTorch, Go HTTP, TypeScript/React/Web Audio.

**Spec:** [voxcpm-performance-spec.md](voxcpm-performance-spec.md).

## Global Constraints

- Existing default 20 steps stays until quality and CUDA measurements pass; compile defaults off; streaming defaults disabled.
- One model lock; bounded reference LRU 4 entries (configurable 0–16), stream producer queue 8 blocks × 64 KiB, 5 second drain limit.
- NDJSON start/audio/end or error, mono pcm_s16le, 64 MiB decoded maximum, 256 KiB line maximum; continuous seq and exact sample count.
- Authenticate ownership before provider/cache access. No partially generated S3 entries. Existing WAV endpoint remains.
- Pin voice content independently of target timesteps. Profile includes model/revision, voice digest, seed/CFG/steps and text pipeline version.
- No deployment, weight downloads or external writes required for implementation; unavailable CUDA/browser/audio acceptance must be reported honestly.

## Review Focus

- Disconnect while waiting for model lock or blocked transport must release resources.
- Singleflight spanning live stream, WAV listener and warmer must not duplicate syntheses or leak producer goroutines.
- Streaming errors after first PCM must not replay speech automatically or publish a partial cache.
- Sample-rate mismatch and chunk boundaries must not corrupt playback or exceed queue limits.
- Compile exceptions swallowed by upstream and deferred first-inference failures must produce truthful readiness/fallback.

### Task 1: TTS runtime and streaming

**Files:** `tts-stt/voxcpm2/api.py`, a focused runtime/stream helper beside it, `src/voxcpm/core.py` / model optimize where required, `tests/http`, TTS Compose and API docs.

**Interfaces:** Produces `POST /synthesize/stream` from spec §7.1; same synthesis JSON; `/ready`; VOXCPM_* settings. Existing `/synthesize` stays WAV. Consumes no other task code.

- [ ] Write failing tests for optimize modes/deferred failure/readiness, pinned voice and bounded content-keyed feature cache.
- [ ] Run tests and observe missing behavior, implement runtime, rerun targeted tests.
- [ ] Write failing tests for incremental protocol, badcase policy, longform, bounded queues/disconnect/error/end.
- [ ] Implement provider streaming and preserve WAV metrics, update deployment/API documentation.
- [ ] Run `uv run --no-sync python -m pytest -q tests/http` and configuration checks; commit only TTS files. Provide test evidence and limitations.

### Task 2: Backend producer and authenticated stream

**Files:** `services/backend/internal/entities/audio`, voice model adapter, Geek speech/service/handler/ports, app wiring/config/tests, env/Compose, generated OpenAPI and backend README.

**Interfaces:** Consumes Task 1 protocol. Produces `GET /diagnostic-sessions/{id}/turns/{turn_id}/audio/stream?part=reply|question`, same NDJSON. `BACKEND_TTS_STREAMING_ENABLED=false`; disabled returns 406. Provider 404/406/501 before PCM falls back once to WAV. Existing diagnostic API remains supported.

- [ ] Write failing protocol/adapter tests including partial lines, invalid events, limits, cancelled context and old-provider fallback.
- [ ] Implement typed domain stream events/parser/PCM-WAV bridge without feature-to-feature imports.
- [ ] Write failing auth/cache/singleflight/cancellation tests and implement shared producer for WAV/stream/warmer; publish only complete WAV.
- [ ] Add handler flush, gate/config/profile identity, env/Compose documentation and regenerate OpenAPI twice to prove idempotence.
- [ ] Run `go test -race ./... -count=1`, `go vet ./...`, `go build -o bin/api ./cmd/api`, relevant Compose checks; commit only owned files. Report DB skips.

### Task 3: Browser stream player

**Files:** frontend diagnostic API/port, `use-turn-audio.ts`, focused shared PCM player/Worklet, corresponding tests and frontend README.

**Interfaces:** Consumes Task 2 protocol/route. New diagnostic stream capability can be optional for existing fake ports; no public model URL. WAV fallback only for unavailable APIs or HTTP 404/406/501 before audio.

- [ ] Write failing parser tests for fragmented lines, seq, end samples, malformed base64, error/EOF and 64 MiB/256 KiB limits.
- [ ] Implement incremental reader and stateful sample-rate conversion with bounded AudioWorklet queue (200 ms initial, 2 s maximum), short-stream finish and underrun behavior.
- [ ] Write failing integration tests for reply/question ordering, stop/replacement/recording, onStart, autoplay denial, cache replay and allowed fallback.
- [ ] Integrate player without buffering entire stream before playback; document support and test boundaries.
- [ ] Run formatting, lint, typecheck, Vitest and production build; commit only frontend files. Real browser/audio measurements remain an explicit release gate if unavailable.

### Task 4: Integration and acceptance artifacts

**Files:** reproducible 30-replica benchmark manifest/tool and results template, root README/checklist, proxy settings if needed, implementation report.

**Interfaces:** Uses Tasks 1–3 without changing their agreed wire format.

- [ ] Validate real backend against fake TTS incrementally and inspect proxy behavior/configuration; add missing focused integration regression tests before fixes.
- [ ] Provide reproducible baseline/variant measurement and quality-review instructions, clearly mark unavailable GPU/listening/browser checks.
- [ ] Run independent final review of cross-repo diff, fix important findings and rerun affected suites.
- [ ] Commit docs/report, verify both branches clean and list commits/checks/remaining release gates.
