CREATE TABLE IF NOT EXISTS incidents (
    id VARCHAR(64) PRIMARY KEY,
    sys_id VARCHAR(64),
    state VARCHAR(64) NOT NULL DEFAULT 'Назначен',
    state_enum VARCHAR(64) DEFAULT 'Assigned',
    state_id INT DEFAULT 0,
    state_reason TEXT,
    submit_date TIMESTAMPTZ,
    deadline TIMESTAMPTZ,
    completion_date TIMESTAMPTZ,
    last_assignment_to_team TIMESTAMPTZ,
    header TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    short_description TEXT,
    assigned_group_name TEXT NOT NULL DEFAULT '',
    assigned_employee TEXT,
    assigned_employee_login TEXT,
    initiator_name TEXT,
    initiator_login TEXT,
    priority_id INT NOT NULL DEFAULT 1,
    priority VARCHAR(64) NOT NULL DEFAULT 'Низкий',
    urgency_id INT DEFAULT 1,
    urgency VARCHAR(64) DEFAULT 'Низкая',
    impact_id INT DEFAULT 1,
    impact VARCHAR(64) DEFAULT 'Низкое',
    product_name TEXT,
    product_master_id TEXT,
    service TEXT,
    scenario_name TEXT,
    solution TEXT,
    service_type TEXT DEFAULT 'Технологический',
    service_type_id INT DEFAULT 30,
    is_effect BOOLEAN DEFAULT FALSE,
    is_reason BOOLEAN DEFAULT FALSE,
    is_double BOOLEAN DEFAULT FALSE,
    is_master BOOLEAN DEFAULT FALSE,
    ai_routing_hint TEXT DEFAULT '',
    ai_routing_confidence DOUBLE PRECISION DEFAULT 0.0,
    ai_sla_risk DOUBLE PRECISION DEFAULT 0.0,
    ai_sla_level VARCHAR(32) DEFAULT 'SAFE',
    ai_team_workload INT DEFAULT 0,
    ai_assignee_workload INT DEFAULT 0,
    ai_queue_remaining_minutes INT DEFAULT 0,
    ai_estimated_minutes INT DEFAULT 0,
    ai_estimated_min_minutes INT DEFAULT 0,
    ai_estimated_max_minutes INT DEFAULT 0,
    ai_history_median_minutes INT DEFAULT 0,
    ai_assignee_speed_factor DOUBLE PRECISION DEFAULT 1.0,
    ai_top_factors JSONB,
    ai_low_confidence BOOLEAN DEFAULT FALSE,
    ai_fallback_used BOOLEAN DEFAULT FALSE,
    ai_model_version VARCHAR(64) DEFAULT 'gbdt_reg_v1.0',
    ai_alt_assignee TEXT DEFAULT '',
    ai_alt_estimated_minutes INT DEFAULT 0,
    manual_override_minutes INT,
    manual_override_reason TEXT DEFAULT '',
    actual_duration_minutes INT,
    prediction_error_minutes INT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Safe migrations for existing databases
ALTER TABLE incidents DROP COLUMN IF EXISTS ai_risk_score;
ALTER TABLE incidents DROP COLUMN IF EXISTS ai_risk_level;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_assignee_workload INT DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_queue_remaining_minutes INT DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_estimated_min_minutes INT DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_estimated_max_minutes INT DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_history_median_minutes INT DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_assignee_speed_factor DOUBLE PRECISION DEFAULT 1.0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_top_factors JSONB;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_low_confidence BOOLEAN DEFAULT FALSE;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_fallback_used BOOLEAN DEFAULT FALSE;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_model_version VARCHAR(64) DEFAULT 'gbdt_reg_v1.0';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_alt_assignee TEXT DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS ai_alt_estimated_minutes INT DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS manual_override_minutes INT;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS manual_override_reason TEXT DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS actual_duration_minutes INT;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS prediction_error_minutes INT;

CREATE INDEX IF NOT EXISTS idx_incidents_priority ON incidents (priority_id);
CREATE INDEX IF NOT EXISTS idx_incidents_state ON incidents (state);
CREATE INDEX IF NOT EXISTS idx_incidents_group ON incidents (assigned_group_name);
CREATE INDEX IF NOT EXISTS idx_incidents_employee ON incidents (assigned_employee_login);
CREATE INDEX IF NOT EXISTS idx_incidents_updated_at ON incidents (updated_at);

CREATE TABLE IF NOT EXISTS incident_events (
    id SERIAL PRIMARY KEY,
    incident_id VARCHAR(64) NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    message TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_incident_id ON incident_events (incident_id);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON incident_events (created_at DESC);

CREATE TABLE IF NOT EXISTS ai_predictions (
    id SERIAL PRIMARY KEY,
    incident_id VARCHAR(64) NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    predicted_group VARCHAR(255),
    confidence DOUBLE PRECISION,
    top_candidates JSONB,
    predicted_minutes INT DEFAULT 0,
    predicted_sla_risk DOUBLE PRECISION DEFAULT 0.0,
    model_version VARCHAR(64) DEFAULT 'gbdt_reg_v2.0',
    actual_group VARCHAR(255),
    is_accepted BOOLEAN,
    input_features JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    evaluated_at TIMESTAMPTZ
);

ALTER TABLE ai_predictions DROP COLUMN IF EXISTS predicted_risk_score;
ALTER TABLE ai_predictions DROP COLUMN IF EXISTS predicted_risk_level;
ALTER TABLE ai_predictions ADD COLUMN IF NOT EXISTS predicted_minutes INT DEFAULT 0;
ALTER TABLE ai_predictions ADD COLUMN IF NOT EXISTS predicted_sla_risk DOUBLE PRECISION DEFAULT 0.0;
ALTER TABLE ai_predictions ADD COLUMN IF NOT EXISTS model_version VARCHAR(64) DEFAULT 'gbdt_reg_v2.0';

CREATE INDEX IF NOT EXISTS idx_ai_predictions_incident_id ON ai_predictions (incident_id);
CREATE INDEX IF NOT EXISTS idx_ai_predictions_created_at ON ai_predictions (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_predictions_is_accepted ON ai_predictions (is_accepted);
