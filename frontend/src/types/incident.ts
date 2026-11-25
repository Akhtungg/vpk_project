export interface FactorContribution {
  factor: string
  impactHours: number
}

export interface Incident {
  id: string
  sysId?: string
  state: string
  stateEnum?: string
  stateId: number
  stateReason?: string
  stateReasonId?: string
  submitDate?: string
  deadline?: string
  completionDate?: string
  lastAssignmentToTeamDateTime?: string
  header: string
  description: string
  shortDescription?: string
  assignedGroupName: string
  assignedEmployee?: string
  assignedEmployeeLogin?: string
  initiatorName?: string
  initiatorLogin?: string
  isEffect?: boolean
  isReason?: boolean
  isDouble?: boolean
  isMaster?: boolean
  serviceTypeId?: number
  serviceType?: string
  priorityId: number
  priority: string
  urgencyId?: number
  urgency?: string
  impactId?: number
  impact?: string
  productName?: string
  productId?: string
  productMasterId?: string
  productUrl?: string
  service?: string
  serviceId?: string
  scenarioName?: string
  isNeedUp?: boolean
  solution?: string
  message?: string
  isAccepted?: boolean

  // Unified Regression ML Enrichments (Routing, Execution Time, Queue & SLA Risk)
  aiRoutingHint: string
  aiRoutingConfidence: number
  aiRoutingTop?: Array<{
    group: string
    confidence: number
    estimatedMinutes?: number
    slaRisk?: number
    workload?: number
  }>
  aiSlaRisk?: number
  aiSlaLevel?: 'SAFE' | 'WARNING' | 'CRITICAL'
  aiTeamWorkload?: number
  aiAssigneeWorkload?: number
  aiQueueRemainingMinutes?: number
  aiEstimatedMinutes?: number
  aiEstimatedMinMinutes?: number
  aiEstimatedMaxMinutes?: number
  aiHistoryMedianMinutes?: number
  aiAssigneeSpeedFactor?: number
  aiSlaMarginMinutes?: number
  aiTopFactors?: FactorContribution[]
  aiLowConfidence?: boolean
  aiFallbackUsed?: boolean
  aiModelVersion?: string
  aiAltAssignee?: string
  aiAltEstimatedMinutes?: number

  // Manual Override (F-12) & Actual Completion Error (F-13)
  manualOverrideMinutes?: number | null
  manualOverrideReason?: string
  actualDurationMinutes?: number | null
  predictionErrorMinutes?: number | null

  createdAt?: string
  updatedAt?: string
}

export interface DashboardStats {
  total: number
  critical: number
  assignedCoordinators: number
  highPriority: number
  underInvestigation: number
  active: number
  resolved: number
  slaRiskCount?: number
}

export interface SSEMessage {
  type: 'INCIDENT_CREATED' | 'INCIDENT_UPDATED' | 'CRITICAL_ALERT' | 'COORDINATOR_ALERT' | 'INCIDENT_RESOLVED' | 'DASHBOARD_RESET'
  incident?: Incident
  message: string
  timestamp: string
}

export interface PredictRequest {
  ticket_id: string
  category: string
  priority: string
  assignee_id: string
  assigned_group_name?: string
  sla_target_hours: number
  title_text: string
  description?: string
  service_type?: string
}

export interface PredictResponse {
  ticket_id: string
  predicted_time_hours: number
  total_wait_hours: number
  predicted_finish_time: string
  confidence_interval: [number, number]
  sla_status: 'OK' | 'WARNING' | 'CRITICAL_SLA_BREACH'
  sla_risk_score: number
  current_assignee_load: number
  team_workload: number
  queue_remaining_hours: number
  history_category_median: number
  recommended_group?: string
  recommended_assignee?: string
  recommended_total_hours?: number
  top_factors: FactorContribution[]
  model_version: string
  fallback: boolean
  low_confidence: boolean
  latency_ms: number
}

export interface MLMetricsResponse {
  modelName: string
  modelVersion: string
  features: string[]
  featureImportances: Record<string, number>
  offlineMetrics: {
    maeHours: number
    rmseHours: number
    mapePercent: number
    r2: number
    slaPrecision: number
    residualStdHours: number
    trainSamples: number
    testSamples: number
    totalClosedSamples: number
  }
  liveMetrics: {
    totalIncidents: number
    closedWithActualTime: number
    liveMaeHours: number
    liveMapePercent: number
    totalPredictions: number
    evaluatedPredictions: number
    routingAcceptancePct: number
  }
}

export interface IncidentEvent {
  id: number
  incidentId: string
  eventType: 'STATE_CHANGED' | 'REASSIGNED' | 'CREATED' | 'RESOLVED' | 'AI_ROUTING_RECOMMENDED' | string
  message: string
  payload?: unknown
  createdAt: string
}

export interface NotificationItem {
  id: string
  type: 'CRITICAL_ALERT' | 'COORDINATOR_ALERT' | 'INCIDENT_RESOLVED' | 'INCIDENT_UPDATED' | 'STATE_CHANGED' | 'REASSIGNED' | 'INFO'
  title: string
  message: string
  timestamp: string
  read: boolean
  incidentId?: string
}
