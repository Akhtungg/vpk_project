import React, { useMemo } from 'react'
import { Clock } from 'lucide-react'
import type { DashboardStats, Incident, TeamSummary } from '../types/incident'

interface OverviewDashboardProps {
  stats: DashboardStats | null
  incidents: Incident[]
  teamsData?: TeamSummary[]
  onSelectIncident?: (inc: Incident) => void
  onSelectTeam?: (team: string) => void
}

export const OverviewDashboard: React.FC<OverviewDashboardProps> = ({
  stats,
  incidents,
  teamsData = [],
  onSelectIncident,
  onSelectTeam,
}) => {
  const activePool = useMemo(
    () => incidents.filter((i) => i.state !== 'Завершен' && i.state !== 'Закрыт'),
    [incidents]
  )
  const total = incidents.length || 0

  // 1. Priority counts (over active incidents)
  const activeTotal = activePool.length || 0
  const p4Count = activePool.filter(
    (i) => i.priorityId === 4 || i.priority === 'Наивысший' || i.priority === 'Критический'
  ).length
  const p3Count = activePool.filter(
    (i) => i.priorityId === 3 || i.priority === 'Высокий'
  ).length
  const p2Count = activePool.filter(
    (i) => i.priorityId === 2 || i.priority === 'Средний'
  ).length
  const p1Count = activePool.filter(
    (i) => i.priorityId === 1 || i.priority === 'Низкий'
  ).length

  const sumPriorities = activeTotal || 1
  const priorityBars = [
    {
      label: 'Критические (P4)',
      count: p4Count,
      color: '#E30611',
      pct: activeTotal > 0 ? Math.round((p4Count / sumPriorities) * 100) : 0,
    },
    {
      label: 'Высокие (P3)',
      count: p3Count,
      color: '#F97316',
      pct: activeTotal > 0 ? Math.round((p3Count / sumPriorities) * 100) : 0,
    },
    {
      label: 'Средние (P2)',
      count: p2Count,
      color: '#EAB308',
      pct: activeTotal > 0 ? Math.round((p2Count / sumPriorities) * 100) : 0,
    },
    {
      label: 'Низкие (P1)',
      count: p1Count,
      color: '#22C55E',
      pct: activeTotal > 0 ? Math.round((p1Count / sumPriorities) * 100) : 0,
    },
  ]

  // 2. Statuses counts
  const sumAll = total || 1
  const newCount = incidents.filter(
    (i) => i.state?.trim() === 'Зарегистрирован' || i.state?.trim() === 'Новый' || !i.assignedGroupName
  ).length
  const inProgressCount = incidents.filter(
    (i) =>
      i.state?.trim() === 'Выполняется' ||
      i.state?.trim() === 'В работе' ||
      i.state?.trim() === 'Направлен в команду'
  ).length
  const investCount = incidents.filter(
    (i) =>
      i.state?.trim() === 'Исследование ситуации' ||
      i.state?.trim() === 'Диагностика' ||
      i.state?.trim() === 'Требуется информация'
  ).length
  const closedCount = incidents.filter(
    (i) => i.state?.trim() === 'Завершен' || i.state?.trim() === 'Закрыт'
  ).length

  const statusBars = [
    {
      label: 'Новые / Зарегистрированы',
      count: newCount,
      color: '#3B82F6',
      pct: total > 0 ? Math.round((newCount / sumAll) * 100) : 0,
    },
    {
      label: 'В работе / Направлены',
      count: inProgressCount,
      color: '#06B6D4',
      pct: total > 0 ? Math.round((inProgressCount / sumAll) * 100) : 0,
    },
    {
      label: 'На исследовании / Ожидание',
      count: investCount,
      color: '#A855F7',
      pct: total > 0 ? Math.round((investCount / sumAll) * 100) : 0,
    },
    {
      label: 'Закрытые / Завершённые',
      count: closedCount,
      color: '#64748B',
      pct: total > 0 ? Math.round((closedCount / sumAll) * 100) : 0,
    },
  ]

  // Active incidents list sorted by priority & SLA risk
  const activeIncidents = useMemo(() => {
    return [...activePool]
      .sort((a, b) => {
        if (b.priorityId !== a.priorityId) return b.priorityId - a.priorityId
        return (b.aiSlaRisk ?? 0) - (a.aiSlaRisk ?? 0)
      })
      .slice(0, 20)
  }, [activePool])

  // Team resolution statistics from normalized teams table (with client fallback)
  const teamLeaderboard = useMemo(() => {
    if (teamsData.length > 0) {
      return [...teamsData]
        .filter((t) => t.activeCount > 0 || t.totalCount > 0)
        .sort((a, b) => b.activeCount - a.activeCount || b.totalCount - a.totalCount)
        .slice(0, 8)
        .map((t) => ({
          id: t.id,
          team: t.name,
          active: t.activeCount,
          queue: t.queueCount,
          avgEstMin: t.avgEstimatedMinutes,
          avgSla: Math.round(t.avgSlaRisk),
        }))
    }
    const map = new Map<
      string,
      { team: string; active: number; queue: number; estSum: number; estCnt: number; slaSum: number; slaCnt: number }
    >()
    for (const inc of activePool) {
      const team = inc.assignedGroupName || 'Профильная команда'
      let item = map.get(team)
      if (!item) {
        item = { team, active: 0, queue: 0, estSum: 0, estCnt: 0, slaSum: 0, slaCnt: 0 }
        map.set(team, item)
      }
      item.active++
      if ((inc.aiTeamWorkload ?? 0) > item.queue) item.queue = inc.aiTeamWorkload ?? 0
      if (inc.aiEstimatedMinutes && inc.aiEstimatedMinutes > 0) {
        item.estSum += inc.aiEstimatedMinutes
        item.estCnt++
      }
      if (inc.aiSlaRisk != null) {
        item.slaSum += inc.aiSlaRisk
        item.slaCnt++
      }
    }
    return Array.from(map.values())
      .sort((a, b) => b.active - a.active)
      .slice(0, 8)
      .map((item) => ({
        id: item.team,
        team: item.team,
        active: item.active,
        queue: item.queue,
        avgEstMin: item.estCnt > 0 ? Math.round(item.estSum / item.estCnt) : 0,
        avgSla: item.slaCnt > 0 ? Math.round(item.slaSum / item.slaCnt) : 0,
      }))
  }, [activePool, teamsData])

  const getPriorityDot = (inc: Incident) => {
    if (inc.priorityId === 4 || inc.priority === 'Наивысший' || inc.priority === 'Критический') {
      return 'bg-[#E30611]'
    }
    if (inc.priorityId === 3 || inc.priority === 'Высокий') return 'bg-[#F97316]'
    if (inc.priorityId === 2 || inc.priority === 'Средний') return 'bg-[#EAB308]'
    return 'bg-[#22C55E]'
  }

  const formatSubmitTime = (dateStr?: string) => {
    if (!dateStr) return '—'
    try {
      const d = new Date(dateStr)
      return d.toLocaleString('ru-RU', {
        day: '2-digit',
        month: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return '—'
    }
  }

  const formatSlaRemaining = (deadlineStr?: string) => {
    if (!deadlineStr) return 'Не задан'
    try {
      const deadline = new Date(deadlineStr).getTime()
      const diffMs = deadline - Date.now()
      if (diffMs <= 0) return '00:00 (просрочен)'
      const hours = Math.floor(diffMs / (3600 * 1000))
      const mins = Math.floor((diffMs % (3600 * 1000)) / (60 * 1000))
      return `${String(hours).padStart(2, '0')}:${String(mins).padStart(2, '0')} до SLA`
    } catch {
      return 'Не задан'
    }
  }

  return (
    <div className="space-y-6 pb-6 select-none animate-in fade-in duration-150">
      {/* Title & Subtitle */}
      <div>
        <h1 className="text-2xl sm:text-3xl font-extrabold text-slate-900 tracking-tight">
          Мониторинг заявок
        </h1>
        <p className="text-xs sm:text-sm text-slate-400 mt-0.5">
          прогнозирование времени выполнения, контроль SLA и координация инцидентов
        </p>
      </div>

      {/* 4 KPI Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-white border border-slate-200/90 rounded-2xl p-4 sm:p-5 shadow-xs">
          <span className="text-xs font-semibold text-slate-500 block">
            Критические инциденты
          </span>
          <div className="mt-2 text-2xl sm:text-3xl font-bold text-slate-900">
            {stats?.critical !== undefined ? stats.critical : p4Count}
          </div>
          <span className="text-[11px] font-medium text-slate-400 mt-2 block">Приоритет P4</span>
        </div>

        <div className="bg-white border border-slate-200/90 rounded-2xl p-4 sm:p-5 shadow-xs">
          <span className="text-xs font-semibold text-slate-500 block">Высокий приоритет</span>
          <div className="mt-2 text-2xl sm:text-3xl font-bold text-slate-900">
            {stats?.highPriority !== undefined ? stats.highPriority : p3Count}
          </div>
          <span className="text-[11px] font-medium text-slate-400 mt-2 block">Приоритет P3</span>
        </div>

        <div className="bg-white border border-slate-200/90 rounded-2xl p-4 sm:p-5 shadow-xs">
          <span className="text-xs font-semibold text-slate-500 block">У координаторов</span>
          <div className="mt-2 text-2xl sm:text-3xl font-bold text-slate-900">
            {stats?.assignedCoordinators !== undefined ? stats.assignedCoordinators : '-'}
          </div>
          <span className="text-[11px] font-medium text-slate-400 mt-2 block">
            Назначенные инциденты
          </span>
        </div>

        <div className="bg-white border border-slate-200/90 rounded-2xl p-4 sm:p-5 shadow-xs">
          <span className="text-xs font-semibold text-slate-500 block">На исследовании</span>
          <div className="mt-2 text-2xl sm:text-3xl font-bold text-slate-900">
            {stats?.underInvestigation !== undefined ? stats.underInvestigation : investCount}
          </div>
          <span className="text-[11px] font-medium text-slate-400 mt-2 block">Текущий статус</span>
        </div>
      </div>

      {/* 2 Side-by-Side Analytics Cards */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
        <div className="bg-white border border-slate-200/90 rounded-2xl p-5 sm:p-6 shadow-xs flex flex-col justify-between">
          <div className="flex items-center justify-between mb-4">
            <h2 className="font-bold text-slate-900 text-sm sm:text-base">
              Распределение приоритетов
            </h2>
            <span className="text-[11px] text-slate-400 font-medium">
              Активные: {activeTotal}
            </span>
          </div>
          <div className="space-y-4">
            {priorityBars.map((item) => (
              <div key={item.label}>
                <div className="flex items-center justify-between text-xs mb-1.5">
                  <span className="font-medium text-slate-600">{item.label}</span>
                  <span className="font-bold text-slate-800 font-mono">
                    {item.count} ({item.pct}%)
                  </span>
                </div>
                <div className="h-2 w-full bg-slate-100 rounded-full overflow-hidden">
                  <div
                    className="h-full rounded-full transition-all duration-500"
                    style={{
                      width: `${item.pct}%`,
                      backgroundColor: item.color,
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="bg-white border border-slate-200/90 rounded-2xl p-5 sm:p-6 shadow-xs flex flex-col justify-between">
          <div className="flex items-center justify-between mb-4">
            <h2 className="font-bold text-slate-900 text-sm sm:text-base">Статусы инцидентов</h2>
            <span className="text-[11px] text-slate-400 font-medium">Всего: {total}</span>
          </div>
          <div className="space-y-4">
            {statusBars.map((item) => (
              <div key={item.label}>
                <div className="flex items-center justify-between text-xs mb-1.5">
                  <span className="font-medium text-slate-600">{item.label}</span>
                  <span className="font-bold text-slate-800 font-mono">
                    {item.count} ({item.pct}%)
                  </span>
                </div>
                <div className="h-2 w-full bg-slate-100 rounded-full overflow-hidden">
                  <div
                    className="h-full rounded-full transition-all duration-500"
                    style={{
                      width: `${item.pct}%`,
                      backgroundColor: item.color,
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Team Statistics Section */}
      {teamLeaderboard.length > 0 && (
        <div className="bg-white border border-slate-200/90 rounded-2xl p-5 sm:p-6 shadow-xs">
          <div className="flex items-center justify-between gap-2 mb-4">
            <h2 className="font-bold text-slate-900 text-sm sm:text-base">
              Статистика команд решения
            </h2>
            <span className="text-[11px] text-slate-400 font-medium">
              Топ-8 команд по активной нагрузке (нажмите для перехода)
            </span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            {teamLeaderboard.map((item) => {
              const avgH = item.avgEstMin > 0 ? (item.avgEstMin / 60).toFixed(1) : '—'
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => onSelectTeam && onSelectTeam(item.team)}
                  className="text-left p-3 rounded-xl border border-slate-200/80 bg-slate-50/70 hover:bg-white hover:border-[#E30611] transition-all cursor-pointer"
                >
                  <div className="flex items-center justify-between gap-2 mb-1.5">
                    <span className="text-xs font-bold text-slate-900 truncate" title={item.team}>
                      {item.team}
                    </span>
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-full bg-rose-50 text-[#E30611] flex-shrink-0">
                      {item.active}
                    </span>
                  </div>
                  <div className="flex items-center justify-between text-[11px] text-slate-500">
                    <span>
                      Очередь: <strong className="text-slate-800 font-mono">{item.queue}</strong>
                    </span>
                    <span>
                      Ср.: <strong className="text-indigo-700 font-mono">~{avgH}ч</strong>
                    </span>
                    <span
                      className={
                        item.avgSla >= 70
                          ? 'text-[#E30611] font-bold'
                          : item.avgSla >= 40
                            ? 'text-amber-700 font-semibold'
                            : 'text-emerald-700 font-semibold'
                      }
                    >
                      SLA {item.avgSla}%
                    </span>
                  </div>
                </button>
              )
            })}
          </div>
        </div>
      )}

      {/* Bottom Section: Активные инциденты */}
      <div className="bg-white border border-slate-200/90 rounded-2xl p-5 sm:p-6 shadow-xs">
        <div className="flex items-center gap-2.5 mb-4">
          <h2 className="font-bold text-slate-900 text-sm sm:text-base">Активные инциденты</h2>
          <span className="bg-rose-50 text-[#E30611] font-bold px-2 py-0.5 rounded-full text-xs">
            {stats?.active !== undefined ? stats.active : activeTotal}
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-slate-100 text-slate-400 font-semibold">
                <th className="py-2.5 px-3">ID</th>
                <th className="py-2.5 px-3">Продукт</th>
                <th className="py-2.5 px-3">Время возникновения</th>
                <th className="py-2.5 px-3">Команда решения</th>
                <th className="py-2.5 px-3">Статус</th>
                <th className="py-2.5 px-3">Прогноз ML</th>
                <th className="py-2.5 px-3">Срок решения</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-50 font-medium text-slate-700">
              {activeIncidents.length === 0 ? (
                <tr>
                  <td colSpan={7} className="py-6 text-center text-slate-400">
                    Нет активных инцидентов
                  </td>
                </tr>
              ) : (
                activeIncidents.map((inc) => (
                  <tr
                    key={inc.id}
                    onClick={() => onSelectIncident && onSelectIncident(inc)}
                    className="hover:bg-slate-50/80 transition-colors cursor-pointer group"
                  >
                    <td className="py-3 px-3 flex items-center gap-2">
                      <span className={`w-2 h-2 rounded-full ${getPriorityDot(inc)} flex-shrink-0`} />
                      <span className="font-semibold text-slate-900 group-hover:text-[#E30611] transition-colors">
                        #{inc.ticketNumber || inc.id}
                      </span>
                    </td>

                    <td className="py-3 px-3 text-slate-800">
                      {inc.productName || inc.service || 'ИТ-сервис'}
                    </td>

                    <td className="py-3 px-3 text-slate-500 whitespace-nowrap">
                      {formatSubmitTime(inc.submitDate)}
                    </td>

                    <td className="py-3 px-3 text-slate-800 max-w-[200px] truncate">
                      {inc.assignedGroupName || 'Профильная команда'}
                    </td>

                    <td className="py-3 px-3">
                      <span
                        className={`inline-block px-2.5 py-0.5 rounded-full text-[11px] font-semibold ${
                          inc.state === 'Выполняется' || inc.state === 'В работе'
                            ? 'bg-sky-50 text-sky-700'
                            : 'bg-slate-100 text-slate-600'
                        }`}
                      >
                        {inc.state || 'В работе'}
                      </span>
                    </td>

                    <td className="py-3 px-3 font-mono font-bold text-indigo-700 whitespace-nowrap">
                      {inc.aiEstimatedMinutes ? `~${(inc.aiEstimatedMinutes / 60).toFixed(1)} ч` : '—'}
                    </td>

                    <td className="py-3 px-3 text-[#E30611] font-semibold flex items-center gap-1.5 whitespace-nowrap">
                      <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                      <span>{formatSlaRemaining(inc.deadline)}</span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
