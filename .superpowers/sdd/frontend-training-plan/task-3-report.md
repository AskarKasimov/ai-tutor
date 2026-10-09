# Task 3 report: subject home and explicit diagnostic lifecycle

## Implemented

- Added the subject-selection feature provider and hooks for the subject catalog, selected-subject learning state, and subject creation. Queries use the entity's stable subject keys, include user/subject IDs, register authenticated requests, abort on session changes, and clear the session on 401.
- Added the student home with explicit subject selection, catalog/loading/error/empty states, selected-subject learning state, diagnostic continue/start actions, and a localized training placeholder gated by completed diagnosis.
- The home mounts the diagnostic screen only after start/continue. Start is the only initial POST path; repeated clicks are guarded. Continue and page refresh restore an existing session ID and perform GET. The page intent is scoped to API base, user, and subject and is cleared when returning to selection.
- Added `subjectId` to diagnostic identity, variant creation, and storage interfaces. Session storage keys now include API base, user ID, and subject ID; older identities without subject ID are ignored. A pending start restores its idempotency keys and variant ID after a refresh.
- Updated the diagnostic screen to use catalog subject names, return to subject selection after completion, and keep all callers on the required signatures.
- Removed the mock's missing-subject fallback. Mock learning state now exposes a later active diagnostic while preserving the latest completed diagnostic and training eligibility.
- Preserved the existing `legacy-demo` route behavior. Updated existing diagnostic route tests to select a subject and explicitly start, and adjusted the mock audio test to create an owned variant/session first.

## TDD evidence

- RED: `npm test -- --run tests/features/diagnostic-session/model/start-session.test.ts` failed all 3 tests immediately after requiring a subject-scoped identity/storage and `createVariant(subjectId, key, signal)`. Failures showed existing fixtures/callers still used the prior identity and port signatures.
- GREEN: updated the fixtures and ports; those tests pass in the focused and full runs below.
- Added route/API/storage regression coverage for no variant POST before explicit start, no learning-state read before subject selection, one variant after a double click, refresh by GET without another variant POST, explicit subject in the variant JSON body, scoped/legacy identity handling, and training availability while a later diagnostic is active.

## Files changed

- Bootstrap composition and mock behavior: `services/frontend/src/bootstrap/{dependencies.ts,providers.tsx,mock-api.ts,mock-diagnostic-api.ts,mock-diagnostic-storage.ts}`.
- Diagnostic subject/identity lifecycle and API: `services/frontend/src/entities/diagnostic-session/{api/diagnostic-api.ts,model/session-identity.ts}` and `services/frontend/src/features/diagnostic-session/model/{dependencies-context.tsx,diagnostic-identity.ts,diagnostic-session-storage.ts,start-session.ts,use-diagnostic-session.ts}`.
- New subject-selection slice: `services/frontend/src/features/subject-selection/**`.
- Home and diagnostic UI: `services/frontend/src/pages/trainer/{index.ts,ui/diagnostic-trainer.tsx,ui/trainer-screen.tsx,ui/learning-home.tsx,ui/learning-home.module.scss}` and `services/frontend/src/shared/i18n/i18n.ts`.
- Tests: subject route, diagnostic route/session/API/storage tests, mock-learning and mock API interaction tests, and shared test setup for Radix browser APIs.

## Checks

- Focused route run after the selection/refresh changes: 3 files / 17 tests passed. The diagnostic API/session/mock groups also passed (10 files / 78 tests).
- `npm run format:check` — passed.
- `npm run lint` — passed.
- `npm run typecheck` — passed.
- `npm test -- --run` — passed, 29 files / 180 tests. jsdom emitted its existing `HTMLMediaElement.pause/load()` not-implemented notices in audio tests.
- `npm run build` — passed (Vite production build).
- `git diff --check` — passed.

## Self-review / concerns

- No known functional concerns. The available demo catalog has one subject; multi-subject selection is covered through the selected ID contract and UI control, while multi-subject catalog data requires backend/mock fixture support from a future task.

## Commit

- `5f4ca12 feat(frontend): select subjects and explicitly start diagnostics`.
