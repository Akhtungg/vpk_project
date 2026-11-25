package models

import (
	"time"
)

type FactorContribution struct {
	Factor      string  `json:"factor"`
	ImpactHours float64 `json:"impactHours"`
}

// Team represents a normalized resolution team entity from the teams table.
type Team struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	IsCoordinator           bool      `json:"isCoordinator"`
	ActiveCount             int       `json:"activeCount"`
	TotalCount              int       `json:"totalCount"`
	QueueCount              int       `json:"queueCount"`
	AvgEstimatedMinutes     int       `json:"avgEstimatedMinutes"`
	AvgSLARisk              float64   `json:"avgSlaRisk"`
	HistoricalMedianMinutes int       `json:"historicalMedianMinutes"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// Incident represents the normalized ITSM incident DTO joined across incidents, teams, services, and incident_forecasts.
type Incident struct {
	ID                           string     `json:"id"`
	TicketNumber                 string     `json:"ticketNumber"`
	SysID                        string     `json:"sysId,omitempty"`
	TeamID                       *string    `json:"teamId,omitempty"`
	ServiceIDRef                 *string    `json:"serviceRefId,omitempty"`
	State                        string     `json:"state"`
	StateEnum                    string     `json:"stateEnum,omitempty"`
	StateID                      int        `json:"stateId"`
	StateReason                  string     `json:"stateReason,omitempty"`
	StateReasonID                string     `json:"stateReasonId,omitempty"`
	SubmitDate                   *time.Time `json:"submitDate,omitempty"`
	Deadline                     *time.Time `json:"deadline,omitempty"`
	CompletionDate               *time.Time `json:"completionDate,omitempty"`
	LastAssignmentToTeamDateTime *time.Time `json:"lastAssignmentToTeamDateTime,omitempty"`
	Header                       string     `json:"header"`
	Description                  string     `json:"description"`
	ShortDescription             string     `json:"shortDescription,omitempty"`
	AssignedGroupName            string     `json:"assignedGroupName"`
	AssignedEmployee             string     `json:"-"`
	AssignedEmployeeLogin        string     `json:"-"`
	InitiatorName                string     `json:"initiatorName,omitempty"`
	InitiatorLogin               string     `json:"initiatorLogin,omitempty"`
	IsEffect                     bool       `json:"isEffect"`
	IsReason                     bool       `json:"isReason"`
	IsDouble                     bool       `json:"isDouble"`
	IsMaster                     bool       `json:"isMaster"`
	IncType                      string     `json:"incType,omitempty"`
	ServiceTypeID                int        `json:"serviceTypeId,omitempty"`
	ServiceType                  string     `json:"serviceType,omitempty"`
	PriorityID                   int        `json:"priorityId"`
	Priority                     string     `json:"priority"`
	UrgencyID                    int        `json:"urgencyId,omitempty"`
	Urgency                      string     `json:"urgency,omitempty"`
	ImpactID                     int        `json:"impactId,omitempty"`
	Impact                       string     `json:"impact,omitempty"`
	WaitingExitDate              string     `json:"waitingExitDate,omitempty"`
	IsRootIncident               bool       `json:"isRootIncident,omitempty"`
	ProductName                  string     `json:"productName,omitempty"`
	ProductID                    string     `json:"productId,omitempty"`
	ProductMasterID              string     `json:"productMasterId,omitempty"`
	ProductURL                   string     `json:"productUrl,omitempty"`
	Service                      string     `json:"service,omitempty"`
	ServiceID                    string     `json:"serviceId,omitempty"`
	ScenarioName                 string     `json:"scenarioName,omitempty"`
	IsNeedUp                     *bool      `json:"isNeedUp,omitempty"`
	EliminationType              string     `json:"eliminationType,omitempty"`
	EliminationTypeID            string     `json:"eliminationTypeId,omitempty"`
	ReceivingChannel             string     `json:"receivingChannel,omitempty"`
	Solution                     string     `json:"solution,omitempty"`

	// Unified Regression ML Enrichments (from incident_forecasts)
	RecommendedTeamID       *string              `json:"recommendedTeamId,omitempty"`
	AIRoutingHint           string               `json:"aiRoutingHint"`
	AIRoutingConfidence     float64              `json:"aiRoutingConfidence"`
	AIRoutingTop            []GroupScore         `json:"aiRoutingTop,omitempty"`
	AISLARisk               float64              `json:"aiSlaRisk"`
	AISLALevel              string               `json:"aiSlaLevel"` // "SAFE", "WARNING", "CRITICAL"
	AITeamWorkload          int                  `json:"aiTeamWorkload"`
	AIAssigneeWorkload      int                  `json:"-"`
	AIQueueRemainingMinutes int                  `json:"aiQueueRemainingMinutes"`
	AIEstimatedMinutes      int                  `json:"aiEstimatedMinutes"`
	AIEstimatedMinMinutes   int                  `json:"aiEstimatedMinMinutes"`
	AIEstimatedMaxMinutes   int                  `json:"aiEstimatedMaxMinutes"`
	AIHistoryMedianMinutes  int                  `json:"aiHistoryMedianMinutes"`
	AIAssigneeSpeedFactor   float64              `json:"-"`
	AISLAMarginMinutes      int                  `json:"aiSlaMarginMinutes"`
	AITopFactors            []FactorContribution `json:"aiTopFactors,omitempty"`
	AILowConfidence         bool                 `json:"aiLowConfidence"`
	AIFallbackUsed          bool                 `json:"aiFallbackUsed"`
	AIModelVersion          string               `json:"aiModelVersion"`
	AIAltAssignee           string               `json:"-"`
	AIAltEstimatedMinutes   int                  `json:"-"`

	// Manual Override (F-12) & Actual Completion Error (F-13)
	ManualOverrideMinutes  *int   `json:"manualOverrideMinutes,omitempty"`
	ManualOverrideReason   string `json:"manualOverrideReason,omitempty"`
	ActualDurationMinutes  *int   `json:"actualDurationMinutes,omitempty"`
	PredictionErrorMinutes *int   `json:"predictionErrorMinutes,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type GroupScore struct {
	Group            string  `json:"group"`
	Confidence       float64 `json:"confidence"`
	EstimatedMinutes int     `json:"estimatedMinutes"`
	SLARisk          float64 `json:"slaRisk"`
	Workload         int     `json:"workload"`
}

// SSEMessage represents events streamed to frontend
type SSEMessage struct {
	Type      string    `json:"type"`
	Incident  *Incident `json:"incident,omitempty"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// DashboardStats provides quick summary counters for the dashboard
type DashboardStats struct {
	Total                int `json:"total"`
	Critical             int `json:"critical"`
	AssignedCoordinators int `json:"assignedCoordinators"`
	HighPriority         int `json:"highPriority"`
	UnderInvestigation   int `json:"underInvestigation"`
	Active               int `json:"active"`
	Resolved             int `json:"resolved"`
	SLARiskCount         int `json:"slaRiskCount"`
}

// PredictRequest mirrors the API contract for POST /api/v1/predict
type PredictRequest struct {
	TicketID          string  `json:"ticket_id"`
	Category          string  `json:"category"`
	Priority          string  `json:"priority"`
	PriorityID        int     `json:"priority_id,omitempty"`
	AssigneeID        string  `json:"assignee_id,omitempty"`
	AssignedGroupName string  `json:"assigned_group_name,omitempty"`
	CreatedAt         string  `json:"created_at"`
	SLATargetHours    float64 `json:"sla_target_hours"`
	TitleText         string  `json:"title_text"`
	Description       string  `json:"description,omitempty"`
	ServiceType       string  `json:"service_type,omitempty"`
}

// PredictResponse returns regression forecast, CI, SLA risk, top factors, and team recommendation
type PredictResponse struct {
	TicketID              string               `json:"ticket_id"`
	PredictedTimeHours    float64              `json:"predicted_time_hours"`
	TotalWaitHours        float64              `json:"total_wait_hours"`
	PredictedFinishTime   string               `json:"predicted_finish_time"`
	ConfidenceInterval    [2]float64           `json:"confidence_interval"`
	SLAStatus             string               `json:"sla_status"`
	SLARiskScore          float64              `json:"sla_risk_score"`
	CurrentAssigneeLoad   int                  `json:"-"`
	TeamWorkload          int                  `json:"team_workload"`
	QueueRemainingHours   float64              `json:"queue_remaining_hours"`
	HistoryCategoryMedian float64              `json:"history_category_median"`
	RecommendedGroup      string               `json:"recommended_group,omitempty"`
	RecommendedAssignee   string               `json:"-"`
	RecommendedTotalHours float64              `json:"recommended_total_hours,omitempty"`
	TopFactors            []FactorContribution `json:"top_factors"`
	ModelVersion          string               `json:"model_version"`
	Fallback              bool                 `json:"fallback"`
	LowConfidence         bool                 `json:"low_confidence"`
	LatencyMs             float64              `json:"latency_ms"`
}

// MLMetricsResponse exposes model validation metrics and live DB telemetry
type MLMetricsResponse struct {
	ModelName          string             `json:"modelName"`
	ModelVersion       string             `json:"modelVersion"`
	Features           []string           `json:"features"`
	FeatureImportances map[string]float64 `json:"featureImportances"`
	OfflineMetrics     RegressionMetrics  `json:"offlineMetrics"`
	LiveMetrics        LiveTelemetry      `json:"liveMetrics"`
}

type RegressionMetrics struct {
	MAEHours           float64 `json:"maeHours"`
	RMSEHours          float64 `json:"rmseHours"`
	MAPEPercent        float64 `json:"mapePercent"`
	R2                 float64 `json:"r2"`
	SLAPrecision       float64 `json:"slaPrecision"`
	ResidualStdHours   float64 `json:"residualStdHours"`
	TrainSamples       int     `json:"trainSamples"`
	TestSamples        int     `json:"testSamples"`
	TotalClosedSamples int     `json:"totalClosedSamples"`
}

type LiveTelemetry struct {
	TotalIncidents       int     `json:"totalIncidents"`
	ClosedWithActualTime int     `json:"closedWithActualTime"`
	LiveMAEHours         float64 `json:"liveMaeHours"`
	LiveMAPEPercent      float64 `json:"liveMapePercent"`
	TotalPredictions     int     `json:"totalPredictions"`
	EvaluatedPredictions int     `json:"evaluatedPredictions"`
	RoutingAcceptancePct float64 `json:"routingAcceptancePct"`
}
