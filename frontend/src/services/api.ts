import type {
  DashboardStats,
  Incident,
  MLMetricsResponse,
  PredictRequest,
  PredictResponse,
  TeamSummary,
} from '../types/incident'

const API_BASE = '/api'

export async function getIncidents(search = '', group = ''): Promise<Incident[]> {
  const params = new URLSearchParams()
  if (search) params.append('search', search)
  if (group) params.append('group', group)

  const res = await fetch(`${API_BASE}/incidents?${params.toString()}`)
  if (!res.ok) throw new Error('Failed to fetch incidents')
  const data = await res.json()
  return Array.isArray(data) ? data : []
}

export async function getTeams(): Promise<TeamSummary[]> {
  const res = await fetch(`${API_BASE}/teams`)
  if (!res.ok) throw new Error('Failed to fetch teams')
  const data = await res.json()
  return Array.isArray(data) ? data : []
}

export async function getIncidentById(id: string): Promise<Incident> {
  const res = await fetch(`${API_BASE}/incidents/${id}`)
  if (!res.ok) throw new Error('Incident not found')
  return res.json()
}

export async function updateIncident(id: string, data: Partial<Incident>): Promise<Incident> {
  const res = await fetch(`${API_BASE}/incidents/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error('Failed to update incident')
  return res.json()
}

export async function getStats(): Promise<DashboardStats> {
  const res = await fetch(`${API_BASE}/stats`)
  if (!res.ok) throw new Error('Failed to fetch stats')
  return res.json()
}

export async function getMLMetrics(): Promise<MLMetricsResponse> {
  const res = await fetch(`${API_BASE}/ml/metrics`)
  if (!res.ok) throw new Error('Failed to fetch ML metrics')
  return res.json()
}

export async function predictTicket(payload: PredictRequest): Promise<PredictResponse> {
  const res = await fetch(`${API_BASE}/v1/predict`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const errText = await res.text()
    throw new Error(errText || 'Prediction failed')
  }
  return res.json()
}

export function getExportCSVUrl(): string {
  return `${API_BASE}/incidents/export`
}

// Demo endpoints for live presentation
export async function demoCreateCritical(): Promise<Incident> {
  const res = await fetch(`${API_BASE}/demo/create-critical`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to simulate critical')
  return res.json()
}

export async function demoAssignCoordinator(): Promise<Incident> {
  const res = await fetch(`${API_BASE}/demo/assign-coordinator`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to simulate coordinator assignment')
  return res.json()
}

export async function demoResolveCritical(id?: string): Promise<Incident> {
  const url = id ? `${API_BASE}/demo/resolve-critical?id=${id}` : `${API_BASE}/demo/resolve-critical`
  const res = await fetch(url, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to simulate resolution')
  return res.json()
}

export async function demoResetData(): Promise<void> {
  const res = await fetch(`${API_BASE}/demo/reset`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to reset data')
}

export async function getIncidentEvents(id: string): Promise<import('../types/incident').IncidentEvent[]> {
  try {
    const res = await fetch(`${API_BASE}/incidents/${encodeURIComponent(id)}/events`)
    if (res.ok) {
      const data = await res.json()
      if (Array.isArray(data)) {
        const uniqueEvents: import('../types/incident').IncidentEvent[] = []
        const seen = new Set<string>()
        for (const evt of data) {
          const timeSec = evt.createdAt ? new Date(evt.createdAt).toISOString().substring(0, 19) : ''
          const key = `${evt.eventType}_${evt.message}_${timeSec}`
          if (!seen.has(key)) {
            seen.add(key)
            uniqueEvents.push(evt)
          }
        }
        return uniqueEvents.sort(
          (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
        )
      }
    }
  } catch (err) {
    console.warn('[API] Could not fetch incident events from backend', err)
  }
  return []
}
