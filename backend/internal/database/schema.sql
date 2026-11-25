-- Normalized 3NF PostgreSQL Schema with UUID Primary & Foreign Keys

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Auto-reset legacy unnormalized VARCHAR(64) incidents table if present
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'incidents'
          AND column_name = 'id'
          AND data_type <> 'uuid'
    ) THEN
        DROP TABLE IF EXISTS ai_predictions, incident_events, incident_forecasts, incidents, services, teams CASCADE;
    END IF;
END $$;

-- 1. Команды решения (Teams)
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    is_coordinator BOOLEAN NOT NULL DEFAULT FALSE,
    historical_median_minutes INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Справочник продуктов и ИТ-сервисов (Services)
CREATE TABLE IF NOT EXISTS services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_name VARCHAR(255) NOT NULL,
    product_master_id VARCHAR(128) NOT NULL DEFAULT '',
    service_name VARCHAR(255) NOT NULL,
    service_type VARCHAR(128) NOT NULL DEFAULT 'Технологический',
    service_type_id INT NOT NULL DEFAULT 30,
    scenario_name VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_name, service_name)
);

-- 3. Заявки / Инциденты (Incidents)
CREATE TABLE IF NOT EXISTS incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_number VARCHAR(64) NOT NULL UNIQUE,
    external_sys_id VARCHAR(64) DEFAULT '',
    team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    service_id UUID REFERENCES services(id) ON DELETE SET NULL,
    state VARCHAR(64) NOT NULL DEFAULT 'Зарегистрирован',
    state_enum VARCHAR(64) DEFAULT 'Registered',
    state_id INT NOT NULL DEFAULT 0,
    state_reason TEXT DEFAULT '',
    priority_id INT NOT NULL DEFAULT 1,
    priority VARCHAR(64) NOT NULL DEFAULT 'Низкий',
    urgency_id INT NOT NULL DEFAULT 1,
    urgency VARCHAR(64) DEFAULT 'Низкая',
    impact_id INT NOT NULL DEFAULT 1,
    impact VARCHAR(64) DEFAULT 'Низкое',
    header TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    short_description TEXT DEFAULT '',
    initiator_name VARCHAR(255) DEFAULT '',
    initiator_login VARCHAR(128) DEFAULT '',
    solution TEXT DEFAULT '',
    is_effect BOOLEAN NOT NULL DEFAULT FALSE,
    is_reason BOOLEAN NOT NULL DEFAULT FALSE,
    is_double BOOLEAN NOT NULL DEFAULT FALSE,
    is_master BOOLEAN NOT NULL DEFAULT FALSE,
    submit_date TIMESTAMPTZ,
    deadline TIMESTAMPTZ,
    completion_date TIMESTAMPTZ,
    last_assignment_to_team TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. Актуальный ML-прогноз времени, риска SLA и корректировка (1:1 к incidents)
CREATE TABLE IF NOT EXISTS incident_forecasts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL UNIQUE REFERENCES incidents(id) ON DELETE CASCADE,
    recommended_team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    routing_confidence DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    sla_risk DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    sla_level VARCHAR(32) NOT NULL DEFAULT 'SAFE',
    team_workload INT NOT NULL DEFAULT 0,
    queue_remaining_minutes INT NOT NULL DEFAULT 0,
    estimated_minutes INT NOT NULL DEFAULT 0,
    estimated_min_minutes INT NOT NULL DEFAULT 0,
    estimated_max_minutes INT NOT NULL DEFAULT 0,
    history_median_minutes INT NOT NULL DEFAULT 0,
    sla_margin_minutes INT NOT NULL DEFAULT 0,
    top_factors JSONB,
    routing_candidates JSONB,
    low_confidence BOOLEAN NOT NULL DEFAULT FALSE,
    fallback_used BOOLEAN NOT NULL DEFAULT FALSE,
    model_version VARCHAR(64) NOT NULL DEFAULT 'gbdt_rubert_v3.0',
    manual_override_minutes INT,
    manual_override_reason TEXT DEFAULT '',
    actual_duration_minutes INT,
    prediction_error_minutes INT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. Журнал аудита переходов и действий (1:N к incidents)
CREATE TABLE IF NOT EXISTS incident_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    message TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 6. История ML-прогнозов и feedback по выбору команды (1:N к incidents)
CREATE TABLE IF NOT EXISTS ai_predictions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    predicted_team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    actual_team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    confidence DOUBLE PRECISION,
    top_candidates JSONB,
    predicted_minutes INT NOT NULL DEFAULT 0,
    predicted_sla_risk DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    model_version VARCHAR(64) NOT NULL DEFAULT 'gbdt_rubert_v3.0',
    is_accepted BOOLEAN,
    input_features JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    evaluated_at TIMESTAMPTZ
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_teams_name ON teams (name);
CREATE INDEX IF NOT EXISTS idx_services_product ON services (product_name);
CREATE INDEX IF NOT EXISTS idx_incidents_ticket_number ON incidents (ticket_number);
CREATE INDEX IF NOT EXISTS idx_incidents_team_id ON incidents (team_id);
CREATE INDEX IF NOT EXISTS idx_incidents_service_id ON incidents (service_id);
CREATE INDEX IF NOT EXISTS idx_incidents_priority ON incidents (priority_id);
CREATE INDEX IF NOT EXISTS idx_incidents_state ON incidents (state);
CREATE INDEX IF NOT EXISTS idx_incidents_updated_at ON incidents (updated_at);
CREATE INDEX IF NOT EXISTS idx_forecasts_incident_id ON incident_forecasts (incident_id);
CREATE INDEX IF NOT EXISTS idx_forecasts_recommended_team ON incident_forecasts (recommended_team_id);
CREATE INDEX IF NOT EXISTS idx_events_incident_id ON incident_events (incident_id);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON incident_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_predictions_incident_id ON ai_predictions (incident_id);
