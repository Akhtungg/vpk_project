import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Menu } from 'lucide-react'
import { SlaBreachColumn } from './components/SlaBreachColumn'
import { ColumnGroup } from './components/ColumnGroup'
import { DemoPanel } from './components/DemoPanel'
import { EmergencyAlertBanner, type EmergencyAlertItem } from './components/EmergencyAlertBanner'
import { Header } from './components/Header'
import { IncidentDetailModal } from './components/IncidentDetailModal'
import { IncidentTableView } from './components/IncidentTableView'
import { MLMetricsModal } from './components/MLMetricsModal'
import { OverviewDashboard } from './components/OverviewDashboard'
import { ToolbarFilters } from './components/ToolbarFilters'
import { AlertToast } from './components/AlertToast'
import { getIncidentById, getIncidents, getStats, getTeams, updateIncident } from './services/api'
import { sseService } from './services/sse'
import type { DashboardStats, Incident, NotificationItem, TeamSummary } from './types/incident'
import { soundAlert } from './utils/audio'

type ViewMode = 'kanban' | 'overview' | 'table'

export function App() {
  const [incidents, setIncidents] = useState<Incident[]>([])
  const [teamsData, setTeamsData] = useState<TeamSummary[]>([])
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [search, setSearch] = useState('')
  const [dateFilter, setDateFilter] = useState('all')
  const [productFilter, setProductFilter] = useState('')
  const [teamFilter, setTeamFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [viewMode, setViewMode] = useState<ViewMode>('kanban')
  const [soundEnabled, setSoundEnabled] = useState(true)
  const [connected, setConnected] = useState(false)
  const [selectedIncident, setSelectedIncident] = useState<Incident | null>(null)
  const [mlModalOpen, setMlModalOpen] = useState(false)
  const [notifications, setNotifications] = useState<NotificationItem[]>([])
  const [toasts, setToasts] = useState<NotificationItem[]>([])
  const [emergencyAlerts, setEmergencyAlerts] = useState<EmergencyAlertItem[]>([])
  const [nowMs, setNowMs] = useState(() => Date.now())
  const hasInitialAlertsLoaded = useRef(false)

  useEffect(() => {
    const timer = setInterval(() => setNowMs(Date.now()), 30000)
    return () => clearInterval(timer)
  }, [])

  const addEmergencyAlert = useCallback((inc: Incident) => {
    setEmergencyAlerts((prev) => {
      const existingIdx = prev.findIndex((a) => a.id === inc.id)
      if (existingIdx !== -1) {
        const next = [...prev]
        next[existingIdx] = { ...next[existingIdx], incident: inc, appearedAt: Date.now() }
        return next
      }
      if (prev.length < 6) {
        return [...prev, { id: inc.id, incident: inc, appearedAt: Date.now() }]
      }
      const currentAlerts = prev.slice(0, 6)
      let oldestIdx = 0
      for (let i = 1; i < currentAlerts.length; i++) {
        if (currentAlerts[i].appearedAt < currentAlerts[oldestIdx].appearedAt) {
          oldestIdx = i
        }
      }
      const withoutOldest = currentAlerts.filter((_, idx) => idx !== oldestIdx)
      return [...withoutOldest, { id: inc.id, incident: inc, appearedAt: Date.now() }]
    })
  }, [])

  const handleDismissEmergencyAlert = useCallback((id: string) => {
    setEmergencyAlerts((prev) => prev.filter((a) => a.id !== id))
  }, [])

  const addNotification = useCallback(
    (
      type: NotificationItem['type'],
      title: string,
      message: string,
      incidentId?: string
    ) => {
      const item: NotificationItem = {
        id: `notif_${Date.now()}_${Math.random().toString(36).substring(2, 7)}`,
        type,
        title,
        message,
        timestamp: new Date().toISOString(),
        read: false,
        incidentId,
      }
      setNotifications((prev) => [item, ...prev.slice(0, 49)])
      setToasts((prev) => [item, ...prev.slice(0, 2)])
    },
    []
  )

  const loadData = useCallback(async () => {
    try {
      const [incList, statsData, teamList] = await Promise.all([
        getIncidents(),
        getStats(),
        getTeams().catch(() => []),
      ])
      setIncidents(incList)
      setStats(statsData)
      setTeamsData(teamList)

      if (!hasInitialAlertsLoaded.current) {
        hasInitialAlertsLoaded.current = true
        const criticals = incList.filter(
          (i) =>
            (i.priorityId === 4 || i.priority === 'Наивысший' || i.priority === 'Критический') &&
            i.state !== 'Завершен' &&
            i.state !== 'Закрыт'
        )
        if (criticals.length > 0) {
          setEmergencyAlerts(
            criticals.slice(0, 6).map((inc) => ({
              id: inc.id,
              incident: inc,
              appearedAt: Date.now(),
            }))
          )
        }
      } else {
        setEmergencyAlerts((prev) =>
          prev.filter((a) => {
            const found = incList.find((i) => i.id === a.id)
            return !found || (found.state !== 'Завершен' && found.state !== 'Закрыт')
          })
        )
      }
    } catch (e) {
      console.error('Failed to load dashboard data:', e)
    }
  }, [])

  const handleSelectIncidentById = useCallback(
    async (id: string) => {
      let found = incidents.find(
        (i) => i.id === id || i.ticketNumber === id || i.sysId === id
      )
      if (!found) {
        try {
          found = await getIncidentById(id)
        } catch (err) {
          console.warn('[App] Could not fetch incident by ID from API', id, err)
        }
      }
      if (found) {
        setSelectedIncident(found)
        setNotifications((prev) =>
          prev.map((n) =>
            n.incidentId === id || n.message.includes(id) ? { ...n, read: true } : n
          )
        )
      }
    },
    [incidents]
  )

  useEffect(() => {
    loadData()

    sseService.connect()
    const unsubStatus = sseService.onStatusChange((status) => setConnected(status))

    const unsubMessages = sseService.subscribe((msg) => {
      const incId = msg.incident?.id || msg.message.match(/INC\d+/)?.[0]

      if (msg.type === 'CRITICAL_ALERT') {
        soundAlert.playCriticalAlert()
        addNotification('CRITICAL_ALERT', 'Критический инцидент', msg.message, incId)
        if (msg.incident) {
          addEmergencyAlert(msg.incident)
        } else if (incId) {
          getIncidentById(incId)
            .then((inc) => {
              if (inc) addEmergencyAlert(inc)
            })
            .catch(() => {})
        }
      } else if (msg.type === 'COORDINATOR_ALERT') {
        soundAlert.playCoordinatorAlert()
        addNotification('COORDINATOR_ALERT', 'Назначение координатору', msg.message, incId)
      } else if (msg.type === 'INCIDENT_RESOLVED') {
        soundAlert.playResolvedAlert()
        addNotification('INCIDENT_RESOLVED', 'Инцидент решен', msg.message, incId)
        if (incId) {
          handleDismissEmergencyAlert(incId)
        }
      } else if (msg.type === 'INCIDENT_UPDATED') {
        addNotification('INCIDENT_UPDATED', 'Обновление инцидента', msg.message, incId)
        if (
          msg.incident &&
          (msg.incident.priorityId === 4 ||
            msg.incident.priority === 'Наивысший' ||
            msg.incident.priority === 'Критический')
        ) {
          if (msg.incident.state !== 'Завершен' && msg.incident.state !== 'Закрыт') {
            addEmergencyAlert(msg.incident)
          } else {
            handleDismissEmergencyAlert(msg.incident.id)
          }
        }
      } else if (msg.type === 'DASHBOARD_RESET') {
        hasInitialAlertsLoaded.current = false
        addNotification('INFO', 'Сброс дашборда', 'Дашборд сброшен к исходным данным')
      }

      loadData()
    })

    return () => {
      unsubStatus()
      unsubMessages()
      sseService.disconnect()
    }
  }, [addNotification, addEmergencyAlert, handleDismissEmergencyAlert, loadData])

  const handleToggleSound = () => {
    const next = !soundEnabled
    setSoundEnabled(next)
    soundAlert.enabled = next
  }

  const handleQuickReassign = async (inc: Incident, targetGroup: string) => {
    try {
      await updateIncident(inc.id, { assignedGroupName: targetGroup })
      addNotification(
        'REASSIGNED',
        'Перенаправление',
        `Инцидент #${inc.id} перенаправлен в команду "${targetGroup}"`,
        inc.id
      )
      await loadData()
    } catch (e) {
      alert('Ошибка при переназначении: ' + e)
    }
  }

  const handleUpdateIncident = async (
    id: string,
    updates: Partial<Incident> & { manualOverrideMinutes?: number; manualOverrideReason?: string }
  ) => {
    const updated = await updateIncident(id, updates)
    setSelectedIncident(updated)
    if (updates.state) {
      addNotification(
        'STATE_CHANGED',
        'Смена статуса',
        `Инцидент #${id}: статус изменен на "${updates.state}"`,
        id
      )
    }
    if (updates.assignedGroupName) {
      addNotification(
        'REASSIGNED',
        'Перенаправление',
        `Инцидент #${id}: назначен в команду "${updates.assignedGroupName}"`,
        id
      )
    }
    await loadData()
  }

  const uniqueProducts = useMemo(() => {
    const set = new Set<string>()
    incidents.forEach((i) => {
      if (i.productName) set.add(i.productName)
      if (i.service) set.add(i.service)
    })
    return Array.from(set).sort()
  }, [incidents])

  const uniqueTeams = useMemo(() => {
    const set = new Set<string>()
    incidents.forEach((i) => {
      if (i.assignedGroupName) set.add(i.assignedGroupName)
    })
    return Array.from(set).sort()
  }, [incidents])

  const uniqueStatuses = useMemo(() => {
    const set = new Set<string>()
    incidents.forEach((i) => {
      if (i.state) set.add(i.state)
    })
    return Array.from(set).sort()
  }, [incidents])

  const filteredIncidents = useMemo(() => {
    return incidents.filter((inc) => {
      if (search.trim()) {
        const q = search.toLowerCase()
        const matchesSearch =
          inc.id.toLowerCase().includes(q) ||
          (inc.ticketNumber && inc.ticketNumber.toLowerCase().includes(q)) ||
          inc.header.toLowerCase().includes(q) ||
          (inc.productName && inc.productName.toLowerCase().includes(q)) ||
          (inc.assignedGroupName && inc.assignedGroupName.toLowerCase().includes(q))
        if (!matchesSearch) return false
      }

      if (productFilter) {
        if (inc.productName !== productFilter && inc.service !== productFilter) {
          return false
        }
      }

      if (teamFilter) {
        if (inc.assignedGroupName !== teamFilter) {
          return false
        }
      }

      if (statusFilter) {
        const normFilter = statusFilter.trim()
        const normState = inc.state ? inc.state.trim() : ''
        if (normFilter === 'Завершен' || normFilter === 'Закрыт') {
          if (normState !== 'Завершен' && normState !== 'Закрыт') return false
        } else if (normState !== normFilter) {
          return false
        }
      }

      if (dateFilter !== 'all' && inc.submitDate) {
        const submit = new Date(inc.submitDate).getTime()
        if (dateFilter === 'today') {
          const startOfToday = new Date().setHours(0, 0, 0, 0)
          if (submit < startOfToday) return false
        } else if (dateFilter === '24h') {
          if (nowMs - submit > 24 * 3600 * 1000) return false
        } else if (dateFilter === '7d') {
          if (nowMs - submit > 7 * 24 * 3600 * 1000) return false
        }
      }

      return true
    })
  }, [incidents, search, productFilter, teamFilter, statusFilter, dateFilter, nowMs])

  const activeFiltersCount = useMemo(() => {
    let count = 0
    if (dateFilter !== 'all') count++
    if (productFilter) count++
    if (teamFilter) count++
    if (statusFilter) count++
    return count
  }, [dateFilter, productFilter, teamFilter, statusFilter])

  const handleClearFilters = () => {
    setDateFilter('all')
    setProductFilter('')
    setTeamFilter('')
    setStatusFilter('')
    setSearch('')
  }

  // Sort by SLA risk & priority
  const stableSort = (cards: Incident[]) => {
    return [...cards].sort((a, b) => {
      const slaA = a.aiSlaRisk ?? 0
      const slaB = b.aiSlaRisk ?? 0
      if (slaB !== slaA) return slaB - slaA
      if (b.priorityId !== a.priorityId) return b.priorityId - a.priorityId
      return (b.id || '').localeCompare(a.id || '')
    })
  }

  const isExcludedState = useCallback(
    (inc: Incident) => {
      if (statusFilter) return false
      return inc.state === 'Завершен' || inc.state === 'Закрыт' || inc.state === 'Решен'
    },
    [statusFilter]
  )

  const isCoordinator = (inc: Incident) =>
    Boolean(inc.assignedGroupName && inc.assignedGroupName.includes('Координатор')) ||
    inc.state === 'Назначен координатору'

  const criticalGroup = useMemo(() => {
    const list = filteredIncidents.filter(
      (inc) =>
        (inc.priorityId === 4 || inc.priority === 'Наивысший' || inc.priority === 'Критический') &&
        !isExcludedState(inc)
    )
    return stableSort(list)
  }, [filteredIncidents, isExcludedState])

  const coordinatorGroup = useMemo(() => {
    const list = filteredIncidents.filter(
      (inc) =>
        isCoordinator(inc) &&
        inc.priorityId !== 4 &&
        inc.priority !== 'Наивысший' &&
        inc.priority !== 'Критический' &&
        !isExcludedState(inc)
    )
    return stableSort(list)
  }, [filteredIncidents, isExcludedState])

  const highPriorityGroup = useMemo(() => {
    const list = filteredIncidents.filter(
      (inc) =>
        (inc.priorityId === 3 || inc.priority === 'Высокий') &&
        !isCoordinator(inc) &&
        !isExcludedState(inc)
    )
    return stableSort(list)
  }, [filteredIncidents, isExcludedState])

  const diagnosisGroup = useMemo(() => {
    const list = filteredIncidents.filter(
      (inc) =>
        (inc.state === 'Исследование ситуации' || inc.stateId === 3 || inc.priorityId <= 2) &&
        inc.priorityId !== 4 &&
        inc.priority !== 'Наивысший' &&
        inc.priority !== 'Критический' &&
        !isCoordinator(inc) &&
        !isExcludedState(inc)
    )
    return stableSort(list)
  }, [filteredIncidents, isExcludedState])

  const slaOverdueGroup = useMemo(() => {
    const list = filteredIncidents.filter((inc) => {
      if (inc.state === 'Завершен' || inc.state === 'Закрыт' || inc.state === 'Решен') return false
      if (!inc.deadline) return false
      return new Date(inc.deadline).getTime() < nowMs
    })
    return list.sort((a, b) => {
      const deadA = a.deadline ? new Date(a.deadline).getTime() : 0
      const deadB = b.deadline ? new Date(b.deadline).getTime() : 0
      if (deadA !== deadB) return deadA - deadB
      return (b.aiSlaRisk ?? 100) - (a.aiSlaRisk ?? 100)
    })
  }, [filteredIncidents, nowMs])

  return (
    <div className="h-screen w-screen bg-[#F8FAFC] text-slate-900 flex flex-col overflow-hidden font-sans select-none antialiased">
      {/* Top Header */}
      <Header
        connected={connected}
        search={search}
        onSearchChange={setSearch}
        soundEnabled={soundEnabled}
        onToggleSound={handleToggleSound}
        stats={stats}
        notifications={notifications}
        onMarkAllRead={() => setNotifications((prev) => prev.map((n) => ({ ...n, read: true })))}
        onClearNotifications={() => setNotifications([])}
        onSelectIncidentById={handleSelectIncidentById}
        onOpenMLMetrics={() => setMlModalOpen(true)}
      />

      {/* Main Workspace Layout with Narrow Pill Sidebar */}
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-24 sm:w-28 bg-white border-r border-slate-200/80 py-4 px-2.5 flex flex-col items-center gap-2.5 flex-shrink-0 select-none">
          <button
            className="p-1.5 text-slate-700 hover:text-slate-950 rounded-lg hover:bg-slate-100 mb-1 cursor-pointer transition-colors"
            title="Меню"
          >
            <Menu className="h-5 w-5" />
          </button>
          <button
            onClick={() => setViewMode('overview')}
            className={`w-full py-1.5 px-3 rounded-full text-xs text-center transition-all cursor-pointer ${
              viewMode === 'overview'
                ? 'bg-[#E30611] text-white font-bold shadow-xs'
                : 'bg-white border border-[#E30611] text-[#E30611] font-semibold hover:bg-red-50'
            }`}
          >
            Обзор
          </button>
          <button
            onClick={() => setViewMode('kanban')}
            className={`w-full py-1.5 px-3 rounded-full text-xs text-center transition-all cursor-pointer ${
              viewMode === 'kanban'
                ? 'bg-[#E30611] text-white font-bold shadow-xs'
                : 'bg-white border border-[#E30611] text-[#E30611] font-semibold hover:bg-red-50'
            }`}
          >
            Инциденты
          </button>
          <button
            onClick={() => setViewMode('table')}
            className={`w-full py-1.5 px-3 rounded-full text-xs text-center transition-all cursor-pointer ${
              viewMode === 'table'
                ? 'bg-[#E30611] text-white font-bold shadow-xs'
                : 'bg-white border border-[#E30611] text-[#E30611] font-semibold hover:bg-red-50'
            }`}
          >
            Отчет
          </button>
        </aside>

        {/* Content Area */}
        <div className="flex-1 flex flex-col overflow-hidden px-4 md:px-6 pt-3 pb-3">
          <EmergencyAlertBanner
            alerts={emergencyAlerts}
            onOpen={setSelectedIncident}
            onDismiss={handleDismissEmergencyAlert}
          />

          {viewMode !== 'overview' && (
            <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3 pb-2.5 border-b border-slate-200/80 mb-3 flex-shrink-0">
              <ToolbarFilters
                title={viewMode === 'table' ? 'Отчет по инцидентам' : 'Инциденты'}
                dateFilter={dateFilter}
                onDateFilterChange={setDateFilter}
                productFilter={productFilter}
                onProductFilterChange={setProductFilter}
                products={uniqueProducts}
                teamFilter={teamFilter}
                onTeamFilterChange={setTeamFilter}
                teams={uniqueTeams}
                statusFilter={statusFilter}
                onStatusFilterChange={setStatusFilter}
                statuses={uniqueStatuses}
                onClearFilters={handleClearFilters}
                activeFiltersCount={activeFiltersCount}
              />
            </div>
          )}

          {/* View 1: 5-Column Kanban */}
          {viewMode === 'kanban' && (
            <main className="flex-1 overflow-hidden">
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-3 h-full">
                <ColumnGroup
                  title="Критические"
                  count={criticalGroup.length}
                  headerBg="bg-[#FEE2E2]"
                  headerTextClass="text-[#991B1B]"
                  badgeBg="bg-[#E30611] text-white"
                  incidents={criticalGroup}
                  onCardClick={setSelectedIncident}
                  onQuickReassign={handleQuickReassign}
                />

                <ColumnGroup
                  title="На координаторах"
                  count={coordinatorGroup.length}
                  headerBg="bg-[#FEF3C7]"
                  headerTextClass="text-[#92400E]"
                  badgeBg="bg-[#D97706] text-white"
                  incidents={coordinatorGroup}
                  onCardClick={setSelectedIncident}
                  onQuickReassign={handleQuickReassign}
                />

                <ColumnGroup
                  title="Высокий приоритет"
                  count={highPriorityGroup.length}
                  headerBg="bg-[#FFEDD5]"
                  headerTextClass="text-[#9A3412]"
                  badgeBg="bg-[#EA580C] text-white"
                  incidents={highPriorityGroup}
                  onCardClick={setSelectedIncident}
                  onQuickReassign={handleQuickReassign}
                />

                <ColumnGroup
                  title="Исследование ситуации"
                  count={diagnosisGroup.length}
                  headerBg="bg-[#E0F2FE]"
                  headerTextClass="text-[#075985]"
                  badgeBg="bg-[#0284C7] text-white"
                  incidents={diagnosisGroup}
                  onCardClick={setSelectedIncident}
                  onQuickReassign={handleQuickReassign}
                />

                <SlaBreachColumn
                  incidents={slaOverdueGroup}
                  onCardClick={setSelectedIncident}
                  onQuickReassign={handleQuickReassign}
                />
              </div>
            </main>
          )}

          {/* View 2: Overview */}
          {viewMode === 'overview' && (
            <main className="flex-1 overflow-y-auto pr-1">
              <OverviewDashboard
                stats={stats}
                incidents={filteredIncidents}
                teamsData={teamsData}
                onSelectIncident={setSelectedIncident}
                onSelectTeam={(team) => {
                  setTeamFilter(team)
                  setViewMode('table')
                }}
              />
            </main>
          )}

          {/* View 3: Excel-style Table */}
          {viewMode === 'table' && (
            <main className="flex-1 overflow-y-auto pr-1">
              <IncidentTableView
                incidents={stableSort(filteredIncidents)}
                teamsData={teamsData}
                onSelectIncident={setSelectedIncident}
                onQuickReassign={handleQuickReassign}
                selectedTeamFilter={teamFilter}
                onSelectTeamFilter={setTeamFilter}
              />
            </main>
          )}
        </div>
      </div>

      {/* Incident Detail Modal */}
      <IncidentDetailModal
        incident={selectedIncident}
        allIncidents={incidents}
        onClose={() => setSelectedIncident(null)}
        onUpdate={handleUpdateIncident}
        teams={uniqueTeams}
        onSelectIncident={setSelectedIncident}
      />

      {/* ML Regression Model Metrics & Predictor Modal */}
      <MLMetricsModal open={mlModalOpen} onClose={() => setMlModalOpen(false)} />

      {/* Live Notifications Toasts */}
      <AlertToast
        toasts={toasts}
        onDismiss={(id) => setToasts((prev) => prev.filter((t) => t.id !== id))}
        onSelectIncidentById={handleSelectIncidentById}
      />

      {/* Live ITSM Simulation Panel */}
      <DemoPanel onRefresh={loadData} />
    </div>
  )
}

export default App
