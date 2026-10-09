# Task 2 report: mock subject and training APIs

## Result

Implemented the mock transport for the single demo subject `subject:intro-to-ml` (`Введение в ML`) and infinite training endpoints. Mock diagnostic variants retain subject ownership; omission of `subject_id` remains compatible with the pre-Task 3 diagnostic client, while an explicit unknown subject returns 404. Subject and diagnostic/training state is scoped by mock user identity, and login/logout clears mutable learning data.

Training supports completed-diagnostic gating, focused preview targets for confirmed gaps and partial competencies, focused/free-practice starts, active-session reads, demo graded attempts and transcripts, idempotent same-body retries, conflicting key reuse, fresh exercises and rounds, history cursors, and demo audio. Starting a later active diagnostic does not hide an earlier completed diagnostic in learning state.

## Files

- `services/frontend/src/bootstrap/mock-api.ts`
- `services/frontend/src/bootstrap/mock-diagnostic-api.ts`
- `services/frontend/src/bootstrap/mock-training-api.ts`
- `services/frontend/tests/bootstrap/mock-learning.test.ts`

## Checks

- `npm test -- --run tests/bootstrap/mock-learning.test.ts` — passed, 5 tests.
- `npm run typecheck` — passed.
- `npm test -- --run` — passed, 27 files / 176 tests.
- `npm run format:check` — passed.
- `npm run lint` — passed.
- `npm run build` — passed.

The focused test was added after the initial mock implementation, so a pre-implementation RED run was not captured. An initial test attempt used the suite's default real API mode and failed at transport setup; after explicitly enabling mock mode, the focused test passed.

## Follow-up ownership and replay fix

Diagnostic session endpoints now enforce the creating mock user's ownership, and diagnostic current-audio routes require an owned session. Training answer replays compare the exercise ID and audio filename, MIME type, and bytes, so a newly constructed equivalent FormData succeeds while changed audio under the same key returns 409.

- `npm test -- --run tests/bootstrap/mock-learning.test.ts` — passed, 6 tests, including cross-user diagnostic access and reconstructed replay payloads.
- `npm run typecheck` — passed.
- `git diff --check` — passed.
