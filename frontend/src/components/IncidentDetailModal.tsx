import React, { useEffect, useMemo, useState } from 'react'
import {
  Check,
  CheckCircle2,
  Clock,
  CornerUpLeft,
  FileText,
  History,
  Layers,
  Link as LinkIcon,
  MessageSquare,
  Save,
  Sliders,
  Sparkles,
  X,
} from 'lucide-react'
import type { Incident, IncidentEvent } from '../types/incident'
import { getIncidentEvents } from '../services/api'

interface IncidentDetailModalProps {
  incident: Incident | null
  allIncidents?: Incident[]
  onClose: () => void
  onUpdate: (
    id: string,
    updates: Partial<Incident> & {
      manualOverrideMinutes?: number
      manualOverrideReason?: string
    }
  ) => Promise<void>
  teams?: string[]
  onSelectIncident?: (inc: Incident) => void
}

function formatHours(minutes?: number | null): string {
  if (minutes == null || minutes <= 0) return '—'
  return `${(minutes / 60).toFixed(1)} ч`
}

const IncidentDetailContent: React.FC<{
  incident: Incident
  allIncidents: Incident[]
  onClose: () => void
  onUpdate: IncidentDetailModalProps['onUpdate']
  teams: string[]
  onSelectIncident?: (inc: Incident) => void
}> = ({ incident, allIncidents, onClose, onUpdate, teams, onSelectIncident }) => {
  const [activeTab, setActiveTab] = useState<'overview' | 'history' | 'related'>('overview')
  const [state, setState] = useState(incident.state || 'Зарегистрирован')
  const [assignedGroup, setAssignedGroup] = useState(incident.assignedGroupName || '')
  const solution = incident.solution || ''
  const [message, setMessage] = useState(incident.message || '')
  const [overrideHours, setOverrideHours] = useState(
    incident.manualOverrideMinutes ? (incident.manualOverrideMinutes / 60).toFixed(1) : ''
  )
  const [overrideReason, setOverrideReason] = useState(incident.manualOverrideReason || '')
  const [isSaving, setIsSaving] = useState(false)
  const [saveSuccess, setSaveSuccess] = useState(false)

  const [events, setEvents] = useState<IncidentEvent[]>([])
  const [loadingEvents, setLoadingEvents] = useState(false)

  const dedupedEvents = useMemo(() => {
    const list: IncidentEvent[] = []
    const seen = new Set<string>()
    for (const evt of events) {
      const timeSec = evt.createdAt ? new Date(evt.createdAt).toISOString().substring(0, 19) : ''
      const key = `${evt.eventType}_${evt.message}_${timeSec}`
      if (!seen.has(key)) {
        seen.add(key)
        list.push(evt)
      }
    }
    return list
  }, [events])

  useEffect(() => {
    let isMounted = true
    setLoadingEvents(true)
    getIncidentEvents(incident.id)
      .then((evts) => {
        if (isMounted) setEvents(evts)
      })
      .catch((e) => console.error('Failed to load incident history', e))
      .finally(() => {
        if (isMounted) setLoadingEvents(false)
      })
    return () => {
      isMounted = false
    }
  }, [incident.id])

  const handleSave = async () => {
    setIsSaving(true)
    try {
      const overrideMin =
        overrideHours.trim() !== '' && !Number.isNaN(Number(overrideHours))
          ? Math.round(Number(overrideHours) * 60)
          : undefined

      await onUpdate(incident.id, {
        state,
        assignedGroupName: assignedGroup,
        solution,
        message,
        manualOverrideMinutes: overrideMin,
        manualOverrideReason: overrideReason.trim() || undefined,
      })
      setSaveSuccess(true)
      setTimeout(() => setSaveSuccess(false), 3000)
      const freshEvents = await getIncidentEvents(incident.id)
      setEvents(freshEvents)
    } catch (e) {
      console.error('Failed to update incident:', e)
    } finally {
      setIsSaving(false)
    }
  }

  const handleReturnToCoordinator = async () => {
    setIsSaving(true)
    const coordGroup = 'Координаторы массовых инцидентов'
    setAssignedGroup(coordGroup)
    setState('Направлен в команду')
    try {
      await onUpdate(incident.id, {
        state: 'Направлен в команду',
        assignedGroupName: coordGroup,
        message: message || 'Требуется уточнение/переназначение от координатора',
        solution,
      })
      setSaveSuccess(true)
      setTimeout(() => setSaveSuccess(false), 3000)
    } catch (e) {
      console.error('Failed to return incident to coordinator:', e)
    } finally {
      setIsSaving(false)
    }
  }

  const applyRecommendedGroup = async (groupName: string) => {
    setAssignedGroup(groupName)
    setIsSaving(true)
    try {
      await onUpdate(incident.id, {
        assignedGroupName: groupName,
        isAccepted: true,
      })
      setSaveSuccess(true)
      setTimeout(() => setSaveSuccess(false), 3000)
    } catch (e) {
      console.error('Failed to assign group', e)
    } finally {
      setIsSaving(false)
    }
  }

  const formatSlaRemaining = (deadlineStr?: string) => {
    if (!deadlineStr) return 'Не задан'
    const deadline = new Date(deadlineStr)
    const now = new Date()
    const diffMs = deadline.getTime() - now.getTime()
    if (diffMs <= 0) return 'Срок SLA истёк'
    const hours = Math.floor(diffMs / (1000 * 60 * 60))
    const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
    if (hours > 0) return `${hours} ч ${minutes} мин`
    return `${minutes} мин`
  }

  const getPriorityBadge = () => {
    if (incident.priorityId === 4 || incident.priority === 'Наивысший' || incident.priority === 'Критический') {
      return <span className="bg-[#E30611] text-white text-xs font-bold px-3 py-1 rounded-full">Критический</span>
    }
    if (incident.priorityId === 3 || incident.priority === 'Высокий') {
      return <span className="bg-[#F97316] text-white text-xs font-bold px-3 py-1 rounded-full">Высокий</span>
    }
    if (incident.priorityId === 2 || incident.priority === 'Средний') {
      return <span className="bg-[#EAB308] text-white text-xs font-bold px-3 py-1 rounded-full">Средний</span>
    }
    return <span className="bg-[#3B82F6] text-white text-xs font-bold px-3 py-1 rounded-full">Низкий</span>
  }

  const availableTeams = useMemo(() => {
    const set = new Set<string>()
    if (incident.assignedGroupName) set.add(incident.assignedGroupName)
    if (incident.aiRoutingHint) set.add(incident.aiRoutingHint)
    incident.aiRoutingTop?.forEach((r) => r.group && set.add(r.group))
    teams.forEach((t) => t && set.add(t))
    return Array.from(set)
  }, [teams, incident])

  const relatedIncidents = useMemo(() => {
    return allIncidents
      .filter(
        (i) =>
          i.id !== incident.id &&
          ((incident.productName && i.productName === incident.productName) ||
            (incident.assignedGroupName && i.assignedGroupName === incident.assignedGroupName))
      )
      .slice(0, 6)
  }, [allIncidents, incident])

  const slaRisk = Math.round(incident.aiSlaRisk ?? 0)
  const estMin = incident.aiEstimatedMinutes || 0
  const minCI = incident.aiEstimatedMinMinutes || Math.round(estMin * 0.72)
  const maxCI = incident.aiEstimatedMaxMinutes || Math.round(estMin * 1.32)
  const totalWithQueueMin = estMin + (incident.aiQueueRemainingMinutes ?? 0)
  const routingTop =
    incident.aiRoutingTop && incident.aiRoutingTop.length > 0
      ? incident.aiRoutingTop.slice(0, 3)
      : incident.aiRoutingHint
        ? [
            {
              group: incident.aiRoutingHint,
              confidence: incident.aiRoutingConfidence || 82,
              estimatedMinutes: estMin,
              slaRisk,
              workload: incident.aiTeamWorkload ?? 0,
            },
          ]
        : []

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 bg-black/45 backdrop-blur-xs select-none"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="bg-white border border-slate-200 rounded-3xl w-full max-w-6xl max-h-[94vh] flex flex-col shadow-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-150"
      >
        {/* Header */}
        <div className="px-6 py-4 border-b border-slate-100 flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-3 flex-wrap">
              <h2 className="text-lg sm:text-xl font-bold text-slate-900 tracking-tight leading-snug">
                {incident.header || incident.shortDescription || incident.id}
              </h2>
              {getPriorityBadge()}
              {incident.actualDurationMinutes != null && incident.actualDurationMinutes > 0 && (
                <span className="text-emerald-700 bg-emerald-50 border border-emerald-200 text-xs font-semibold px-2.5 py-0.5 rounded-full">
                  T_факт: {formatHours(incident.actualDurationMinutes)} (Δ:{' '}
                  {incident.predictionErrorMinutes != null
                    ? `${incident.predictionErrorMinutes > 0 ? '+' : ''}${(incident.predictionErrorMinutes / 60).toFixed(1)} ч`
                    : '—'}
                  )
                </span>
              )}
              {saveSuccess && (
                <span className="text-emerald-700 bg-emerald-50 border border-emerald-200 text-xs font-semibold px-2.5 py-0.5 rounded-full flex items-center gap-1 animate-in fade-in">
                  <CheckCircle2 className="h-3.5 w-3.5" /> Сохранено
                </span>
              )}
            </div>
            <div className="mt-1 flex items-center gap-3 text-xs font-mono font-medium text-slate-400">
              <span>{incident.ticketNumber || incident.id}</span>
              <span className="text-[10px] text-slate-300" title="UUID в базе данных">
                UUID: {incident.id}
              </span>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-xl text-slate-400 hover:text-slate-700 hover:bg-slate-100 transition-colors cursor-pointer flex-shrink-0 mt-0.5"
            title="Закрыть"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Navigation Tabs */}
        <div className="flex items-center gap-6 px-6 border-b border-slate-100 text-xs font-semibold">
          <button
            onClick={() => setActiveTab('overview')}
            className={`py-3 relative transition-colors cursor-pointer ${
              activeTab === 'overview' ? 'text-slate-900' : 'text-slate-400 hover:text-slate-600'
            }`}
          >
            <span>Обзор</span>
            {activeTab === 'overview' && (
              <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-[#E30611] rounded-full" />
            )}
          </button>
          <button
            onClick={() => setActiveTab('history')}
            className={`py-3 relative transition-colors flex items-center gap-1.5 cursor-pointer ${
              activeTab === 'history' ? 'text-slate-900' : 'text-slate-400 hover:text-slate-600'
            }`}
          >
            <History className="h-3.5 w-3.5" />
            <span>История ({dedupedEvents.length})</span>
            {activeTab === 'history' && (
              <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-[#E30611] rounded-full" />
            )}
          </button>
          <button
            onClick={() => setActiveTab('related')}
            className={`py-3 relative transition-colors flex items-center gap-1.5 cursor-pointer ${
              activeTab === 'related' ? 'text-slate-900' : 'text-slate-400 hover:text-slate-600'
            }`}
          >
            <LinkIcon className="h-3.5 w-3.5" />
            <span>Связанные инциденты ({relatedIncidents.length})</span>
            {activeTab === 'related' && (
              <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-[#E30611] rounded-full" />
            )}
          </button>
        </div>

        {/* Modal Body */}
        <div className="flex-1 overflow-y-auto p-6 text-xs text-slate-700">
          {activeTab === 'overview' && (
            <div className="grid grid-cols-1 md:grid-cols-12 gap-6 items-start">
              {/* Left Column: Описание и Параметры */}
              <div className="md:col-span-7 space-y-5">
                <div>
                  <div className="flex items-center gap-2 mb-2 font-bold text-slate-900 text-sm">
                    <FileText className="h-4 w-4 text-slate-600" />
                    <span>Описание</span>
                  </div>
                  <p className="text-slate-600 leading-relaxed text-xs whitespace-pre-wrap">
                    {incident.description ||
                      incident.shortDescription ||
                      incident.header ||
                      'Подробное описание инцидента отсутствует.'}
                  </p>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center gap-2 font-bold text-slate-900 text-sm">
                    <Layers className="h-4 w-4 text-slate-600" />
                    <span>Параметры инцидента</span>
                  </div>

                  <div className="border-t border-slate-200/80 divide-y divide-slate-100 text-xs">
                    <div className="py-2.5 flex justify-between items-center gap-4">
                      <span className="text-slate-500 font-medium">Продукт</span>
                      <span className="font-semibold text-slate-900 text-right">
                        {incident.productName || 'Инфраструктурная платформа'}
                      </span>
                    </div>

                    <div className="py-2.5 flex justify-between items-center gap-4">
                      <span className="text-slate-500 font-medium">Сервис</span>
                      <span className="font-semibold text-slate-900 text-right">
                        {incident.service || incident.productName || 'ИТ-сервис'}
                      </span>
                    </div>

                    <div className="py-2.5 flex justify-between items-center gap-4">
                      <span className="text-slate-500 font-medium">Сценарий</span>
                      <span className="font-semibold text-slate-900 text-right">
                        {incident.scenarioName || 'Восстановление сервиса'}
                      </span>
                    </div>

                    <div className="py-2.5 flex justify-between items-center gap-4">
                      <span className="text-slate-500 font-medium">Инициатор</span>
                      <span className="font-semibold text-slate-900 text-right">
                        {incident.initiatorName || incident.initiatorLogin || 'Система мониторинга'}
                      </span>
                    </div>

                    <div className="py-2.5 flex justify-between items-center gap-4">
                      <span className="text-slate-500 font-medium">Влияние / срочность</span>
                      <span className="font-semibold text-slate-900 text-right">
                        {incident.impact || 'Среднее'} / {incident.urgency || 'Средняя'}
                      </span>
                    </div>

                    <div className="py-2.5 flex justify-between items-center gap-4">
                      <span className="text-slate-500 font-medium">Тип сервиса</span>
                      <span className="font-semibold text-slate-900 text-right">
                        {incident.serviceType || 'Технологический'}
                      </span>
                    </div>

                    {incident.solution && (
                      <div className="py-2.5 flex justify-between items-center gap-4">
                        <span className="text-slate-500 font-medium">Решение</span>
                        <span className="font-semibold text-slate-900 text-right">
                          {incident.solution}
                        </span>
                      </div>
                    )}
                  </div>

                  {/* Dropdown Selects for Status & Team only */}
                  <div className="pt-2 border-t border-slate-200/80 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <label className="text-slate-600 font-medium text-xs">Статус</label>
                      <select
                        value={state}
                        onChange={(e) => setState(e.target.value)}
                        className="bg-white border border-slate-300 rounded-lg px-3 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-[#E30611] max-w-xs w-full text-right cursor-pointer"
                      >
                        <option value="Зарегистрирован">Зарегистрирован</option>
                        <option value="Направлен в команду">Направлен в команду</option>
                        <option value="В работе">В работе</option>
                        <option value="Требуется информация">Требуется информация</option>
                        <option value="Исследование ситуации">Исследование ситуации</option>
                        <option value="Решен">Решен</option>
                        <option value="Завершен">Завершен</option>
                        <option value="Закрыт">Закрыт</option>
                      </select>
                    </div>

                    <div className="flex items-center justify-between gap-4">
                      <label className="text-slate-600 font-medium text-xs">
                        Команда решения
                      </label>
                      <select
                        value={assignedGroup}
                        onChange={(e) => setAssignedGroup(e.target.value)}
                        className="bg-white border border-slate-300 rounded-lg px-3 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-[#E30611] max-w-xs w-full text-right cursor-pointer truncate"
                      >
                        {availableTeams.map((team) => (
                          <option key={team} value={team}>
                            {team}
                          </option>
                        ))}
                      </select>
                    </div>

                    {/* Manual Override (Theme 16 F-12) */}
                    <div className="pt-2 border-t border-slate-200/80 space-y-2">
                      <label className="text-slate-700 font-semibold text-xs flex items-center gap-1.5">
                        <Sliders className="h-3.5 w-3.5 text-slate-500" />
                        <span>Ручная корректировка прогноза трудозатрат (ч)</span>
                      </label>
                      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                        <input
                          type="number"
                          step="0.5"
                          min="0"
                          value={overrideHours}
                          onChange={(e) => setOverrideHours(e.target.value)}
                          placeholder="Часы (напр. 3.5)"
                          className="bg-white border border-slate-300 rounded-lg px-2.5 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-[#E30611]"
                        />
                        <input
                          type="text"
                          value={overrideReason}
                          onChange={(e) => setOverrideReason(e.target.value)}
                          placeholder="Обоснование корректировки..."
                          className="sm:col-span-2 bg-white border border-slate-300 rounded-lg px-2.5 py-1.5 text-xs text-slate-900 focus:outline-none focus:border-[#E30611]"
                        />
                      </div>
                    </div>

                    {/* Coordinator handover message */}
                    <div className="pt-2 border-t border-slate-200/80 space-y-1.5">
                      <div className="flex items-center justify-between">
                        <label className="text-slate-800 font-bold text-xs flex items-center gap-1.5">
                          <MessageSquare className="h-3.5 w-3.5 text-amber-600" />
                          <span>Сообщение для координатора (необходимые действия)</span>
                        </label>
                        {message && (
                          <button
                            type="button"
                            onClick={() => setMessage('')}
                            className="text-[10px] text-slate-400 hover:text-red-500 transition-colors cursor-pointer"
                          >
                            Очистить
                          </button>
                        )}
                      </div>
                      <textarea
                        value={message}
                        onChange={(e) => setMessage(e.target.value)}
                        placeholder="Опишите выполненную работу и требуемые действия смежной команды..."
                        rows={3}
                        className="w-full bg-white border border-slate-300 rounded-xl p-2.5 text-xs text-slate-900 placeholder:text-slate-400 focus:outline-none focus:border-[#E30611] transition-all resize-none leading-relaxed"
                      />
                    </div>

                    {/* Action Buttons */}
                    <div className="flex flex-wrap items-center justify-between gap-2 pt-2">
                      <button
                        type="button"
                        onClick={handleReturnToCoordinator}
                        disabled={isSaving}
                        className="bg-amber-500 hover:bg-amber-600 text-white font-semibold px-3.5 py-2 rounded-xl text-xs flex items-center gap-1.5 transition-all cursor-pointer shadow-xs disabled:opacity-50"
                      >
                        <CornerUpLeft className="h-3.5 w-3.5" />
                        <span>Вернуть координатору</span>
                      </button>

                      <button
                        onClick={handleSave}
                        disabled={isSaving}
                        className="bg-slate-900 hover:bg-slate-800 text-white font-semibold px-4 py-2 rounded-xl text-xs flex items-center gap-1.5 transition-all cursor-pointer shadow-xs disabled:opacity-50"
                      >
                        <Save className="h-3.5 w-3.5" />
                        <span>{isSaving ? 'Сохранение...' : 'Сохранить изменения'}</span>
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              {/* Right Column: Soft Purple AI Card (Teams & Unified Regression ML) */}
              <div className="md:col-span-5 bg-[#FAF7FD] border border-purple-100 rounded-3xl p-5 sm:p-6 space-y-4">
                <div className="flex items-center justify-between pb-3 border-b border-purple-100/80">
                  <div className="flex items-center gap-2">
                    <Sparkles className="h-4 w-4 text-purple-600" />
                    <div>
                      <span className="font-extrabold text-slate-900 text-sm block leading-tight">
                        Регрессионная модель ML
                      </span>
                      <span className="text-[10px] font-mono text-purple-600">
                        {incident.aiModelVersion || 'gbdt_rubert_v3.0'}
                      </span>
                    </div>
                  </div>
                  <span className="flex items-center gap-1 text-[11px] font-bold text-emerald-700 bg-emerald-50 border border-emerald-200 px-2.5 py-0.5 rounded-full">
                    <Check className="h-3 w-3 text-emerald-600 stroke-[3]" />
                    <span>Завершён</span>
                  </span>
                </div>

                {/* Section 1: Regression Time Forecast & SLA Risk */}
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 font-bold text-slate-900 text-xs">
                      <Clock className="h-4 w-4 text-purple-600" />
                      <span>Прогноз времени и SLA</span>
                    </div>
                    <span
                      className={`font-extrabold font-mono text-sm ${
                        slaRisk >= 70
                          ? 'text-[#E30611]'
                          : slaRisk >= 40
                            ? 'text-amber-600'
                            : 'text-emerald-700'
                      }`}
                    >
                      Риск SLA: {slaRisk}%
                    </span>
                  </div>

                  <div className="p-3 rounded-xl bg-white border border-purple-100 space-y-1.5 shadow-2xs">
                    <div className="flex items-baseline justify-between">
                      <span className="text-slate-500">Прогноз (T_pred):</span>
                      <span className="text-lg font-black text-slate-900 font-mono">
                        {estMin > 0 ? `~${formatHours(estMin)}` : '—'}
                      </span>
                    </div>
                    {estMin > 0 && (
                      <div className="flex justify-between text-[11px]">
                        <span className="text-slate-500">80% доверительный интервал:</span>
                        <span className="font-mono font-semibold text-indigo-700">
                          [{(minCI / 60).toFixed(1)} – {(maxCI / 60).toFixed(1)} ч]
                        </span>
                      </div>
                    )}
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-500">Итого с учётом очереди команды:</span>
                      <span className="font-semibold text-slate-800 font-mono">
                        {totalWithQueueMin > 0 ? formatHours(totalWithQueueMin) : '—'}
                      </span>
                    </div>
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-500">Очередь команды:</span>
                      <span className="font-semibold text-slate-800 font-mono">
                        {incident.aiTeamWorkload ?? 0} активных заявок
                      </span>
                    </div>
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-500">Медиана по истории:</span>
                      <span className="font-semibold text-slate-800 font-mono">
                        {formatHours(incident.aiHistoryMedianMinutes)}
                      </span>
                    </div>
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-500">Запас SLA:</span>
                      <span className="font-semibold text-slate-800">
                        {formatSlaRemaining(incident.deadline)}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Section 2: Top Influence Factors */}
                {incident.aiTopFactors && incident.aiTopFactors.length > 0 && (
                  <div className="space-y-1.5 pt-2 border-t border-purple-100/60">
                    <span className="font-bold text-slate-900 text-xs block">
                      Ключевые факторы прогноза
                    </span>
                    <div className="bg-white rounded-xl border border-purple-100 p-2.5 space-y-1">
                      {incident.aiTopFactors.slice(0, 3).map((f, idx) => (
                        <div key={idx} className="flex items-center justify-between text-[11px]">
                          <span className="text-slate-600 truncate pr-2">{f.factor}</span>
                          <span
                            className={`font-mono font-bold ${
                              f.impactHours >= 0 ? 'text-[#E30611]' : 'text-emerald-700'
                            }`}
                          >
                            {f.impactHours >= 0 ? `+${f.impactHours}` : f.impactHours} ч
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Section 3: Team Statistics & Selection (ML) */}
                <div className="space-y-2.5 pt-2 border-t border-purple-100/60">
                  <div className="flex items-center gap-2 font-bold text-slate-900 text-xs">
                    <Sparkles className="h-4 w-4 text-purple-600" />
                    <span>Выбор команды решения (статистика ML)</span>
                  </div>

                  {incident.message && (
                    <div className="p-3 rounded-xl bg-amber-50/90 border border-amber-200 text-amber-950 text-xs space-y-1 shadow-2xs">
                      <div className="flex items-center gap-1.5 font-bold text-amber-900 text-xs">
                        <MessageSquare className="h-3.5 w-3.5 text-amber-600 flex-shrink-0" />
                        <span>Учтено сообщение команды:</span>
                      </div>
                      <p className="bg-white/80 rounded-lg p-2 border border-amber-200/60 text-slate-800 italic leading-snug text-[11px]">
                        «{incident.message}»
                      </p>
                    </div>
                  )}

                  <div className="space-y-2">
                    {routingTop.map((item) => {
                      const isSelected = item.group === assignedGroup
                      return (
                        <div
                          key={item.group}
                          className={`flex items-center justify-between gap-2 p-2.5 rounded-xl border shadow-2xs transition-colors ${
                            isSelected
                              ? 'bg-purple-50/70 border-purple-300'
                              : 'bg-white border-purple-100'
                          }`}
                        >
                          <div className="min-w-0 flex-1">
                            <span
                              className="font-semibold text-slate-900 text-xs truncate block"
                              title={item.group}
                            >
                              {item.group}
                            </span>
                            <div className="flex flex-wrap items-center gap-2 text-[10px] text-slate-500 mt-0.5">
                              {item.estimatedMinutes ? (
                                <span className="font-mono text-indigo-700 font-semibold">
                                  ~{(item.estimatedMinutes / 60).toFixed(1)} ч
                                </span>
                              ) : null}
                              {item.slaRisk != null && (
                                <span
                                  className={
                                    item.slaRisk >= 70 ? 'text-[#E30611] font-bold' : ''
                                  }
                                >
                                  SLA: {Math.round(item.slaRisk)}%
                                </span>
                              )}
                              {item.workload != null && (
                                <span>Очередь: {item.workload}</span>
                              )}
                            </div>
                          </div>
                          <div className="flex items-center gap-2 flex-shrink-0">
                            <span className="font-bold text-xs text-slate-900 font-mono">
                              {Math.round(item.confidence)}%
                            </span>
                            <button
                              type="button"
                              disabled={isSaving || isSelected}
                              onClick={() => applyRecommendedGroup(item.group)}
                              className={`border text-xs font-semibold px-2.5 py-1 rounded-lg transition-all shadow-2xs ${
                                isSelected
                                  ? 'bg-purple-600 border-purple-600 text-white cursor-default'
                                  : 'border-slate-300 hover:border-purple-500 bg-white hover:bg-purple-50 text-slate-800 cursor-pointer'
                              }`}
                            >
                              {isSelected ? 'Выбрана' : 'Выбрать'}
                            </button>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'history' && (
            <div className="space-y-4">
              <h4 className="font-bold text-slate-900 text-sm">История изменений и аудит</h4>
              {loadingEvents ? (
                <div className="py-8 text-center text-slate-400">Загрузка истории...</div>
              ) : dedupedEvents.length === 0 ? (
                <div className="py-8 text-center text-slate-400">История пока не зафиксирована</div>
              ) : (
                <div className="relative pl-6 space-y-4 before:absolute before:left-2 before:top-2 before:bottom-2 before:w-0.5 before:bg-slate-200">
                  {dedupedEvents.map((evt, idx) => (
                    <div key={idx} className="relative">
                      <span className="absolute -left-6 top-1 w-2.5 h-2.5 rounded-full bg-[#E30611] ring-4 ring-white" />
                      <div className="bg-slate-50 p-3 rounded-xl border border-slate-100">
                        <div className="flex items-center justify-between text-[11px] text-slate-400 mb-1">
                          <span className="font-bold text-slate-700">{evt.eventType}</span>
                          <span>{new Date(evt.createdAt).toLocaleString('ru-RU')}</span>
                        </div>
                        <p className="text-slate-800 text-xs">{evt.message}</p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {activeTab === 'related' && (
            <div className="space-y-4">
              <h4 className="font-bold text-slate-900 text-sm">
                Связанные и аналогичные инциденты ({incident.productName || incident.service})
              </h4>
              {relatedIncidents.length === 0 ? (
                <div className="py-8 text-center text-slate-400 space-y-2">
                  <LinkIcon className="h-8 w-8 mx-auto text-slate-300" />
                  <p className="font-semibold text-slate-600">Связанные инциденты не найдены</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {relatedIncidents.map((rel) => (
                    <div
                      key={rel.id}
                      onClick={() => onSelectIncident && onSelectIncident(rel)}
                      className="p-3 rounded-xl border border-slate-200 hover:border-[#E30611] bg-slate-50/70 hover:bg-white cursor-pointer transition-all"
                    >
                      <div className="flex items-center justify-between text-[11px] mb-1">
                        <span className="font-mono font-bold text-slate-500">
                          {rel.ticketNumber || rel.id}
                        </span>
                        <span className="font-semibold text-slate-600">{rel.state}</span>
                      </div>
                      <p className="text-xs font-bold text-slate-900 line-clamp-1 mb-1">
                        {rel.header}
                      </p>
                      <div className="flex items-center justify-between text-[10px] text-slate-500">
                        <span className="truncate">{rel.assignedGroupName}</span>
                        {rel.actualDurationMinutes ? (
                          <span className="font-mono text-emerald-700 font-bold">
                            Факт: {(rel.actualDurationMinutes / 60).toFixed(1)} ч
                          </span>
                        ) : rel.aiEstimatedMinutes ? (
                          <span className="font-mono text-indigo-700 font-bold">
                            Прогноз: ~{(rel.aiEstimatedMinutes / 60).toFixed(1)} ч
                          </span>
                        ) : null}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export const IncidentDetailModal: React.FC<IncidentDetailModalProps> = ({
  incident,
  allIncidents = [],
  onClose,
  onUpdate,
  teams = [],
  onSelectIncident,
}) => {
  if (!incident) return null
  return (
    <IncidentDetailContent
      key={incident.id}
      incident={incident}
      allIncidents={allIncidents}
      onClose={onClose}
      onUpdate={onUpdate}
      teams={teams}
      onSelectIncident={onSelectIncident}
    />
  )
}
