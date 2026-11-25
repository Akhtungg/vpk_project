import React from 'react'
import {
  ArrowRight,
  CheckCircle,
  Clock,
  Flame,
  Sparkles,
  Users,
} from 'lucide-react'
import type { Incident } from '../types/incident'

interface IncidentCardProps {
  incident: Incident
  onClick: (inc: Incident) => void
  onQuickReassign?: (inc: Incident, targetGroup: string) => void
}

export const IncidentCard: React.FC<IncidentCardProps> = ({ incident, onClick, onQuickReassign }) => {
  const isCritical = incident.priorityId === 4 || incident.priority === 'Наивысший'
  const isResolved = incident.state === 'Завершен' || incident.state === 'Закрыт'
  const isCoordinator = incident.assignedGroupName === 'КЦ ИТ Координаторы Массовых Инцидентов'
  const isHighPriority = incident.priorityId === 3 || incident.priority === 'Высокий'

  // Deadline calculation
  const getDeadlineInfo = (deadlineStr?: string) => {
    if (!deadlineStr) return null
    const deadline = new Date(deadlineStr)
    const now = new Date()
    const diffMs = deadline.getTime() - now.getTime()
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
    const diffMins = Math.floor((Math.abs(diffMs) % (1000 * 60 * 60)) / (1000 * 60))

    if (diffMs < 0) {
      return { text: `Просрочен ${Math.abs(diffHours)}ч`, isOverdue: true }
    }
    if (diffHours < 3) {
      return { text: `SLA: ${diffHours}ч ${diffMins}м`, isUrgent: true }
    }
    return { text: `SLA: ${diffHours}ч`, isNormal: true }
  }

  const deadlineInfo = getDeadlineInfo(incident.deadline)

  return (
    <div
      onClick={() => onClick(incident)}
      className={`group relative bg-slate-900/90 rounded-xl border p-2.5 transition-all duration-150 cursor-pointer ${
        isCritical && !isResolved
          ? 'border-red-600 bg-red-950/20 shadow-md shadow-red-950/50 ring-1 ring-red-500/40 hover:border-red-500'
          : isResolved
          ? 'border-slate-800/60 opacity-75 hover:opacity-100 hover:border-slate-700'
          : 'border-slate-800 hover:border-slate-700 hover:bg-slate-850'
      }`}
    >
      {/* Line 1: ID, Product Tag, State/Critical Tag */}
      <div className="flex items-center justify-between gap-1.5 mb-1.5">
        <div className="flex items-center gap-1.5 min-w-0">
          <span className="font-mono text-xs font-bold text-slate-100 group-hover:text-red-400 transition-colors">
            {incident.id}
          </span>
          {incident.productName && (
            <span className="text-[10px] font-medium text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded truncate max-w-[140px]">
              {incident.productName}
            </span>
          )}
        </div>

        {/* Priority or Resolved badge */}
        <div className="flex items-center gap-1 flex-shrink-0">
          {isResolved ? (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-950 text-emerald-300 border border-emerald-800 flex items-center gap-1">
              <CheckCircle className="h-2.5 w-2.5" /> Решено
            </span>
          ) : isCritical ? (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-black bg-red-600 text-white uppercase tracking-wider flex items-center gap-1 shadow-sm">
              <Flame className="h-2.5 w-2.5" /> Критический
            </span>
          ) : isHighPriority ? (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-orange-950 text-orange-300 border border-orange-800">
              Высокий
            </span>
          ) : null}
        </div>
      </div>

      {/* Line 2: Header (1-2 compact lines) */}
      <h3 className="text-xs font-medium text-slate-200 leading-snug line-clamp-2 mb-2">
        {incident.header || incident.shortDescription || 'Без заголовка'}
      </h3>

      {/* Line 3: Team & SLA Timer (glanceable) */}
      <div className="flex items-center justify-between gap-2 text-[11px] text-slate-400 bg-slate-950/60 px-2 py-1 rounded-md border border-slate-800/40">
        <div className="flex items-center gap-1.5 truncate min-w-0">
          <Users className="h-3 w-3 text-slate-500 flex-shrink-0" />
          <span className={`truncate font-medium ${isCoordinator ? 'text-amber-300 font-semibold' : 'text-slate-300'}`}>
            {incident.assignedGroupName}
          </span>
          {!isCoordinator && incident.aiRoutingConfidence > 0 && (
            <span
              title={`Точность совпадения команды по оценке ИИ: ${incident.aiRoutingConfidence}%`}
              className="px-1 py-0.2 rounded text-[9px] font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700 flex-shrink-0"
            >
              {incident.aiRoutingConfidence}%
            </span>
          )}
        </div>

        {deadlineInfo && (
          <div
            className={`flex items-center gap-1 flex-shrink-0 font-mono text-[10px] font-semibold ${
              deadlineInfo.isOverdue
                ? 'text-rose-400 animate-pulse'
                : deadlineInfo.isUrgent
                ? 'text-amber-400'
                : 'text-slate-400'
            }`}
          >
            <Clock className="h-2.5 w-2.5" />
            <span>{deadlineInfo.text}</span>
          </div>
        )}
      </div>

      {/* Line 4: Unified Regression ML Insights */}
      {/* A) Regression Routing Hint for Coordinator (One-click reassign action) */}
      {incident.aiRoutingHint && isCoordinator && !isResolved && (
        <div className="flex items-center justify-between gap-1.5 mt-1.5 bg-amber-950/40 border border-amber-800/60 p-1.5 rounded-lg text-[11px]">
          <div className="flex items-center gap-1.5 min-w-0 text-amber-200">
            <Sparkles className="h-3.5 w-3.5 text-amber-400 flex-shrink-0" />
            <div className="flex items-center gap-1.5 truncate">
              <span className="text-slate-400 text-[10px]">ИИ:</span>
              <span className="truncate font-semibold text-amber-100 text-[11px]">
                {incident.aiRoutingHint}
              </span>
              <span className="px-1.5 py-0.2 rounded text-[10px] font-mono font-bold bg-amber-500/20 text-amber-300 border border-amber-600/50 flex-shrink-0">
                {incident.aiRoutingConfidence}%
              </span>
            </div>
          </div>
          {onQuickReassign && (
            <button
              onClick={(e) => {
                e.stopPropagation()
                onQuickReassign(incident, incident.aiRoutingHint)
              }}
              className="flex items-center gap-0.5 bg-amber-500 hover:bg-amber-400 text-slate-950 px-2 py-0.5 rounded text-[10px] font-bold transition-colors flex-shrink-0 shadow-sm"
              title={`Передать в ${incident.aiRoutingHint} (${incident.aiRoutingConfidence}%)`}
            >
              <span>Передать</span>
              <ArrowRight className="h-2.5 w-2.5" />
            </button>
          )}
        </div>
      )}

      {/* C) Regression Estimate & SLA Breach Risk badge */}
      {!isResolved && (incident.aiEstimatedMinutes || 0) > 0 && (
        <div className={`flex items-center justify-between mt-1.5 text-[10px] px-2 py-0.5 rounded border ${
          (incident.aiSlaRisk || 0) >= 75
            ? 'bg-rose-950/40 border-rose-800/60 text-rose-300'
            : (incident.aiSlaRisk || 0) >= 50
            ? 'bg-amber-950/30 border-amber-800/50 text-amber-300'
            : 'bg-slate-950/60 border-slate-800/80 text-slate-400'
        }`}>
          <span className="flex items-center gap-1 font-medium truncate">
            <Clock className="h-3 w-3 flex-shrink-0" />
            <span className="font-mono text-slate-200">
              ~{((incident.aiEstimatedMinutes || 0) / 60).toFixed(1)}ч
            </span>
            {(incident.aiEstimatedMinMinutes || 0) > 0 && (incident.aiEstimatedMaxMinutes || 0) > 0 && (
              <span className="text-[9px] text-slate-400 font-mono hidden sm:inline">
                [{((incident.aiEstimatedMinMinutes || 0) / 60).toFixed(1)}–{((incident.aiEstimatedMaxMinutes || 0) / 60).toFixed(1)}]
              </span>
            )}
          </span>
          <div className="flex items-center gap-1.5 font-mono flex-shrink-0">
            {incident.aiTeamWorkload !== undefined && (
              <span className="text-[9px] text-slate-400 font-sans" title="Активных инцидентов в очереди команды">
                оч: {incident.aiTeamWorkload}
              </span>
            )}
            {(incident.aiSlaRisk || 0) >= 50 && (
              <span className="font-bold">SLA {incident.aiSlaRisk}%</span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
