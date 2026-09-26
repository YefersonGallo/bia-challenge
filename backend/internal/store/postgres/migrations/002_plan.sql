-- Plan phase 1/3/6: extra anomaly fields (kept together as JSONB) and operator actions.
ALTER TABLE anomalies ADD COLUMN IF NOT EXISTS details JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS anomaly_actions (
    id          TEXT PRIMARY KEY,
    anomaly_id  TEXT NOT NULL REFERENCES anomalies(id) ON DELETE CASCADE,
    action      TEXT NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT '',
    actor       TEXT NOT NULL DEFAULT '',
    at          TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS anomaly_actions_anomaly_idx ON anomaly_actions (anomaly_id, at);
