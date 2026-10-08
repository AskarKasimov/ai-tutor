-- +goose Up
CREATE TABLE audio_assets (
    id text PRIMARY KEY,
    instruction text NOT NULL CHECK (char_length(instruction) BETWEEN 1 AND 500 AND instruction ~ '[^[:space:]]'),
    object_key text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'failed', 'cancelled')),
    bucket text,
    storage_uri text,
    audio_url text,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    claim_token text,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'ready' AND bucket IS NOT NULL AND storage_uri IS NOT NULL AND audio_url IS NOT NULL)
        OR (status <> 'ready' AND bucket IS NULL AND storage_uri IS NULL AND audio_url IS NULL)),
    CHECK ((status = 'processing' AND lease_until IS NOT NULL AND claim_token IS NOT NULL)
        OR (status <> 'processing' AND lease_until IS NULL AND claim_token IS NULL))
);
CREATE INDEX audio_assets_pending_queue ON audio_assets(next_attempt_at, created_at, id)
    WHERE status = 'pending';
CREATE INDEX audio_assets_expired_leases ON audio_assets(lease_until)
    WHERE status = 'processing';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT source_id
        FROM (
            SELECT id AS source_id, voice_instruction AS instruction
            FROM tasks
            WHERE voice_instruction IS NOT NULL
            UNION ALL
            SELECT source_task_id_snapshot AS source_id,
                   task_snapshot->>'voice_instruction' AS instruction
            FROM variant_tasks
            WHERE task_snapshot->>'voice_instruction' IS NOT NULL
        ) instructions
        GROUP BY source_id
        HAVING count(DISTINCT instruction) > 1
    ) THEN
        RAISE EXCEPTION 'task audio migration found conflicting instructions for a source task';
    END IF;
END $$;
-- +goose StatementEnd

INSERT INTO audio_assets(id, instruction, object_key)
SELECT 'taskaudio_' || task.id, task.voice_instruction,
       'task-audio/v1/taskaudio_' || task.id || '.wav'
FROM tasks task
WHERE task.voice_instruction IS NOT NULL
  AND char_length(task.voice_instruction) BETWEEN 1 AND 500
  AND task.voice_instruction ~ '[^[:space:]]'
ON CONFLICT (id) DO NOTHING;

INSERT INTO audio_assets(id, instruction, object_key)
SELECT DISTINCT ON (variant_task.source_task_id_snapshot)
       'taskaudio_' || variant_task.source_task_id_snapshot,
       variant_task.task_snapshot->>'voice_instruction',
       'task-audio/v1/taskaudio_' || variant_task.source_task_id_snapshot || '.wav'
FROM variant_tasks variant_task
WHERE variant_task.task_snapshot->>'voice_instruction' IS NOT NULL
  AND char_length(variant_task.task_snapshot->>'voice_instruction') BETWEEN 1 AND 500
  AND variant_task.task_snapshot->>'voice_instruction' ~ '[^[:space:]]'
ORDER BY variant_task.source_task_id_snapshot
ON CONFLICT (id) DO NOTHING;

ALTER TABLE tasks ADD COLUMN audio_asset_id text REFERENCES audio_assets(id) ON DELETE SET NULL;
ALTER TABLE variant_tasks ADD COLUMN audio_asset_id text REFERENCES audio_assets(id) ON DELETE SET NULL;
UPDATE tasks SET audio_asset_id = 'taskaudio_' || id
WHERE EXISTS (SELECT 1 FROM audio_assets asset WHERE asset.id = 'taskaudio_' || tasks.id);
UPDATE variant_tasks SET audio_asset_id = 'taskaudio_' || source_task_id_snapshot
WHERE EXISTS (SELECT 1 FROM audio_assets asset WHERE asset.id = 'taskaudio_' || variant_tasks.source_task_id_snapshot);
CREATE INDEX tasks_audio_asset ON tasks(audio_asset_id) WHERE audio_asset_id IS NOT NULL;
CREATE INDEX variant_tasks_audio_asset ON variant_tasks(audio_asset_id) WHERE audio_asset_id IS NOT NULL;

-- Snapshot-only work outside the active map must not consume worker capacity.
UPDATE audio_assets asset
SET status = 'cancelled', last_error_code = 'task_no_longer_current', updated_at = now()
WHERE asset.status = 'pending'
  AND NOT EXISTS (
      SELECT 1
      FROM tasks task
      JOIN outcomes outcome ON outcome.id = task.outcome_id
      JOIN constituents constituent ON constituent.id = outcome.constituent_id
      JOIN competencies competency ON competency.id = constituent.competency_id
      JOIN competency_map_state state ON state.singleton = true AND state.revision = competency.revision
      WHERE task.audio_asset_id = asset.id
  );

-- +goose Down
DROP INDEX IF EXISTS variant_tasks_audio_asset;
DROP INDEX IF EXISTS tasks_audio_asset;
ALTER TABLE variant_tasks DROP COLUMN IF EXISTS audio_asset_id;
ALTER TABLE tasks DROP COLUMN IF EXISTS audio_asset_id;
DROP TABLE IF EXISTS audio_assets;
