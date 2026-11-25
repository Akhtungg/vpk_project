import React, { useEffect, useState } from 'react'
import { Activity, BarChart3, Brain, Play, Sparkles, X } from 'lucide-react'
import { getMLMetrics, predictTicket } from '../services/api'
import type { MLMetricsResponse, PredictResponse } from '../types/incident'

interface MLMetricsModalProps {
  open: boolean
  onClose: () => void
}

const FEATURE_LABELS: Record<string, string> = {
  history_median_hours: 'Историческая медиана (30/90 дн.)',
  team_workload: 'Загруженность команды (очередь)',
  assignee_workload: 'Загруженность исполнителя',
  assignee_speed_factor: 'Скорость работы исполнителя',
  priority_id: 'Приоритет заявки',
  urgency_id: 'Срочность (Urgency)',
  impact_id: 'Влияние (Impact)',
  sla_target_hours: 'Норматив SLA (ч)',
  is_client_service: 'Клиентский сервис',
  created_hour: 'Час регистрации',
  is_business_hours: 'Рабочее время',
  title_length: 'Длина описания',
  has_critical_keywords: 'Критические маркеры текста',
  semantic_complexity: 'Семантическая сложность (ruBERT)',
}

export const MLMetricsModal: React.FC<MLMetricsModalProps> = ({ open, onClose }) => {
  const [metrics, setMetrics] = useState<MLMetricsResponse | null>(null)
  const [loading, setLoading] = useState(false)

  // Interactive POST /api/v1/predict tester
  const [category, setCategory] = useState('Системы хранения данных InfraCloud')
  const [groupName, setGroupName] = useState('КЦ ДОИ Администрирование SAN')
  const [priority, setPriority] = useState('High')
  const [assigneeId, setAssigneeId] = useState('ikuznetsov')
  const [slaHours, setSlaHours] = useState(4)
  const [titleText, setTitleText] = useState('Сбой репликации SAN, высокая загрузка узла')
  const [predicting, setPredicting] = useState(false)
  const [predResult, setPredResult] = useState<PredictResponse | null>(null)

  useEffect(() => {
    if (!open) return
    setLoading(true)
    getMLMetrics()
      .then(setMetrics)
      .catch((e) => console.error('Failed to load ML metrics:', e))
      .finally(() => setLoading(false))
  }, [open])

  if (!open) return null

  const handlePredict = async (e: React.FormEvent) => {
    e.preventDefault()
    setPredicting(true)
    try {
      const res = await predictTicket({
        ticket_id: `SIM-${Math.floor(1000 + Math.random() * 9000)}`,
        category,
        assigned_group_name: groupName,
        priority,
        assignee_id: assigneeId,
        sla_target_hours: Number(slaHours) || 8,
        title_text: titleText,
      })
      setPredResult(res)
    } catch (err) {
      alert('Ошибка инференса: ' + err)
    } finally {
      setPredicting(false)
    }
  }

  const sortedImportances = metrics?.featureImportances
    ? Object.entries(metrics.featureImportances).sort((a, b) => b[1] - a[1]).slice(0, 7)
    : []

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-4xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="px-6 py-4 border-b border-slate-800 flex items-center justify-between bg-slate-950/70">
          <div className="flex items-center gap-2.5">
            <Brain className="h-5 w-5 text-red-400" />
            <div>
              <h2 className="text-sm font-bold text-slate-100">
                Единая регрессионная модель ML и тест API прогноза
              </h2>
              <p className="text-[11px] text-slate-400">
                GradientBoostingRegressor (прогноз времени + 80% ДИ + риск SLA + рекомендация команды)
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-lg text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6 text-xs text-slate-300">
          {loading && !metrics ? (
            <div className="text-center py-8 text-slate-400">Загрузка метрик модели...</div>
          ) : metrics && (
            <>
              {/* Offline & Live Metrics */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="p-4 rounded-xl bg-slate-950/70 border border-slate-800 space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-slate-200 flex items-center gap-1.5">
                      <BarChart3 className="h-4 w-4 text-emerald-400" />
                      Валидация модели ({metrics.modelVersion})
                    </span>
                    <span className="text-[10px] font-mono text-slate-400">
                      train: {metrics.offlineMetrics.trainSamples} / test: {metrics.offlineMetrics.testSamples}
                    </span>
                  </div>
                  <div className="grid grid-cols-3 gap-2">
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">MAE</span>
                      <span className="text-sm font-bold font-mono text-emerald-300">{metrics.offlineMetrics.maeHours} ч</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">RMSE</span>
                      <span className="text-sm font-bold font-mono text-emerald-300">{metrics.offlineMetrics.rmseHours} ч</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">R²</span>
                      <span className="text-sm font-bold font-mono text-sky-300">{metrics.offlineMetrics.r2}</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">SLA Precision</span>
                      <span className="text-sm font-bold font-mono text-amber-300">{(metrics.offlineMetrics.slaPrecision * 100).toFixed(0)}%</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">MAPE</span>
                      <span className="text-sm font-bold font-mono text-slate-200">{metrics.offlineMetrics.mapePercent}%</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">σ (разброс)</span>
                      <span className="text-sm font-bold font-mono text-slate-200">{metrics.offlineMetrics.residualStdHours} ч</span>
                    </div>
                  </div>
                </div>

                <div className="p-4 rounded-xl bg-slate-950/70 border border-slate-800 space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-slate-200 flex items-center gap-1.5">
                      <Activity className="h-4 w-4 text-sky-400" />
                      Живая телеметрия БД (PostgreSQL)
                    </span>
                    <span className="text-[10px] font-mono text-slate-400">
                      заявок: {metrics.liveMetrics.totalIncidents}
                    </span>
                  </div>
                  <div className="grid grid-cols-2 gap-2">
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Закрытых с T_fact</span>
                      <span className="text-sm font-bold font-mono text-slate-100">{metrics.liveMetrics.closedWithActualTime}</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Live MAE (T_fact - T_pred)</span>
                      <span className="text-sm font-bold font-mono text-emerald-300">{metrics.liveMetrics.liveMaeHours} ч</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Логов в ai_predictions</span>
                      <span className="text-sm font-bold font-mono text-slate-100">{metrics.liveMetrics.totalPredictions}</span>
                    </div>
                    <div className="p-2 rounded-lg bg-slate-900 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Принятие маршрутизации</span>
                      <span className="text-sm font-bold font-mono text-amber-300">{metrics.liveMetrics.routingAcceptancePct}%</span>
                    </div>
                  </div>
                </div>
              </div>

              {/* Feature Importances */}
              {sortedImportances.length > 0 && (
                <div className="p-4 rounded-xl bg-slate-950/70 border border-slate-800 space-y-2.5">
                  <span className="font-semibold text-slate-200 block">
                    Значимость признаков (Feature Importance)
                  </span>
                  <div className="space-y-1.5">
                    {sortedImportances.map(([feat, imp]) => (
                      <div key={feat} className="flex items-center gap-3 text-[11px]">
                        <span className="w-60 truncate text-slate-300">{FEATURE_LABELS[feat] || feat}</span>
                        <div className="flex-1 bg-slate-900 rounded-full h-2 overflow-hidden border border-slate-800">
                          <div
                            className="h-full bg-gradient-to-r from-red-500 to-amber-400 rounded-full"
                            style={{ width: `${Math.min(100, imp * 100)}%` }}
                          />
                        </div>
                        <span className="w-12 text-right font-mono text-slate-400">{(imp * 100).toFixed(1)}%</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}

          {/* Interactive POST /api/v1/predict Sandbox */}
          <div className="p-4 rounded-xl bg-slate-950/80 border border-slate-800 space-y-3">
            <div className="flex items-center justify-between">
              <span className="font-semibold text-slate-200 flex items-center gap-1.5">
                <Sparkles className="h-4 w-4 text-amber-400" />
                Интерактивный тест API прогноза (<code className="font-mono text-amber-300">POST /api/v1/predict</code>)
              </span>
            </div>
            <form onSubmit={handlePredict} className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="block text-[10px] text-slate-400 mb-1">Команда / группа</label>
                <select
                  value={groupName}
                  onChange={(e) => setGroupName(e.target.value)}
                  className="w-full bg-slate-900 border border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-200"
                >
                  <option value="КЦ ДОИ Администрирование SAN">КЦ ДОИ Администрирование SAN</option>
                  <option value="КЦ ДОИ VMWare">КЦ ДОИ VMWare</option>
                  <option value="КЦ ИТ Media L2 Streaming">КЦ ИТ Media L2 Streaming</option>
                  <option value="КЦ ИТ Active Directory">КЦ ИТ Active Directory</option>
                  <option value="КЦ ИТ ЦП Billing Platform">КЦ ИТ ЦП Billing Platform</option>
                  <option value="КЦ ИТ Платформа SimpleOne">КЦ ИТ Платформа SimpleOne</option>
                  <option value="AutoClosedMi">AutoClosedMi</option>
                </select>
              </div>
              <div>
                <label className="block text-[10px] text-slate-400 mb-1">Приоритет</label>
                <select
                  value={priority}
                  onChange={(e) => setPriority(e.target.value)}
                  className="w-full bg-slate-900 border border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-200"
                >
                  <option value="Critical">Наивысший (Critical)</option>
                  <option value="High">Высокий (High)</option>
                  <option value="Medium">Средний (Medium)</option>
                  <option value="Low">Низкий (Low)</option>
                </select>
              </div>
              <div>
                <label className="block text-[10px] text-slate-400 mb-1">Норматив SLA (часов)</label>
                <input
                  type="number"
                  min={1}
                  max={72}
                  step={0.5}
                  value={slaHours}
                  onChange={(e) => setSlaHours(Number(e.target.value))}
                  className="w-full bg-slate-900 border border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-200"
                />
              </div>
              <div>
                <label className="block text-[10px] text-slate-400 mb-1">Логин исполнителя</label>
                <input
                  type="text"
                  value={assigneeId}
                  onChange={(e) => setAssigneeId(e.target.value)}
                  className="w-full bg-slate-900 border border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-200"
                />
              </div>
              <div>
                <label className="block text-[10px] text-slate-400 mb-1">Продукт / категория</label>
                <input
                  type="text"
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                  className="w-full bg-slate-900 border border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-200"
                />
              </div>
              <div>
                <label className="block text-[10px] text-slate-400 mb-1">Тема заявки</label>
                <input
                  type="text"
                  value={titleText}
                  onChange={(e) => setTitleText(e.target.value)}
                  className="w-full bg-slate-900 border border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-200"
                />
              </div>
              <div className="sm:col-span-3 flex justify-end">
                <button
                  type="submit"
                  disabled={predicting}
                  className="flex items-center gap-1.5 bg-red-600 hover:bg-red-500 text-white font-bold px-4 py-1.5 rounded-lg text-xs transition-colors"
                >
                  <Play className="h-3.5 w-3.5" />
                  {predicting ? 'Расчёт...' : 'Рассчитать прогноз'}
                </button>
              </div>
            </form>

            {predResult && (
              <div className="mt-3 p-3 rounded-xl bg-slate-900 border border-slate-800 space-y-2 text-[11px]">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-3">
                    <span>
                      Прогноз: <strong className="text-slate-100 font-mono">{predResult.predicted_time_hours} ч</strong>{' '}
                      <span className="text-slate-400 font-mono">
                        (80% ДИ: [{predResult.confidence_interval[0]}–{predResult.confidence_interval[1]} ч])
                      </span>
                    </span>
                    <span>
                      С очередью: <strong className="text-amber-300 font-mono">{predResult.total_wait_hours} ч</strong>
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`px-2 py-0.5 rounded font-bold text-[10px] ${
                      predResult.sla_status === 'CRITICAL_SLA_BREACH'
                        ? 'bg-red-900/60 text-red-200 border border-red-700'
                        : predResult.sla_status === 'WARNING'
                        ? 'bg-amber-900/60 text-amber-200 border border-amber-700'
                        : 'bg-emerald-900/60 text-emerald-200 border border-emerald-700'
                    }`}>
                      {predResult.sla_status} ({(predResult.sla_risk_score * 100).toFixed(0)}%)
                    </span>
                    <span className="text-slate-500 font-mono">{predResult.latency_ms} мс</span>
                  </div>
                </div>
                <div className="flex flex-wrap gap-3 text-slate-400">
                  <span>Очередь команды: <strong className="text-slate-200">{predResult.team_workload}</strong></span>
                  <span>Очередь инженера: <strong className="text-slate-200">{predResult.current_assignee_load}</strong></span>
                  <span>Медиана истории: <strong className="text-slate-200">{predResult.history_category_median} ч</strong></span>
                  {predResult.recommended_assignee && (
                    <span className="text-emerald-300">
                      Альтернатива: <strong>{predResult.recommended_assignee}</strong> (~{predResult.recommended_total_hours} ч)
                    </span>
                  )}
                </div>
                {predResult.top_factors && predResult.top_factors.length > 0 && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    {predResult.top_factors.map((f, i) => (
                      <span key={i} className="px-2 py-0.5 rounded bg-slate-950 border border-slate-800 text-[10px]">
                        {f.factor}:{' '}
                        <strong className={f.impactHours >= 0 ? 'text-rose-300' : 'text-emerald-300'}>
                          {f.impactHours >= 0 ? `+${f.impactHours}` : f.impactHours} ч
                        </strong>
                      </span>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
