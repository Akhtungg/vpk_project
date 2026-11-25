package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"itsm-dashboard/backend/internal/ml"
	"itsm-dashboard/backend/internal/models"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool {
	return uuidRegex.MatchString(strings.TrimSpace(s))
}

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

	log.Println("[DB] Normalized UUID schema initialized successfully")
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

func (db *DB) ensureTeam(name string) (*string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, nil
	}
	isCoord := strings.Contains(trimmed, "Координатор")
	var id string
	err := db.Conn.QueryRow(`
		INSERT INTO teams (name, is_coordinator, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (name) DO UPDATE SET updated_at = teams.updated_at
		RETURNING id::text
	`, trimmed, isCoord).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (db *DB) ensureService(product, masterID, service, scenario, srvType string, srvTypeID int) (*string, error) {
	p := strings.TrimSpace(product)
	s := strings.TrimSpace(service)
	if p == "" && s == "" {
		return nil, nil
	}
	if p == "" {
		p = s
	}
	if s == "" {
		s = p
	}
	if srvType == "" {
		srvType = "Технологический"
	}
	if srvTypeID == 0 {
		srvTypeID = 30
	}
	var id string
	err := db.Conn.QueryRow(`
		INSERT INTO services (product_name, product_master_id, service_name, service_type, service_type_id, scenario_name)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (product_name, service_name) DO UPDATE SET
			service_type = COALESCE(NULLIF(EXCLUDED.service_type, ''), services.service_type)
		RETURNING id::text
	`, p, masterID, s, srvType, srvTypeID, scenario).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (db *DB) resolveIncidentUUID(idOrTicket string) (string, error) {
	var uid string
	err := db.Conn.QueryRow(`
		SELECT id::text FROM incidents
		WHERE id::text = $1 OR ticket_number = $1
		LIMIT 1
	`, idOrTicket).Scan(&uid)
	return uid, err
}

// GetRuntimeContext aggregates active team queues and historical resolution medians over normalized tables.
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

	// 1. Active queue workloads per team
	qActive := `
		SELECT COALESCE(t.name, ''), COALESCE(f.estimated_minutes, 90)
		FROM incidents i
		LEFT JOIN teams t ON t.id = i.team_id
		LEFT JOIN incident_forecasts f ON f.incident_id = i.id
		WHERE i.state NOT IN ('Завершен', 'Закрыт', 'Решен')
	`
	if rows, err := db.Conn.Query(qActive); err == nil {
		defer rows.Close()
		for rows.Next() {
			var grp string
			var estMin int
			if err := rows.Scan(&grp, &estMin); err == nil {
				if estMin <= 0 {
					estMin = 90
				}
				if grp != "" {
					ctx.TeamWorkloads[grp]++
					ctx.TeamQueueMinutes[grp] += estMin
				}
			}
		}
	}

	// 2. Historical durations over closed tickets joined with teams & services
	qHist := `
		SELECT COALESCE(t.name, ''), COALESCE(s.product_name, ''), i.priority_id,
		       EXTRACT(EPOCH FROM (i.completion_date - i.submit_date)) / 3600.0 AS dur_hours
		FROM incidents i
		LEFT JOIN teams t ON t.id = i.team_id
		LEFT JOIN services s ON s.id = i.service_id
		WHERE i.completion_date IS NOT NULL AND i.submit_date IS NOT NULL
		  AND i.completion_date > i.submit_date
	`
	gpBuckets := make(map[string][]float64)
	gBuckets := make(map[string][]float64)
	pBuckets := make(map[string][]float64)
	var allDurs []float64

	if rows, err := db.Conn.Query(qHist); err == nil {
		defer rows.Close()
		for rows.Next() {
			var grp, prod string
			var prio int
			var dur float64
			if err := rows.Scan(&grp, &prod, &prio, &dur); err == nil && dur > 0.05 && dur < 200 {
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

	return ctx, nil
}

func (db *DB) UpsertIncident(inc *models.Incident) error {
	ctx, _ := db.GetRuntimeContext()

	// Support seed/legacy IDs like "INC00001001" alongside UUIDs
	if inc.TicketNumber == "" {
		if isUUID(inc.ID) {
			inc.TicketNumber = fmt.Sprintf("INC%010d", time.Now().UnixNano()%1000000000)
		} else if inc.ID != "" {
			inc.TicketNumber = inc.ID
			inc.ID = ""
		} else {
			inc.TicketNumber = fmt.Sprintf("INC%010d", time.Now().UnixNano()%1000000000)
		}
	} else if !isUUID(inc.ID) {
		inc.ID = ""
	}

	// Evaluate unified Regression ML enrichments
	ml.GetEngine().Evaluate(inc, ctx)

	teamID, err := db.ensureTeam(inc.AssignedGroupName)
	if err != nil {
		return fmt.Errorf("ensure team: %w", err)
	}
	serviceID, err := db.ensureService(inc.ProductName, inc.ProductMasterID, inc.Service, inc.ScenarioName, inc.ServiceType, inc.ServiceTypeID)
	if err != nil {
		return fmt.Errorf("ensure service: %w", err)
	}
	recTeamID, _ := db.ensureTeam(inc.AIRoutingHint)

	inc.TeamID = teamID
	inc.ServiceIDRef = serviceID
	inc.RecommendedTeamID = recTeamID

	// Upsert core incident record
	var savedUUID string
	if isUUID(inc.ID) {
		err = db.Conn.QueryRow(`
			INSERT INTO incidents (
				id, ticket_number, external_sys_id, team_id, service_id,
				state, state_enum, state_id, state_reason,
				priority_id, priority, urgency_id, urgency, impact_id, impact,
				header, description, short_description,
				initiator_name, initiator_login, solution,
				is_effect, is_reason, is_double, is_master,
				submit_date, deadline, completion_date, last_assignment_to_team,
				updated_at
			) VALUES (
				$1::uuid, $2, $3, $4::uuid, $5::uuid,
				$6, $7, $8, $9,
				$10, $11, $12, $13, $14, $15,
				$16, $17, $18,
				$19, $20, $21,
				$22, $23, $24, $25,
				$26, $27, $28, $29,
				NOW()
			)
			ON CONFLICT (id) DO UPDATE SET
				team_id = EXCLUDED.team_id,
				service_id = COALESCE(EXCLUDED.service_id, incidents.service_id),
				state = EXCLUDED.state,
				state_enum = EXCLUDED.state_enum,
				state_id = EXCLUDED.state_id,
				state_reason = EXCLUDED.state_reason,
				priority_id = EXCLUDED.priority_id,
				priority = EXCLUDED.priority,
				urgency_id = EXCLUDED.urgency_id,
				urgency = EXCLUDED.urgency,
				impact_id = EXCLUDED.impact_id,
				impact = EXCLUDED.impact,
				header = EXCLUDED.header,
				description = EXCLUDED.description,
				short_description = EXCLUDED.short_description,
				solution = EXCLUDED.solution,
				deadline = EXCLUDED.deadline,
				completion_date = EXCLUDED.completion_date,
				last_assignment_to_team = EXCLUDED.last_assignment_to_team,
				updated_at = NOW()
			RETURNING id::text
		`,
			inc.ID, inc.TicketNumber, inc.SysID, teamID, serviceID,
			inc.State, inc.StateEnum, inc.StateID, inc.StateReason,
			inc.PriorityID, inc.Priority, inc.UrgencyID, inc.Urgency, inc.ImpactID, inc.Impact,
			inc.Header, inc.Description, inc.ShortDescription,
			inc.InitiatorName, inc.InitiatorLogin, inc.Solution,
			inc.IsEffect, inc.IsReason, inc.IsDouble, inc.IsMaster,
			inc.SubmitDate, inc.Deadline, inc.CompletionDate, inc.LastAssignmentToTeamDateTime,
		).Scan(&savedUUID)
	} else {
		err = db.Conn.QueryRow(`
			INSERT INTO incidents (
				ticket_number, external_sys_id, team_id, service_id,
				state, state_enum, state_id, state_reason,
				priority_id, priority, urgency_id, urgency, impact_id, impact,
				header, description, short_description,
				initiator_name, initiator_login, solution,
				is_effect, is_reason, is_double, is_master,
				submit_date, deadline, completion_date, last_assignment_to_team,
				updated_at
			) VALUES (
				$1, $2, $3::uuid, $4::uuid,
				$5, $6, $7, $8,
				$9, $10, $11, $12, $13, $14,
				$15, $16, $17,
				$18, $19, $20,
				$21, $22, $23, $24,
				$25, $26, $27, $28,
				NOW()
			)
			ON CONFLICT (ticket_number) DO UPDATE SET
				team_id = EXCLUDED.team_id,
				service_id = COALESCE(EXCLUDED.service_id, incidents.service_id),
				state = EXCLUDED.state,
				state_enum = EXCLUDED.state_enum,
				state_id = EXCLUDED.state_id,
				state_reason = EXCLUDED.state_reason,
				priority_id = EXCLUDED.priority_id,
				priority = EXCLUDED.priority,
				urgency_id = EXCLUDED.urgency_id,
				urgency = EXCLUDED.urgency,
				impact_id = EXCLUDED.impact_id,
				impact = EXCLUDED.impact,
				header = EXCLUDED.header,
				description = EXCLUDED.description,
				short_description = EXCLUDED.short_description,
				solution = EXCLUDED.solution,
				deadline = EXCLUDED.deadline,
				completion_date = EXCLUDED.completion_date,
				last_assignment_to_team = EXCLUDED.last_assignment_to_team,
				updated_at = NOW()
			RETURNING id::text
		`,
			inc.TicketNumber, inc.SysID, teamID, serviceID,
			inc.State, inc.StateEnum, inc.StateID, inc.StateReason,
			inc.PriorityID, inc.Priority, inc.UrgencyID, inc.Urgency, inc.ImpactID, inc.Impact,
			inc.Header, inc.Description, inc.ShortDescription,
			inc.InitiatorName, inc.InitiatorLogin, inc.Solution,
			inc.IsEffect, inc.IsReason, inc.IsDouble, inc.IsMaster,
			inc.SubmitDate, inc.Deadline, inc.CompletionDate, inc.LastAssignmentToTeamDateTime,
		).Scan(&savedUUID)
	}
	if err != nil {
		return err
	}
	inc.ID = savedUUID

	factorsJSON, _ := json.Marshal(inc.AITopFactors)
	routingTopJSON, _ := json.Marshal(inc.AIRoutingTop)

	_, err = db.Conn.Exec(`
		INSERT INTO incident_forecasts (
			incident_id, recommended_team_id, routing_confidence,
			sla_risk, sla_level, team_workload, queue_remaining_minutes,
			estimated_minutes, estimated_min_minutes, estimated_max_minutes,
			history_median_minutes, sla_margin_minutes, top_factors, routing_candidates,
			low_confidence, fallback_used, model_version,
			manual_override_minutes, manual_override_reason,
			actual_duration_minutes, prediction_error_minutes, updated_at
		) VALUES (
			$1::uuid, $2::uuid, $3,
			$4, $5, $6, $7,
			$8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17,
			$18, $19,
			$20, $21, NOW()
		)
		ON CONFLICT (incident_id) DO UPDATE SET
			recommended_team_id = EXCLUDED.recommended_team_id,
			routing_confidence = EXCLUDED.routing_confidence,
			sla_risk = EXCLUDED.sla_risk,
			sla_level = EXCLUDED.sla_level,
			team_workload = EXCLUDED.team_workload,
			queue_remaining_minutes = EXCLUDED.queue_remaining_minutes,
			estimated_minutes = EXCLUDED.estimated_minutes,
			estimated_min_minutes = EXCLUDED.estimated_min_minutes,
			estimated_max_minutes = EXCLUDED.estimated_max_minutes,
			history_median_minutes = EXCLUDED.history_median_minutes,
			sla_margin_minutes = EXCLUDED.sla_margin_minutes,
			top_factors = EXCLUDED.top_factors,
			routing_candidates = EXCLUDED.routing_candidates,
			low_confidence = EXCLUDED.low_confidence,
			fallback_used = EXCLUDED.fallback_used,
			model_version = EXCLUDED.model_version,
			manual_override_minutes = EXCLUDED.manual_override_minutes,
			manual_override_reason = EXCLUDED.manual_override_reason,
			actual_duration_minutes = EXCLUDED.actual_duration_minutes,
			prediction_error_minutes = EXCLUDED.prediction_error_minutes,
			updated_at = NOW()
	`,
		inc.ID, recTeamID, inc.AIRoutingConfidence,
		inc.AISLARisk, inc.AISLALevel, inc.AITeamWorkload, inc.AIQueueRemainingMinutes,
		inc.AIEstimatedMinutes, inc.AIEstimatedMinMinutes, inc.AIEstimatedMaxMinutes,
		inc.AIHistoryMedianMinutes, inc.AISLAMarginMinutes, factorsJSON, routingTopJSON,
		inc.AILowConfidence, inc.AIFallbackUsed, inc.AIModelVersion,
		inc.ManualOverrideMinutes, inc.ManualOverrideReason,
		inc.ActualDurationMinutes, inc.PredictionErrorMinutes,
	)

	if err == nil && teamID != nil && inc.AIHistoryMedianMinutes > 0 {
		_, _ = db.Conn.Exec(`UPDATE teams SET historical_median_minutes = $2, updated_at = NOW() WHERE id = $1::uuid`, *teamID, inc.AIHistoryMedianMinutes)
	}

	return err
}

const incidentSelectJoinedCols = `
	i.id::text, i.ticket_number, COALESCE(i.external_sys_id, ''),
	i.team_id::text, i.service_id::text,
	i.state, COALESCE(i.state_enum, ''), i.state_id, COALESCE(i.state_reason, ''),
	i.submit_date, i.deadline, i.completion_date, i.last_assignment_to_team,
	i.header, i.description, COALESCE(i.short_description, ''),
	COALESCE(t.name, ''),
	COALESCE(i.initiator_name, ''), COALESCE(i.initiator_login, ''),
	i.priority_id, i.priority, i.urgency_id, COALESCE(i.urgency, ''), i.impact_id, COALESCE(i.impact, ''),
	COALESCE(s.product_name, ''), COALESCE(s.product_master_id, ''), COALESCE(s.service_name, ''),
	COALESCE(s.scenario_name, ''), COALESCE(i.solution, ''),
	COALESCE(s.service_type, 'Технологический'), COALESCE(s.service_type_id, 30),
	i.is_effect, i.is_reason, i.is_double, i.is_master,
	f.recommended_team_id::text, COALESCE(rt.name, ''), COALESCE(f.routing_confidence, 0.0),
	COALESCE(f.sla_risk, 0.0), COALESCE(f.sla_level, 'SAFE'), COALESCE(f.team_workload, 0),
	COALESCE(f.queue_remaining_minutes, 0), COALESCE(f.estimated_minutes, 0),
	COALESCE(f.estimated_min_minutes, 0), COALESCE(f.estimated_max_minutes, 0),
	COALESCE(f.history_median_minutes, 0), COALESCE(f.sla_margin_minutes, 0),
	f.top_factors, f.routing_candidates,
	COALESCE(f.low_confidence, false), COALESCE(f.fallback_used, false),
	COALESCE(f.model_version, 'gbdt_rubert_v3.0'),
	f.manual_override_minutes, COALESCE(f.manual_override_reason, ''),
	f.actual_duration_minutes, f.prediction_error_minutes,
	i.created_at, i.updated_at
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIncident(rs rowScanner) (*models.Incident, error) {
	var inc models.Incident
	var submitDate, deadline, completionDate, lastAssignment *time.Time
	var teamID, serviceRefID, recTeamID sql.NullString
	var topFactorsRaw, routingTopRaw sql.NullString
	var manOverrideMin, actDurMin, predErrMin sql.NullInt64

	err := rs.Scan(
		&inc.ID, &inc.TicketNumber, &inc.SysID,
		&teamID, &serviceRefID,
		&inc.State, &inc.StateEnum, &inc.StateID, &inc.StateReason,
		&submitDate, &deadline, &completionDate, &lastAssignment,
		&inc.Header, &inc.Description, &inc.ShortDescription,
		&inc.AssignedGroupName,
		&inc.InitiatorName, &inc.InitiatorLogin,
		&inc.PriorityID, &inc.Priority, &inc.UrgencyID, &inc.Urgency, &inc.ImpactID, &inc.Impact,
		&inc.ProductName, &inc.ProductMasterID, &inc.Service,
		&inc.ScenarioName, &inc.Solution,
		&inc.ServiceType, &inc.ServiceTypeID,
		&inc.IsEffect, &inc.IsReason, &inc.IsDouble, &inc.IsMaster,
		&recTeamID, &inc.AIRoutingHint, &inc.AIRoutingConfidence,
		&inc.AISLARisk, &inc.AISLALevel, &inc.AITeamWorkload,
		&inc.AIQueueRemainingMinutes, &inc.AIEstimatedMinutes,
		&inc.AIEstimatedMinMinutes, &inc.AIEstimatedMaxMinutes,
		&inc.AIHistoryMedianMinutes, &inc.AISLAMarginMinutes,
		&topFactorsRaw, &routingTopRaw,
		&inc.AILowConfidence, &inc.AIFallbackUsed,
		&inc.AIModelVersion,
		&manOverrideMin, &inc.ManualOverrideReason,
		&actDurMin, &predErrMin,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if teamID.Valid {
		v := teamID.String
		inc.TeamID = &v
	}
	if serviceRefID.Valid {
		v := serviceRefID.String
		inc.ServiceIDRef = &v
	}
	if recTeamID.Valid {
		v := recTeamID.String
		inc.RecommendedTeamID = &v
	}

	inc.SubmitDate = submitDate
	inc.Deadline = deadline
	inc.CompletionDate = completionDate
	inc.LastAssignmentToTeamDateTime = lastAssignment

	if topFactorsRaw.Valid && topFactorsRaw.String != "" {
		_ = json.Unmarshal([]byte(topFactorsRaw.String), &inc.AITopFactors)
	}
	if routingTopRaw.Valid && routingTopRaw.String != "" {
		_ = json.Unmarshal([]byte(routingTopRaw.String), &inc.AIRoutingTop)
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
		FROM incidents i
		LEFT JOIN teams t ON t.id = i.team_id
		LEFT JOIN services s ON s.id = i.service_id
		LEFT JOIN incident_forecasts f ON f.incident_id = i.id
		LEFT JOIN teams rt ON rt.id = f.recommended_team_id
		ORDER BY i.priority_id DESC, COALESCE(f.sla_risk, 0) DESC, i.updated_at DESC
	`, incidentSelectJoinedCols)

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

func (db *DB) GetIncidentByID(idOrTicket string) (*models.Incident, error) {
	ctx, _ := db.GetRuntimeContext()

	query := fmt.Sprintf(`
		SELECT %s
		FROM incidents i
		LEFT JOIN teams t ON t.id = i.team_id
		LEFT JOIN services s ON s.id = i.service_id
		LEFT JOIN incident_forecasts f ON f.incident_id = i.id
		LEFT JOIN teams rt ON rt.id = f.recommended_team_id
		WHERE i.id::text = $1 OR i.ticket_number = $1
		LIMIT 1
	`, incidentSelectJoinedCols)

	inc, err := scanIncident(db.Conn.QueryRow(query, idOrTicket))
	if err != nil {
		return nil, err
	}

	ml.GetEngine().Evaluate(inc, ctx)
	return inc, nil
}

// GetTeams returns all teams from the normalized teams table with aggregated live and forecast stats.
func (db *DB) GetTeams() ([]*models.Team, error) {
	query := `
		SELECT
			t.id::text,
			t.name,
			t.is_coordinator,
			COUNT(i.id) FILTER (WHERE i.state NOT IN ('Завершен', 'Закрыт', 'Решен')) AS active_count,
			COUNT(i.id) AS total_count,
			COALESCE(MAX(f.team_workload), 0) AS queue_count,
			COALESCE(ROUND(AVG(NULLIF(f.estimated_minutes, 0))), 0)::int AS avg_est_min,
			COALESCE(ROUND(AVG(CASE WHEN i.state NOT IN ('Завершен', 'Закрыт', 'Решен') THEN f.sla_risk END)::numeric, 1), 0)::double precision AS avg_sla_risk,
			t.historical_median_minutes,
			t.created_at,
			t.updated_at
		FROM teams t
		LEFT JOIN incidents i ON i.team_id = t.id
		LEFT JOIN incident_forecasts f ON f.incident_id = i.id
		GROUP BY t.id
		ORDER BY active_count DESC, total_count DESC, t.name ASC
	`
	rows, err := db.Conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*models.Team
	for rows.Next() {
		var tm models.Team
		if err := rows.Scan(
			&tm.ID, &tm.Name, &tm.IsCoordinator,
			&tm.ActiveCount, &tm.TotalCount, &tm.QueueCount,
			&tm.AvgEstimatedMinutes, &tm.AvgSLARisk, &tm.HistoricalMedianMinutes,
			&tm.CreatedAt, &tm.UpdatedAt,
		); err != nil {
			return nil, err
		}
		teams = append(teams, &tm)
	}
	return teams, nil
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
		isResolved := inc.State == "Завершен" || inc.State == "Закрыт" || inc.State == "Решен"
		if isResolved {
			stats.Resolved++
		} else {
			stats.Active++
		}

		if !isResolved && (inc.PriorityID == 4 || inc.Priority == "Наивысший") {
			stats.Critical++
		}
		if !isResolved && strings.Contains(inc.AssignedGroupName, "Координатор") {
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
		SELECT COALESCE(actual_duration_minutes, 0), COALESCE(estimated_minutes, 0)
		FROM incident_forecasts
		WHERE actual_duration_minutes IS NOT NULL AND actual_duration_minutes > 0 AND estimated_minutes > 0
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

	log.Printf("[DB] Seeding %d incidents into normalized UUID schema from %s...\n", len(rawIncidents), jsonPath)
	for _, inc := range rawIncidents {
		if err := db.UpsertIncident(inc); err != nil {
			log.Printf("[DB] Failed to insert incident %s: %v\n", inc.TicketNumber, err)
		}
	}

	log.Println("[DB] Seeding complete!")
	return nil
}

// RecordEvent writes an audit log event into incident_events table
func (db *DB) RecordEvent(incidentIDOrTicket, eventType, message string, payload any) error {
	uid := incidentIDOrTicket
	if !isUUID(uid) {
		resolved, err := db.resolveIncidentUUID(incidentIDOrTicket)
		if err != nil {
			return err
		}
		uid = resolved
	}

	var payloadJSON []byte
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}

	_, err := db.Conn.Exec(`
		INSERT INTO incident_events (incident_id, event_type, message, payload, created_at)
		VALUES ($1::uuid, $2, $3, $4, NOW())
	`, uid, eventType, message, payloadJSON)
	return err
}

type IncidentEventItem struct {
	ID         string          `json:"id"`
	IncidentID string          `json:"incidentId"`
	EventType  string          `json:"eventType"`
	Message    string          `json:"message"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

func (db *DB) GetEventsForIncident(idOrTicket string) ([]*IncidentEventItem, error) {
	uid := idOrTicket
	if !isUUID(uid) {
		resolved, err := db.resolveIncidentUUID(idOrTicket)
		if err != nil {
			return nil, nil
		}
		uid = resolved
	}

	rows, err := db.Conn.Query(`
		SELECT id::text, incident_id::text, event_type, message, payload, created_at
		FROM incident_events
		WHERE incident_id = $1::uuid
		ORDER BY created_at DESC
	`, uid)
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
	ID               string          `json:"id"`
	IncidentID       string          `json:"incidentId"`
	PredictedTeamID  *string         `json:"predictedTeamId,omitempty"`
	PredictedGroup   string          `json:"predictedGroup"`
	Confidence       float64         `json:"confidence"`
	TopCandidates    json.RawMessage `json:"topCandidates,omitempty"`
	PredictedMinutes int             `json:"predictedMinutes"`
	PredictedSLARisk float64         `json:"predictedSlaRisk"`
	ModelVersion     string          `json:"modelVersion"`
	ActualTeamID     *string         `json:"actualTeamId,omitempty"`
	ActualGroup      *string         `json:"actualGroup,omitempty"`
	IsAccepted       *bool           `json:"isAccepted,omitempty"`
	InputFeatures    json.RawMessage `json:"inputFeatures,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	EvaluatedAt      *time.Time      `json:"evaluatedAt,omitempty"`
}

// RecordAIPrediction writes every unified regression prediction into ai_predictions table
func (db *DB) RecordAIPrediction(inc *models.Incident) error {
	predTeamID, _ := db.ensureTeam(inc.AIRoutingHint)
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
		"historyMedianMinutes": inc.AIHistoryMedianMinutes,
	})

	_, err := db.Conn.Exec(`
		INSERT INTO ai_predictions (
			incident_id, predicted_team_id, confidence, top_candidates,
			predicted_minutes, predicted_sla_risk, model_version, input_features, created_at
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, NOW())
	`,
		inc.ID, predTeamID, inc.AIRoutingConfidence, topJSON,
		inc.AIEstimatedMinutes, inc.AISLARisk, inc.AIModelVersion, featuresJSON,
	)
	return err
}

func (db *DB) UpdateAIPredictionFeedback(idOrTicket, actualGroup string) error {
	uid := idOrTicket
	if !isUUID(uid) {
		resolved, err := db.resolveIncidentUUID(idOrTicket)
		if err != nil {
			return err
		}
		uid = resolved
	}
	actualTeamID, err := db.ensureTeam(actualGroup)
	if err != nil || actualTeamID == nil {
		return err
	}

	_, err = db.Conn.Exec(`
		UPDATE ai_predictions
		SET
			actual_team_id = $2::uuid,
			is_accepted = (predicted_team_id = $2::uuid),
			evaluated_at = NOW()
		WHERE incident_id = $1::uuid AND actual_team_id IS NULL
	`, uid, *actualTeamID)
	if err != nil {
		log.Printf("[DB] Error updating AI feedback: %v\n", err)
	}
	return err
}

func (db *DB) GetAIPredictions(idOrTicket string) ([]*AIPredictionRecord, error) {
	uid := idOrTicket
	if !isUUID(uid) {
		resolved, err := db.resolveIncidentUUID(idOrTicket)
		if err != nil {
			return nil, nil
		}
		uid = resolved
	}

	rows, err := db.Conn.Query(`
		SELECT
			p.id::text, p.incident_id::text, p.predicted_team_id::text, COALESCE(pt.name, ''),
			p.confidence, p.top_candidates,
			COALESCE(p.predicted_minutes, 0),
			COALESCE(p.predicted_sla_risk, 0.0), COALESCE(p.model_version, 'gbdt_rubert_v3.0'),
			p.actual_team_id::text, at.name, p.is_accepted, p.input_features, p.created_at, p.evaluated_at
		FROM ai_predictions p
		LEFT JOIN teams pt ON pt.id = p.predicted_team_id
		LEFT JOIN teams at ON at.id = p.actual_team_id
		WHERE p.incident_id = $1::uuid
		ORDER BY p.created_at DESC
	`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*AIPredictionRecord
	for rows.Next() {
		var r AIPredictionRecord
		var predTeamID, actTeamID, top, feat, act sql.NullString
		var acc sql.NullBool
		var eval *time.Time

		err := rows.Scan(
			&r.ID, &r.IncidentID, &predTeamID, &r.PredictedGroup,
			&r.Confidence, &top,
			&r.PredictedMinutes,
			&r.PredictedSLARisk, &r.ModelVersion,
			&actTeamID, &act, &acc, &feat, &r.CreatedAt, &eval,
		)
		if err != nil {
			return nil, err
		}

		if predTeamID.Valid {
			v := predTeamID.String
			r.PredictedTeamID = &v
		}
		if actTeamID.Valid {
			v := actTeamID.String
			r.ActualTeamID = &v
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
