CREATE TABLE IF NOT EXISTS meters (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    location    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS readings (
    meter_id         TEXT NOT NULL REFERENCES meters(id),
    ts               TIMESTAMPTZ NOT NULL,
    consumption_kwh  DOUBLE PRECISION NOT NULL,
    voltage_v        DOUBLE PRECISION NOT NULL,
    current_a        DOUBLE PRECISION NOT NULL,
    power_factor     DOUBLE PRECISION NOT NULL,
    status           TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (meter_id, ts)
);

CREATE TABLE IF NOT EXISTS events (
    id           TEXT PRIMARY KEY,
    meter_id     TEXT NOT NULL REFERENCES meters(id),
    ts           TIMESTAMPTZ NOT NULL,
    type         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS events_meter_idx ON events (meter_id);

CREATE TABLE IF NOT EXISTS analysis_runs (
    id            TEXT PRIMARY KEY,
    status        TEXT NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    current_step  INT NOT NULL DEFAULT 0,
    steps         JSONB NOT NULL,
    summary       JSONB,
    error         TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS anomalies (
    id                  TEXT PRIMARY KEY,
    analysis_id         TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
    meter_id            TEXT NOT NULL REFERENCES meters(id),
    detected_at         TIMESTAMPTZ NOT NULL,
    type                TEXT NOT NULL,
    severity            TEXT NOT NULL,
    confidence          DOUBLE PRECISION NOT NULL,
    priority_score      DOUBLE PRECISION NOT NULL,
    rank                INT NOT NULL,
    reason              TEXT NOT NULL,
    recommended_action  TEXT NOT NULL,
    next_steps          JSONB NOT NULL,
    explained_by        TEXT NOT NULL,
    status              TEXT NOT NULL,
    evidence            JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS anomalies_analysis_idx ON anomalies (analysis_id);
