package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"itsm-dashboard/backend/internal/ml"
	"itsm-dashboard/backend/internal/models"
)

type DB struct {
	Conn *sql.DB
}

func Connect(connStr string) (*DB, error) {
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/itsm_dashboard?sslmode=disable"
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("[DB] Connected to PostgreSQL successfully")
	return &DB{Conn: db}, nil
}

func (db *DB) InitSchema(schemaPath string) error {
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		schemaPath = filepath.Join("internal", "database", "schema.sql")
		content, err = os.ReadFile(schemaPath)
		if err != nil {
			return fmt.Errorf("could not read schema file: %w", err)
		}
	}

	_, err = db.Conn.Exec(string(content))
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	log.Println("[DB] Schema initialized successfully")
	return nil
}

func medianFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]float64, len(vals))
	copy(cp, vals)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2.0
}

// GetRuntimeContext aggregates active queues, historical 90-day medians, and specialist speed factors.
func (db *DB) GetRuntimeContext() (*ml.RuntimeContext, error) {
	ctx := &ml.RuntimeContext{
		TeamWorkloads:        make(map[string]int),
		TeamQueueMinutes:     make(map[string]int),
		AssigneeWorkloads:    make(map[string]int),
		AssigneeQueueMinutes: make(map[string]int),
		HistoryMedians:       make(map[string]float64),
		HistoryCounts:        make(map[string]int),
		AssigneeSpeedFactors: make(map[string]float64),
		GroupEmployees:       make(map[string][]ml.EmployeeCandidate),
	}

	// 1. Active queue workloads per team and employee
	qActive := `
		SELECT assigned_group_name, COALESCE(assigned_employee_login, ''), COALESCE(assigned_employee, ''), COALESCE(ai_estimated_minutes, 90)
		FROM incidents
		WHERE state NOT IN ('Завершен', 'Закрыт')
	`
	if rows, err := db.Conn.Query(qActive); err == nil {
		defer rows.Close()
		for rows.Next() {
			var grp, empLogin, empName string
			var estMin int
			if err := rows.Scan(&grp, &empLogin, &empName, &estMin); err == nil {
				if estMin <= 0 {
					estMin = 90
				}
				if grp != "" {
					ctx.TeamWorkloads[grp]++
					ctx.TeamQueueMinutes[grp] += estMin
				}
				key := strings.ToLower(strings.TrimSpace(empLogin))
				if key == "" {
					key = strings.ToLower(strings.TrimSpace(empName))
				}
				if key != "" {
					ctx.AssigneeWorkloads[key]++
					ctx.AssigneeQueueMinutes[key] += estMin
				}
			}
		}
	}

	// 2. Historical durations over closed tickets (for category/group medians & speed factors)
	qHist := `
		SELECT assigned_group_name, COALESCE(product_name, ''), priority_id,
		       COALESCE(assigned_employee_login, ''), COALESCE(assigned_employee, ''),
		       EXTRACT(EPOCH FROM (completion_date - submit_date)) / 3600.0 AS dur_hours
		FROM incidents
		WHERE completion_date IS NOT NULL AND submit_date IS NOT NULL
		  AND completion_date > submit_date
	`
	gpBuckets := make(map[string][]float64)
	gBuckets := make(map[string][]float64)
	pBuckets := make(map[string][]float64)
	empBuckets := make(map[string][]float64)
	empInfo := make(map[string]struct{ name, group string })
	var allDurs []float64

	if rows, err := db.Conn.Query(qHist); err == nil {
		defer rows.Close()
		for rows.Next() {
			var grp, prod, empLogin, empName string
			var prio int
			var dur float64
			if err := rows.Scan(&grp, &prod, &prio, &empLogin, &empName, &dur); err == nil && dur > 0.05 && dur < 200 {
				allDurs = append(allDurs, dur)
				if grp != "" {
					gpKey := fmt.Sprintf("gp|%s|%d", grp, prio)
					gKey := fmt.Sprintf("g|%s", grp)
					gpBuckets[gpKey] = append(gpBuckets[gpKey], dur)
					gBuckets[gKey] = append(gBuckets[gKey], dur)
				}
				if prod != "" {
					pKey := fmt.Sprintf("p|%s", prod)
					pBuckets[pKey] = append(pBuckets[pKey], dur)
				}
				ek := strings.ToLower(strings.TrimSpace(empLogin))
				if ek == "" {
					ek = strings.ToLower(strings.TrimSpace(empName))
				}
				if ek != "" {
					empBuckets[ek] = append(empBuckets[ek], dur)
					if empName != "" {
						empInfo[ek] = struct{ name, group string }{name: empName, group: grp}
					}
				}
			}
		}
	}

	globalMed := medianFloat64(allDurs)
	if globalMed <= 0 {
		globalMed = 1.8
	}
	ctx.HistoryMedians["global"] = globalMed
	ctx.HistoryCounts["global"] = len(allDurs)

	for k, vals := range gpBuckets {
		ctx.HistoryMedians[k] = medianFloat64(vals)
		ctx.HistoryCounts[k] = len(vals)
	}
	for k, vals := range gBuckets {
		ctx.HistoryMedians[k] = medianFloat64(vals)
		ctx.HistoryCounts[k] = len(vals)
	}
	for k, vals := range pBuckets {
		ctx.HistoryMedians[k] = medianFloat64(vals)
		ctx.HistoryCounts[k] = len(vals)
	}

	// Build specialist speed factors and candidate lists per group
	seenEmpPerGroup := make(map[string]map[string]bool)
	qEmps := `
		SELECT DISTINCT assigned_group_name, COALESCE(assigned_employee, ''), COALESCE(assigned_employee_login, '')
		FROM incidents
		WHERE assigned_group_name != '' AND (assigned_employee != '' OR assigned_employee_login != '')
	`
	if rows, err := db.Conn.Query(qEmps); err == nil {
		defer rows.Close()
		for rows.Next() {
			var grp, name, login string
			if err := rows.Scan(&grp, &name, &login); err == nil {
				ek := strings.ToLower(strings.TrimSpace(login))
				if ek == "" {
					ek = strings.ToLower(strings.TrimSpace(name))
				}
				if ek == "" {
					continue
				}
				sf := 1.0
				if durs := empBuckets[ek]; len(durs) >= 2 && globalMed > 0 {
					sf = math.Max(0.65, math.Min(1.50, medianFloat64(durs)/globalMed))
				}
				ctx.AssigneeSpeedFactors[ek] = sf

				if seenEmpPerGroup[grp] == nil {
					seenEmpPerGroup[grp] = make(map[string]bool)
				}
				if !seenEmpPerGroup[grp][ek] {
					seenEmpPerGroup[grp][ek] = true
					dispName := name
					if dispName == "" {
						dispName = login
					}
					ctx.GroupEmployees[grp] = append(ctx.GroupEmployees[grp], ml.EmployeeCandidate{
						Name:       dispName,
						Login:      login,
						Group:      grp,
						Workload:   ctx.AssigneeWorkloads[ek],
						QueueMin:   ctx.AssigneeQueueMinutes[ek],
						SpeedRatio: sf,
					})
				}
			}
		}
	}

	return ctx, nil
}

func (db *DB) UpsertIncident(inc *models.Incident) error {
	ctx, _ := db.GetRuntimeContext()

	// Evaluate unified Regression ML enrichments (Team Routing, Time Prediction, SLA Breach Risk)
	ml.GetEngine().Evaluate(inc, ctx)

	factorsJSON, _ := json.Marshal(inc.AITopFactors)

	query := `
		INSERT INTO incidents (
			id, sys_id, state, state_enum, state_id, state_reason,
			submit_date, deadline, completion_date, last_assignment_to_team,
			header, description, short_description, assigned_group_name,
			assigned_employee, assigned_employee_login, initiator_name, initiator_login,
			priority_id, priority, urgency_id, urgency, impact_id, impact,
			product_name, product_master_id, service, scenario_name, solution,
			service_type, service_type_id, is_effect, is_reason, is_double, is_master,
			ai_routing_hint, ai_routing_confidence,
			ai_sla_risk, ai_sla_level, ai_team_workload, ai_assignee_workload,
			ai_queue_remaining_minutes, ai_estimated_minutes, ai_estimated_min_minutes,
			ai_estimated_max_minutes, ai_history_median_minutes, ai_assignee_speed_factor,
			ai_top_factors, ai_low_confidence, ai_fallback_used, ai_model_version,
			ai_alt_assignee, ai_alt_estimated_minutes,
			manual_override_minutes, manual_override_reason,
			actual_duration_minutes, prediction_error_minutes, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24,
			$25, $26, $27, $28, $29,
			$30, $31, $32, $33, $34, $35,
			$36, $37,
			$38, $39, $40, $41,
			$42, $43, $44,
			$45, $46, $47,
			$48, $49, $50, $51,
			$52, $53,
			$54, $55,
			$56, $57, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			state_enum = EXCLUDED.state_enum,
			state_id = EXCLUDED.state_id,
			state_reason = EXCLUDED.state_reason,
			deadline = EXCLUDED.deadline,
			completion_date = EXCLUDED.completion_date,
			last_assignment_to_team = EXCLUDED.last_assignment_to_team,
			header = EXCLUDED.header,
			description = EXCLUDED.description,
			short_description = EXCLUDED.short_description,
			assigned_group_name = EXCLUDED.assigned_group_name,
			assigned_employee = EXCLUDED.assigned_employee,
			assigned_employee_login = EXCLUDED.assigned_employee_login,
			priority_id = EXCLUDED.priority_id,
			priority = EXCLUDED.priority,
			urgency_id = EXCLUDED.urgency_id,
			urgency = EXCLUDED.urgency,
			impact_id = EXCLUDED.impact_id,
			impact = EXCLUDED.impact,
			product_name = EXCLUDED.product_name,
			solution = EXCLUDED.solution,
			ai_routing_hint = EXCLUDED.ai_routing_hint,
			ai_routing_confidence = EXCLUDED.ai_routing_confidence,
			ai_sla_risk = EXCLUDED.ai_sla_risk,
			ai_sla_level = EXCLUDED.ai_sla_level,
			ai_team_workload = EXCLUDED.ai_team_workload,
			ai_assignee_workload = EXCLUDED.ai_assignee_workload,
			ai_queue_remaining_minutes = EXCLUDED.ai_queue_remaining_minutes,
			ai_estimated_minutes = EXCLUDED.ai_estimated_minutes,
			ai_estimated_min_minutes = EXCLUDED.ai_estimated_min_minutes,
			ai_estimated_max_minutes = EXCLUDED.ai_estimated_max_minutes,
			ai_history_median_minutes = EXCLUDED.ai_history_median_minutes,
			ai_assignee_speed_factor = EXCLUDED.ai_assignee_speed_factor,
			ai_top_factors = EXCLUDED.ai_top_factors,
			ai_low_confidence = EXCLUDED.ai_low_confidence,
			ai_fallback_used = EXCLUDED.ai_fallback_used,
			ai_model_version = EXCLUDED.ai_model_version,
			ai_alt_assignee = EXCLUDED.ai_alt_assignee,
			ai_alt_estimated_minutes = EXCLUDED.ai_alt_estimated_minutes,
			manual_override_minutes = EXCLUDED.manual_override_minutes,
			manual_override_reason = EXCLUDED.manual_override_reason,
			actual_duration_minutes = EXCLUDED.actual_duration_minutes,
			prediction_error_minutes = EXCLUDED.prediction_error_minutes,
			updated_at = NOW()
	`

	_, err := db.Conn.Exec(
		query,
		inc.ID, inc.SysID, inc.State, inc.StateEnum, inc.StateID, inc.StateReason,
		inc.SubmitDate, inc.Deadline, inc.CompletionDate, inc.LastAssignmentToTeamDateTime,
		inc.Header, inc.Description, inc.ShortDescription, inc.AssignedGroupName,
		inc.AssignedEmployee, inc.AssignedEmployeeLogin, inc.InitiatorName, inc.InitiatorLogin,
		inc.PriorityID, inc.Priority, inc.UrgencyID, inc.Urgency, inc.ImpactID, inc.Impact,
		inc.ProductName, inc.ProductMasterID, inc.Service, inc.ScenarioName, inc.Solution,
		inc.ServiceType, inc.ServiceTypeID, inc.IsEffect, inc.IsReason, inc.IsDouble, inc.IsMaster,
		inc.AIRoutingHint, inc.AIRoutingConfidence,
		inc.AISLARisk, inc.AISLALevel, inc.AITeamWorkload, inc.AIAssigneeWorkload,
		inc.AIQueueRemainingMinutes, inc.AIEstimatedMinutes, inc.AIEstimatedMinMinutes,
		inc.AIEstimatedMaxMinutes, inc.AIHistoryMedianMinutes, inc.AIAssigneeSpeedFactor,
		factorsJSON, inc.AILowConfidence, inc.AIFallbackUsed, inc.AIModelVersion,
		inc.AIAltAssignee, inc.AIAltEstimatedMinutes,
		inc.ManualOverrideMinutes, inc.ManualOverrideReason,
		inc.ActualDurationMinutes, inc.PredictionErrorMinutes,
	)

	return err
}

const incidentSelectCols = `
	id, sys_id, state, state_enum, state_id, state_reason,
	submit_date, deadline, completion_date, last_assignment_to_team,
	header, description, short_description, assigned_group_name,
	assigned_employee, assigned_employee_login, initiator_name, initiator_login,
	priority_id, priority, urgency_id, urgency, impact_id, impact,
	product_name, product_master_id, service, scenario_name, solution,
	service_type, service_type_id, is_effect, is_reason, is_double, is_master,
	ai_routing_hint, ai_routing_confidence,
	ai_sla_risk, ai_sla_level, ai_team_workload, COALESCE(ai_assignee_workload, 0),
	COALESCE(ai_queue_remaining_minutes, 0), ai_estimated_minutes,
	COALESCE(ai_estimated_min_minutes, 0), COALESCE(ai_estimated_max_minutes, 0),
	COALESCE(ai_history_median_minutes, 0), COALESCE(ai_assignee_speed_factor, 1.0),
	ai_top_factors, COALESCE(ai_low_confidence, false), COALESCE(ai_fallback_used, false),
	COALESCE(ai_model_version, 'gbdt_reg_v2.0'),
	COALESCE(ai_alt_assignee, ''), COALESCE(ai_alt_estimated_minutes, 0),
	manual_override_minutes, COALESCE(manual_override_reason, ''),
	actual_duration_minutes, prediction_error_minutes,
	created_at, updated_at
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIncident(rs rowScanner) (*models.Incident, error) {
	var inc models.Incident
	var submitDate, deadline, completionDate, lastAssignment *time.Time
	var sysID, stateReason, shortDesc, emp, empLogin, initName, initLogin sql.NullString
	var prodName, prodMaster, srv, scen, sol, srvType sql.NullString
	var topFactorsRaw sql.NullString
	var manOverrideMin, actDurMin, predErrMin sql.NullInt64

	err := rs.Scan(
		&inc.ID, &sysID, &inc.State, &inc.StateEnum, &inc.StateID, &stateReason,
		&submitDate, &deadline, &completionDate, &lastAssignment,
		&inc.Header, &inc.Description, &shortDesc, &inc.AssignedGroupName,
		&emp, &empLogin, &initName, &initLogin,
		&inc.PriorityID, &inc.Priority, &inc.UrgencyID, &inc.Urgency, &inc.ImpactID, &inc.Impact,
		&prodName, &prodMaster, &srv, &scen, &sol,
		&srvType, &inc.ServiceTypeID, &inc.IsEffect, &inc.IsReason, &inc.IsDouble, &inc.IsMaster,
		&inc.AIRoutingHint, &inc.AIRoutingConfidence,
		&inc.AISLARisk, &inc.AISLALevel, &inc.AITeamWorkload, &inc.AIAssigneeWorkload,
		&inc.AIQueueRemainingMinutes, &inc.AIEstimatedMinutes,
		&inc.AIEstimatedMinMinutes, &inc.AIEstimatedMaxMinutes,
		&inc.AIHistoryMedianMinutes, &inc.AIAssigneeSpeedFactor,
		&topFactorsRaw, &inc.AILowConfidence, &inc.AIFallbackUsed,
		&inc.AIModelVersion,
		&inc.AIAltAssignee, &inc.AIAltEstimatedMinutes,
		&manOverrideMin, &inc.ManualOverrideReason,
		&actDurMin, &predErrMin,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	inc.SysID = sysID.String
	inc.StateReason = stateReason.String
	inc.ShortDescription = shortDesc.String
	inc.AssignedEmployee = emp.String
	inc.AssignedEmployeeLogin = empLogin.String
	inc.InitiatorName = initName.String
	inc.InitiatorLogin = initLogin.String
	inc.ProductName = prodName.String
	inc.ProductMasterID = prodMaster.String
	inc.Service = srv.String
	inc.ScenarioName = scen.String
	inc.Solution = sol.String
	inc.ServiceType = srvType.String

	inc.SubmitDate = submitDate
	inc.Deadline = deadline
	inc.CompletionDate = completionDate
	inc.LastAssignmentToTeamDateTime = lastAssignment

	if topFactorsRaw.Valid && topFactorsRaw.String != "" {
		_ = json.Unmarshal([]byte(topFactorsRaw.String), &inc.AITopFactors)
	}
	if manOverrideMin.Valid {
		v := int(manOverrideMin.Int64)
		inc.ManualOverrideMinutes = &v
	}
	if actDurMin.Valid {
		v := int(actDurMin.Int64)
		inc.ActualDurationMinutes = &v
	}
	if predErrMin.Valid {
		v := int(predErrMin.Int64)
		inc.PredictionErrorMinutes = &v
	}

	return &inc, nil
}

func (db *DB) GetIncidents() ([]*models.Incident, error) {
	ctx, _ := db.GetRuntimeContext()

	query := fmt.Sprintf(`
		SELECT %s
		FROM incidents
		ORDER BY priority_id DESC, ai_sla_risk DESC, updated_at DESC
	`, incidentSelectCols)

	rows, err := db.Conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []*models.Incident
	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		ml.GetEngine().Evaluate(inc, ctx)
		incidents = append(incidents, inc)
	}

	return incidents, nil
}

func (db *DB) GetIncidentByID(id string) (*models.Incident, error) {
	ctx, _ := db.GetRuntimeContext()

	query := fmt.Sprintf(`
		SELECT %s
		FROM incidents
		WHERE id = $1
	`, incidentSelectCols)

	inc, err := scanIncident(db.Conn.QueryRow(query, id))
	if err != nil {
		return nil, err
	}

	ml.GetEngine().Evaluate(inc, ctx)
	return inc, nil
}

func (db *DB) GetStats() (*models.DashboardStats, error) {
	incidents, err := db.GetIncidents()
	if err != nil {
		return nil, err
	}

	stats := &models.DashboardStats{
		Total: len(incidents),
	}

	for _, inc := range incidents {
		isResolved := inc.State == "Завершен" || inc.State == "Закрыт"
		if isResolved {
			stats.Resolved++
		} else {
			stats.Active++
		}

		if !isResolved && (inc.PriorityID == 4 || inc.Priority == "Наивысший") {
			stats.Critical++
		}
		if !isResolved && inc.AssignedGroupName == "КЦ ИТ Координаторы Массовых Инцидентов" {
			stats.AssignedCoordinators++
		}
		if !isResolved && (inc.PriorityID == 3 || inc.Priority == "Высокий") {
			stats.HighPriority++
		}
		if inc.State == "Исследование ситуации" {
			stats.UnderInvestigation++
		}
		if !isResolved && inc.AISLARisk >= 50.0 {
			stats.SLARiskCount++
		}
	}

	return stats, nil
}

// GetLiveTelemetry calculates live prediction accuracy and feedback metrics from PostgreSQL.
func (db *DB) GetLiveTelemetry() models.LiveTelemetry {
	var t models.LiveTelemetry

	_ = db.Conn.QueryRow("SELECT COUNT(*) FROM incidents").Scan(&t.TotalIncidents)

	qClosed := `
		SELECT COALESCE(actual_duration_minutes, 0), COALESCE(ai_estimated_minutes, 0)
		FROM incidents
		WHERE actual_duration_minutes IS NOT NULL AND actual_duration_minutes > 0 AND ai_estimated_minutes > 0
	`
	if rows, err := db.Conn.Query(qClosed); err == nil {
		defer rows.Close()
		var absErrSum, apeSum float64
		for rows.Next() {
			var actMin, predMin int
			if err := rows.Scan(&actMin, &predMin); err == nil {
				t.ClosedWithActualTime++
				actH := float64(actMin) / 60.0
				predH := float64(predMin) / 60.0
				absErrSum += math.Abs(actH - predH)
				if actH > 0.1 {
					apeSum += (math.Abs(actH-predH) / actH) * 100.0
				}
			}
		}
		if t.ClosedWithActualTime > 0 {
			t.LiveMAEHours = math.Round((absErrSum/float64(t.ClosedWithActualTime))*1000) / 1000
			t.LiveMAPEPercent = math.Round((apeSum/float64(t.ClosedWithActualTime))*10) / 10
		}
	}

	var accepted int
	_ = db.Conn.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE evaluated_at IS NOT NULL),
		       COUNT(*) FILTER (WHERE is_accepted = true)
		FROM ai_predictions
	`).Scan(&t.TotalPredictions, &t.EvaluatedPredictions, &accepted)

	if t.EvaluatedPredictions > 0 {
		t.RoutingAcceptancePct = math.Round((float64(accepted)*100.0/float64(t.EvaluatedPredictions))*10) / 10
	}

	return t
}

func (db *DB) SeedFromJSONIfEmpty(jsonPath string) error {
	var count int
	err := db.Conn.QueryRow("SELECT COUNT(*) FROM incidents").Scan(&count)
	if err == nil && count > 0 {
		log.Printf("[DB] Table already contains %d incidents, skipping seed\n", count)
		return nil
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read seed json %s: %w", jsonPath, err)
	}

	var rawIncidents []*models.Incident
	if err := json.Unmarshal(data, &rawIncidents); err != nil {
		return fmt.Errorf("failed to parse seed json: %w", err)
	}

	log.Printf("[DB] Seeding %d incidents from %s...\n", len(rawIncidents), jsonPath)
	for _, inc := range rawIncidents {
		if err := db.UpsertIncident(inc); err != nil {
			log.Printf("[DB] Failed to insert incident %s: %v\n", inc.ID, err)
		}
	}

	log.Println("[DB] Seeding complete!")
	return nil
}

// RecordEvent writes an audit log event into incident_events table
func (db *DB) RecordEvent(incidentID, eventType, message string, payload any) error {
	var payloadJSON []byte
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}

	query := `
		INSERT INTO incident_events (incident_id, event_type, message, payload, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	_, err := db.Conn.Exec(query, incidentID, eventType, message, payloadJSON)
	return err
}

type IncidentEventItem struct {
	ID         int             `json:"id"`
	IncidentID string          `json:"incidentId"`
	EventType  string          `json:"eventType"`
	Message    string          `json:"message"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

func (db *DB) GetEventsForIncident(incidentID string) ([]*IncidentEventItem, error) {
	query := `
		SELECT id, incident_id, event_type, message, payload, created_at
		FROM incident_events
		WHERE incident_id = $1
		ORDER BY created_at DESC
	`
	rows, err := db.Conn.Query(query, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*IncidentEventItem
	for rows.Next() {
		var it IncidentEventItem
		var p sql.NullString
		if err := rows.Scan(&it.ID, &it.IncidentID, &it.EventType, &it.Message, &p, &it.CreatedAt); err != nil {
			return nil, err
		}
		if p.Valid {
			it.Payload = json.RawMessage(p.String)
		}
		events = append(events, &it)
	}
	return events, nil
}

type AIPredictionRecord struct {
	ID               int             `json:"id"`
	IncidentID       string          `json:"incidentId"`
	PredictedGroup   string          `json:"predictedGroup"`
	Confidence       float64         `json:"confidence"`
	TopCandidates    json.RawMessage `json:"topCandidates,omitempty"`
	PredictedMinutes int             `json:"predictedMinutes"`
	PredictedSLARisk float64         `json:"predictedSlaRisk"`
	ModelVersion     string          `json:"modelVersion"`
	ActualGroup      *string         `json:"actualGroup,omitempty"`
	IsAccepted       *bool           `json:"isAccepted,omitempty"`
	InputFeatures    json.RawMessage `json:"inputFeatures,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	EvaluatedAt      *time.Time      `json:"evaluatedAt,omitempty"`
}

// RecordAIPrediction writes every unified regression prediction into ai_predictions table for future retraining & MLOps
func (db *DB) RecordAIPrediction(inc *models.Incident) error {
	topJSON, _ := json.Marshal(inc.AIRoutingTop)
	featuresJSON, _ := json.Marshal(map[string]any{
		"header":               inc.Header,
		"description":          inc.Description,
		"productName":          inc.ProductName,
		"service":              inc.Service,
		"scenarioName":         inc.ScenarioName,
		"priorityId":           inc.PriorityID,
		"urgencyId":            inc.UrgencyID,
		"impactId":             inc.ImpactID,
		"serviceType":          inc.ServiceType,
		"teamWorkload":         inc.AITeamWorkload,
		"assigneeWorkload":     inc.AIAssigneeWorkload,
		"historyMedianMinutes": inc.AIHistoryMedianMinutes,
	})

	query := `
		INSERT INTO ai_predictions (
			incident_id, predicted_group, confidence, top_candidates,
			predicted_minutes, predicted_sla_risk, model_version, input_features, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err := db.Conn.Exec(
		query,
		inc.ID, inc.AIRoutingHint, inc.AIRoutingConfidence, topJSON,
		inc.AIEstimatedMinutes, inc.AISLARisk, inc.AIModelVersion, featuresJSON,
	)
	return err
}

func (db *DB) UpdateAIPredictionFeedback(incidentID, actualGroup string) error {
	query := `
		UPDATE ai_predictions
		SET 
			actual_group = $2,
			is_accepted = (predicted_group = $2::varchar),
			evaluated_at = NOW()
		WHERE incident_id = $1 AND actual_group IS NULL
	`
	_, err := db.Conn.Exec(query, incidentID, actualGroup)
	if err != nil {
		log.Printf("[DB] Error updating AI feedback: %v\n", err)
	}
	return err
}

func (db *DB) GetAIPredictions(incidentID string) ([]*AIPredictionRecord, error) {
	query := `
		SELECT 
			id, incident_id, predicted_group, confidence, top_candidates,
			COALESCE(predicted_minutes, 0),
			COALESCE(predicted_sla_risk, 0.0), COALESCE(model_version, 'gbdt_reg_v2.0'),
			actual_group, is_accepted, input_features, created_at, evaluated_at
		FROM ai_predictions
		WHERE incident_id = $1
		ORDER BY created_at DESC
	`
	rows, err := db.Conn.Query(query, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*AIPredictionRecord
	for rows.Next() {
		var r AIPredictionRecord
		var top, feat sql.NullString
		var act sql.NullString
		var acc sql.NullBool
		var eval *time.Time

		err := rows.Scan(
			&r.ID, &r.IncidentID, &r.PredictedGroup, &r.Confidence, &top,
			&r.PredictedMinutes,
			&r.PredictedSLARisk, &r.ModelVersion,
			&act, &acc, &feat, &r.CreatedAt, &eval,
		)
		if err != nil {
			return nil, err
		}

		if top.Valid {
			r.TopCandidates = json.RawMessage(top.String)
		}
		if feat.Valid {
			r.InputFeatures = json.RawMessage(feat.String)
		}
		if act.Valid {
			r.ActualGroup = &act.String
		}
		if acc.Valid {
			r.IsAccepted = &acc.Bool
		}
		r.EvaluatedAt = eval

		records = append(records, &r)
	}
	return records, nil
}
