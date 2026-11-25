package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"itsm-dashboard/backend/internal/database"
	"itsm-dashboard/backend/internal/ml"
	"itsm-dashboard/backend/internal/models"
)

type IncidentHandler struct {
	db     *database.DB
	sseHub *SSEHub
}

func NewIncidentHandler(db *database.DB, sseHub *SSEHub) *IncidentHandler {
	return &IncidentHandler{
		db:     db,
		sseHub: sseHub,
	}
}

func (h *IncidentHandler) GetTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.db.GetTeams()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(teams)
}

func (h *IncidentHandler) GetIncidents(w http.ResponseWriter, r *http.Request) {
	incidents, err := h.db.GetIncidents()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	search := strings.ToLower(r.URL.Query().Get("search"))
	groupFilter := r.URL.Query().Get("group")

	var filtered []*models.Incident
	for _, inc := range incidents {
		if search != "" {
			match := strings.Contains(strings.ToLower(inc.ID), search) ||
				strings.Contains(strings.ToLower(inc.TicketNumber), search) ||
				strings.Contains(strings.ToLower(inc.Header), search) ||
				strings.Contains(strings.ToLower(inc.Description), search) ||
				strings.Contains(strings.ToLower(inc.ProductName), search) ||
				strings.Contains(strings.ToLower(inc.AssignedGroupName), search)
			if !match {
				continue
			}
		}

		if groupFilter != "" {
			switch groupFilter {
			case "critical":
				if inc.PriorityID != 4 && inc.Priority != "Наивысший" {
					continue
				}
			case "coordinators":
				if !strings.Contains(inc.AssignedGroupName, "Координатор") {
					continue
				}
			case "high":
				if inc.PriorityID != 3 && inc.Priority != "Высокий" {
					continue
				}
			case "investigation":
				if inc.State != "Исследование ситуации" {
					continue
				}
			}
		}

		filtered = append(filtered, inc)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (h *IncidentHandler) GetIncidentByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inc, err := h.db.GetIncidentByID(id)
	if err != nil {
		http.Error(w, "Incident not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inc)
}

func (h *IncidentHandler) CreateIncident(w http.ResponseWriter, r *http.Request) {
	var inc models.Incident
	if err := json.NewDecoder(r.Body).Decode(&inc); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if inc.ID == "" && inc.TicketNumber == "" {
		inc.TicketNumber = fmt.Sprintf("INC%010d", time.Now().UnixNano()%1000000000)
	}

	if err := h.db.UpsertIncident(&inc); err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	saved, _ := h.db.GetIncidentByID(inc.ID)
	_ = h.db.RecordAIPrediction(saved)
	_ = h.db.RecordEvent(saved.ID, "CREATED", "Инцидент зарегистрирован в системе", map[string]any{
		"ticket_number": saved.TicketNumber,
		"priority":      saved.Priority,
		"group":         saved.AssignedGroupName,
		"sla_risk":      saved.AISLARisk,
		"est_min":       saved.AIEstimatedMinutes,
	})

	h.sseHub.BroadcastEvent("INCIDENT_CREATED", saved, "Новый инцидент зарегистрирован: "+saved.TicketNumber)

	if saved.PriorityID == 4 || saved.Priority == "Наивысший" {
		h.sseHub.BroadcastEvent("CRITICAL_ALERT", saved, "🚨 КРИТИЧЕСКИЙ ИНЦИДЕНТ: "+saved.Header)
	}
	if strings.Contains(saved.AssignedGroupName, "Координатор") {
		_ = h.db.RecordEvent(saved.ID, "AI_ROUTING_RECOMMENDED", "Регрессионная модель рекомендовала передать в: "+saved.AIRoutingHint, map[string]any{
			"recommended_group": saved.AIRoutingHint,
			"confidence":        saved.AIRoutingConfidence,
			"sla_risk":          saved.AISLARisk,
			"est_min":           saved.AIEstimatedMinutes,
		})
		h.sseHub.BroadcastEvent("COORDINATOR_ALERT", saved, "Инцидент назначен на координаторов: "+saved.TicketNumber)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(saved)
}

func (h *IncidentHandler) UpdateIncident(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.db.GetIncidentByID(id)
	if err != nil {
		http.Error(w, "Incident not found", http.StatusNotFound)
		return
	}

	var updateData models.Incident
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	oldGroup := existing.AssignedGroupName
	oldState := existing.State
	oldOverride := existing.ManualOverrideMinutes

	if updateData.State != "" {
		existing.State = updateData.State
		if (updateData.State == "Завершен" || updateData.State == "Закрыт") && existing.CompletionDate == nil {
			now := time.Now()
			existing.CompletionDate = &now
		}
	}
	if updateData.AssignedGroupName != "" {
		existing.AssignedGroupName = updateData.AssignedGroupName
	}
	if updateData.PriorityID != 0 {
		existing.PriorityID = updateData.PriorityID
		existing.Priority = updateData.Priority
	}
	if updateData.Deadline != nil {
		existing.Deadline = updateData.Deadline
	}
	if updateData.Solution != "" {
		existing.Solution = updateData.Solution
	}
	if updateData.ManualOverrideMinutes != nil {
		if *updateData.ManualOverrideMinutes <= 0 {
			existing.ManualOverrideMinutes = nil
			existing.ManualOverrideReason = ""
		} else {
			existing.ManualOverrideMinutes = updateData.ManualOverrideMinutes
			existing.ManualOverrideReason = updateData.ManualOverrideReason
		}
	} else if updateData.ManualOverrideReason != "" {
		existing.ManualOverrideReason = updateData.ManualOverrideReason
	}

	if err := h.db.UpsertIncident(existing); err != nil {
		http.Error(w, "Database update error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	saved, _ := h.db.GetIncidentByID(existing.ID)

	if existing.AssignedGroupName != oldGroup {
		_ = h.db.RecordEvent(existing.ID, "REASSIGNED", "Перенаправлен в команду: "+existing.AssignedGroupName, map[string]string{
			"from": oldGroup,
			"to":   existing.AssignedGroupName,
		})
		_ = h.db.UpdateAIPredictionFeedback(existing.ID, existing.AssignedGroupName)
	}
	if existing.State != oldState {
		_ = h.db.RecordEvent(existing.ID, "STATE_CHANGED", "Статус изменен на: "+existing.State, map[string]string{
			"old": oldState,
			"new": existing.State,
		})
	}
	if (oldOverride == nil && existing.ManualOverrideMinutes != nil) ||
		(oldOverride != nil && existing.ManualOverrideMinutes != nil && *oldOverride != *existing.ManualOverrideMinutes) {
		_ = h.db.RecordEvent(existing.ID, "MANUAL_OVERRIDE", "Ручная корректировка прогноза диспетчером", map[string]any{
			"override_minutes": *existing.ManualOverrideMinutes,
			"reason":           existing.ManualOverrideReason,
		})
	}

	h.sseHub.BroadcastEvent("INCIDENT_UPDATED", saved, "Обновлен статус инцидента: "+saved.TicketNumber)

	if saved.PriorityID == 4 || saved.Priority == "Наивысший" {
		h.sseHub.BroadcastEvent("CRITICAL_ALERT", saved, "🚨 Эскалация в КРИТИЧЕСКИЙ: "+saved.Header)
	}
	if strings.Contains(saved.AssignedGroupName, "Координатор") {
		_ = h.db.RecordEvent(saved.ID, "AI_ROUTING_RECOMMENDED", "Регрессионная модель рекомендовала передать в: "+saved.AIRoutingHint, map[string]any{
			"recommended_group": saved.AIRoutingHint,
			"confidence":        saved.AIRoutingConfidence,
			"sla_risk":          saved.AISLARisk,
			"est_min":           saved.AIEstimatedMinutes,
		})
		h.sseHub.BroadcastEvent("COORDINATOR_ALERT", saved, "Передан координаторам: "+saved.TicketNumber)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saved)
}

func (h *IncidentHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *IncidentHandler) GetIncidentEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	events, err := h.db.GetEventsForIncident(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (h *IncidentHandler) GetIncidentPredictions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	preds, err := h.db.GetAIPredictions(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preds)
}

// PredictTicket implements POST /api/v1/predict for standalone regression & SLA inference.
func (h *IncidentHandler) PredictTicket(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req models.PredictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.TitleText) == "" && strings.TrimSpace(req.Category) == "" {
		http.Error(w, "Validation error: title_text or category is required", http.StatusUnprocessableEntity)
		return
	}

	submitAt := time.Now()
	if req.CreatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, req.CreatedAt); err == nil {
			submitAt = parsed
		}
	}

	slaHours := req.SLATargetHours
	if slaHours <= 0 {
		slaHours = 8.0
	}
	deadline := submitAt.Add(time.Duration(math.Round(slaHours * 60)) * time.Minute)

	prioID := req.PriorityID
	prioName := req.Priority
	if prioID == 0 {
		switch strings.ToLower(strings.TrimSpace(req.Priority)) {
		case "critical", "наивысший", "критический":
			prioID, prioName = 4, "Наивысший"
		case "high", "высокий":
			prioID, prioName = 3, "Высокий"
		case "medium", "средний":
			prioID, prioName = 2, "Средний"
		default:
			prioID, prioName = 1, "Низкий"
		}
	}
	if prioName == "" {
		prioName = "Средний"
	}

	svcType := req.ServiceType
	if svcType == "" {
		svcType = "Технологический"
	}

	inc := models.Incident{
		TicketNumber:      req.TicketID,
		State:             "Назначен",
		SubmitDate:        &submitAt,
		Deadline:          &deadline,
		Header:            req.TitleText,
		Description:       req.Description,
		ProductName:       req.Category,
		Service:           req.Category,
		AssignedGroupName: req.AssignedGroupName,
		PriorityID:        prioID,
		Priority:          prioName,
		UrgencyID:         prioID,
		ImpactID:          prioID,
		ServiceType:       svcType,
	}

	ctx, _ := h.db.GetRuntimeContext()
	ml.GetEngine().Evaluate(&inc, ctx)

	predHours := math.Round((float64(inc.AIEstimatedMinutes)/60.0)*100) / 100
	queueHours := math.Round((float64(inc.AIQueueRemainingMinutes)/60.0)*100) / 100
	totalWaitHours := math.Round((predHours+queueHours*0.35)*100) / 100
	minHours := math.Round((float64(inc.AIEstimatedMinMinutes)/60.0)*100) / 100
	maxHours := math.Round((float64(inc.AIEstimatedMaxMinutes)/60.0)*100) / 100
	histMedianHours := math.Round((float64(inc.AIHistoryMedianMinutes)/60.0)*100) / 100
	finishAt := time.Now().Add(time.Duration(math.Round(totalWaitHours*60)) * time.Minute)

	slaStatus := "OK"
	switch inc.AISLALevel {
	case "WARNING":
		slaStatus = "WARNING"
	case "CRITICAL":
		slaStatus = "CRITICAL_SLA_BREACH"
	}

	resp := models.PredictResponse{
		TicketID:              req.TicketID,
		PredictedTimeHours:    predHours,
		TotalWaitHours:        totalWaitHours,
		PredictedFinishTime:   finishAt.Format(time.RFC3339),
		ConfidenceInterval:    [2]float64{minHours, maxHours},
		SLAStatus:             slaStatus,
		SLARiskScore:          math.Round((inc.AISLARisk/100.0)*1000) / 1000,
		TeamWorkload:          inc.AITeamWorkload,
		QueueRemainingHours:   queueHours,
		HistoryCategoryMedian: histMedianHours,
		RecommendedGroup:      inc.AIRoutingHint,
		TopFactors:            inc.AITopFactors,
		ModelVersion:          inc.AIModelVersion,
		Fallback:              inc.AIFallbackUsed,
		LowConfidence:         inc.AILowConfidence,
		LatencyMs:             math.Round(float64(time.Since(start).Microseconds())/10.0) / 100.0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetMLMetrics exposes offline validation metrics and live PostgreSQL telemetry.
func (h *IncidentHandler) GetMLMetrics(w http.ResponseWriter, r *http.Request) {
	name, version, features, importances, offline := ml.GetEngine().GetOfflineMetrics()
	live := h.db.GetLiveTelemetry()

	resp := models.MLMetricsResponse{
		ModelName:          name,
		ModelVersion:       version,
		Features:           features,
		FeatureImportances: importances,
		OfflineMetrics:     offline,
		LiveMetrics:        live,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ExportIncidentsCSV exports the incidents registry with ML forecasts and team statistics as UTF-8 CSV.
func (h *IncidentHandler) ExportIncidentsCSV(w http.ResponseWriter, r *http.Request) {
	incidents, err := h.db.GetIncidents()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"incidents_sla_report_%s.csv\"", time.Now().Format("20060102_1504")))

	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{
		"UUID", "Номер_заявки", "Заголовок", "Статус", "Приоритет", "Продукт",
		"Команда_решения", "Очередь_команды",
		"Прогноз_мин", "ДИ_80_мин", "ДИ_80_макс", "Медиана_истории_мин",
		"Факт_мин", "Ошибка_мин", "Ручная_коррекция_мин",
		"Риск_SLA_%", "Уровень_SLA", "Версия_модели",
	})

	for _, inc := range incidents {
		actStr := ""
		if inc.ActualDurationMinutes != nil {
			actStr = strconv.Itoa(*inc.ActualDurationMinutes)
		}
		errStr := ""
		if inc.PredictionErrorMinutes != nil {
			errStr = strconv.Itoa(*inc.PredictionErrorMinutes)
		}
		manStr := ""
		if inc.ManualOverrideMinutes != nil {
			manStr = strconv.Itoa(*inc.ManualOverrideMinutes)
		}
		_ = cw.Write([]string{
			inc.ID,
			inc.TicketNumber,
			inc.Header,
			inc.State,
			inc.Priority,
			inc.ProductName,
			inc.AssignedGroupName,
			strconv.Itoa(inc.AITeamWorkload),
			strconv.Itoa(inc.AIEstimatedMinutes),
			strconv.Itoa(inc.AIEstimatedMinMinutes),
			strconv.Itoa(inc.AIEstimatedMaxMinutes),
			strconv.Itoa(inc.AIHistoryMedianMinutes),
			actStr,
			errStr,
			manStr,
			fmt.Sprintf("%.1f", inc.AISLARisk),
			inc.AISLALevel,
			inc.AIModelVersion,
		})
	}
	cw.Flush()
}
