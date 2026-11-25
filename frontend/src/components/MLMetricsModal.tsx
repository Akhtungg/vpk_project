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
  const [groupName, setGroupName] = useState('Команда инфраструктуры SAN')
  const [priority, setPriority] = useState('High')
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
        assignee_id: '',
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
    ? Object.entries(metrics.featureImportances)
        .filter(([feat]) => feat !== 'assignee_workload' && feat !== 'assignee_speed_factor')
        .sort((a, b) => b[1] - a[1])
        .slice(0, 7)
    : []

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/45 backdrop-blur-xs"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="bg-white border border-slate-200 rounded-2xl w-full max-w-4xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-150"
      >
        {/* Header */}
        <div className="px-6 py-4 border-b border-slate-200 flex items-center justify-between bg-slate-50/80">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-indigo-100 flex items-center justify-center">
              <Brain className="h-5 w-5 text-indigo-700" />
            </div>
            <div>
              <h2 className="text-sm font-black text-slate-900">
                Единая регрессионная модель ML и тест API прогноза
              </h2>
              <p className="text-[11px] text-slate-500">
                GradientBoostingRegressor (прогноз времени + 80% ДИ + риск SLA + рекомендация команды)
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-700 hover:bg-slate-200/60 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-5 text-xs text-slate-700 bg-white">
          {loading && !metrics ? (
            <div className="text-center py-8 text-slate-400">Загрузка метрик модели...</div>
          ) : (
            metrics && (
              <>
                {/* Offline & Live Metrics */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="p-4 rounded-xl bg-slate-50 border border-slate-200 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="font-bold text-slate-900 flex items-center gap-1.5">
                        <BarChart3 className="h-4 w-4 text-emerald-600" />
                        Валидация модели ({metrics.modelVersion})
                      </span>
                      <span className="text-[10px] font-mono text-slate-500">
                        train: {metrics.offlineMetrics.trainSamples} / test:{' '}
                        {metrics.offlineMetrics.testSamples}
                      </span>
                    </div>
                    <div className="grid grid-cols-3 gap-2">
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">MAE</span>
                        <span className="text-sm font-black font-mono text-emerald-700">
                          {metrics.offlineMetrics.maeHours} ч
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">RMSE</span>
                        <span className="text-sm font-black font-mono text-emerald-700">
                          {metrics.offlineMetrics.rmseHours} ч
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">R²</span>
                        <span className="text-sm font-black font-mono text-indigo-700">
                          {metrics.offlineMetrics.r2}
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">
                          SLA Precision
                        </span>
                        <span className="text-sm font-black font-mono text-amber-700">
                          {(metrics.offlineMetrics.slaPrecision * 100).toFixed(0)}%
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">MAPE</span>
                        <span className="text-sm font-black font-mono text-slate-800">
                          {metrics.offlineMetrics.mapePercent}%
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">
                          σ (разброс)
                        </span>
                        <span className="text-sm font-black font-mono text-slate-800">
                          {metrics.offlineMetrics.residualStdHours} ч
                        </span>
                      </div>
                    </div>
                  </div>

                  <div className="p-4 rounded-xl bg-slate-50 border border-slate-200 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="font-bold text-slate-900 flex items-center gap-1.5">
                        <Activity className="h-4 w-4 text-indigo-600" />
                        Живая телеметрия БД (PostgreSQL)
                      </span>
                      <span className="text-[10px] font-mono text-slate-500">
                        заявок: {metrics.liveMetrics.totalIncidents}
                      </span>
                    </div>
                    <div className="grid grid-cols-2 gap-2">
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">
                          Закрытых с T_fact
                        </span>
                        <span className="text-sm font-black font-mono text-slate-900">
                          {metrics.liveMetrics.closedWithActualTime}
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">
                          Live MAE (T_fact - T_pred)
                        </span>
                        <span className="text-sm font-black font-mono text-emerald-700">
                          {metrics.liveMetrics.liveMaeHours} ч
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">
                          Логов в ai_predictions
                        </span>
                        <span className="text-sm font-black font-mono text-slate-900">
                          {metrics.liveMetrics.totalPredictions}
                        </span>
                      </div>
                      <div className="p-2.5 rounded-lg bg-white border border-slate-200">
                        <span className="text-[10px] text-slate-400 block font-semibold">
                          Принятие маршрутизации
                        </span>
                        <span className="text-sm font-black font-mono text-amber-700">
                          {metrics.liveMetrics.routingAcceptancePct}%
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                {/* Feature Importances */}
                {sortedImportances.length > 0 && (
                  <div className="p-4 rounded-xl bg-slate-50 border border-slate-200 space-y-2.5">
                    <span className="font-bold text-slate-900 block">
                      Значимость признаков (Feature Importance)
                    </span>
                    <div className="space-y-2">
                      {sortedImportances.map(([feat, imp]) => (
                        <div key={feat} className="flex items-center gap-3 text-[11px]">
                          <span className="w-60 truncate text-slate-700 font-medium">
                            {FEATURE_LABELS[feat] || feat}
                          </span>
                          <div className="flex-1 bg-slate-200 rounded-full h-2 overflow-hidden">
                            <div
                              className="h-full bg-gradient-to-r from-[#E30611] to-indigo-600 rounded-full"
                              style={{ width: `${Math.min(100, imp * 100)}%` }}
                            />
                          </div>
                          <span className="w-12 text-right font-mono font-bold text-slate-600">
                            {(imp * 100).toFixed(1)}%
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )
          )}

          {/* Interactive POST /api/v1/predict Sandbox */}
          <div className="p-4 rounded-xl bg-[#FAF7FD] border border-purple-200 space-y-3">
            <div className="flex items-center justify-between">
              <span className="font-bold text-purple-950 flex items-center gap-1.5">
                <Sparkles className="h-4 w-4 text-purple-600" />
                Интерактивный тест API прогноза (
                <code className="font-mono text-purple-700">POST /api/v1/predict</code>)
              </span>
            </div>
            <form onSubmit={handlePredict} className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="block text-[10px] font-bold text-slate-500 mb-1">
                  Команда / группа
                </label>
                <select
                  value={groupName}
                  onChange={(e) => setGroupName(e.target.value)}
                  className="w-full bg-white border border-slate-200 rounded-lg px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-purple-500/20"
                >
                  <option value="Команда инфраструктуры SAN">Команда инфраструктуры SAN</option>
                  <option value="Команда виртуализации VMware">Команда виртуализации VMware</option>
                  <option value="Поддержка медиаплатформ L2">Поддержка медиаплатформ L2</option>
                  <option value="Администрирование Active Directory">
                    Администрирование Active Directory
                  </option>
                  <option value="Поддержка биллинговой платформы">
                    Поддержка биллинговой платформы
                  </option>
                  <option value="Платформа ITSM SimpleOne">Платформа ITSM SimpleOne</option>
                  <option value="Ситуационный центр L1">Ситуационный центр L1</option>
                </select>
              </div>
              <div>
                <label className="block text-[10px] font-bold text-slate-500 mb-1">Приоритет</label>
                <select
                  value={priority}
                  onChange={(e) => setPriority(e.target.value)}
                  className="w-full bg-white border border-slate-200 rounded-lg px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-purple-500/20"
                >
                  <option value="Critical">Наивысший (Critical)</option>
                  <option value="High">Высокий (High)</option>
                  <option value="Medium">Средний (Medium)</option>
                  <option value="Low">Низкий (Low)</option>
                </select>
              </div>
              <div>
                <label className="block text-[10px] font-bold text-slate-500 mb-1">
                  Норматив SLA (часов)
                </label>
                <input
                  type="number"
                  min={1}
                  max={72}
                  step={0.5}
                  value={slaHours}
                  onChange={(e) => setSlaHours(Number(e.target.value))}
                  className="w-full bg-white border border-slate-200 rounded-lg px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-purple-500/20"
                />
              </div>
              <div>
                <label className="block text-[10px] font-bold text-slate-500 mb-1">
                  Продукт / категория
                </label>
                <input
                  type="text"
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                  className="w-full bg-white border border-slate-200 rounded-lg px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-purple-500/20"
                />
              </div>
              <div className="sm:col-span-2">
                <label className="block text-[10px] font-bold text-slate-500 mb-1">
                  Тема заявки
                </label>
                <input
                  type="text"
                  value={titleText}
                  onChange={(e) => setTitleText(e.target.value)}
                  className="w-full bg-white border border-slate-200 rounded-lg px-2.5 py-1.5 text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-purple-500/20"
                />
              </div>
              <div className="sm:col-span-3 flex justify-end">
                <button
                  type="submit"
                  disabled={predicting}
                  className="flex items-center gap-1.5 bg-[#E30611] hover:bg-red-700 text-white font-bold px-4 py-2 rounded-lg text-xs shadow-sm transition-colors disabled:opacity-50"
                >
                  <Play className="h-3.5 w-3.5" />
                  {predicting ? 'Расчёт...' : 'Рассчитать прогноз'}
                </button>
              </div>
            </form>

            {predResult && (
              <div className="mt-3 p-3.5 rounded-xl bg-white border border-purple-200 space-y-2 text-[11px] shadow-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-3">
                    <span>
                      Прогноз:{' '}
                      <strong className="text-slate-900 font-mono">
                        {predResult.predicted_time_hours} ч
                      </strong>{' '}
                      <span className="text-slate-500 font-mono">
                        (80% ДИ: [{predResult.confidence_interval[0]}–
                        {predResult.confidence_interval[1]} ч])
                      </span>
                    </span>
                    <span>
                      С очередью:{' '}
                      <strong className="text-amber-700 font-mono">
                        {predResult.total_wait_hours} ч
                      </strong>
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span
                      className={`px-2 py-0.5 rounded font-bold text-[10px] border ${
                        predResult.sla_status === 'CRITICAL_SLA_BREACH'
                          ? 'bg-red-50 text-[#E30611] border-red-200'
                          : predResult.sla_status === 'WARNING'
                            ? 'bg-amber-50 text-amber-700 border-amber-200'
                            : 'bg-emerald-50 text-emerald-700 border-emerald-200'
                      }`}
                    >
                      {predResult.sla_status} ({(predResult.sla_risk_score * 100).toFixed(0)}%)
                    </span>
                    <span className="text-slate-400 font-mono">{predResult.latency_ms} мс</span>
                  </div>
                </div>
                <div className="flex flex-wrap gap-3 text-slate-600">
                  <span>
                    Очередь команды: <strong className="text-slate-900">{predResult.team_workload}</strong>
                  </span>
                  <span>
                    Медиана истории:{' '}
                    <strong className="text-slate-900">{predResult.history_category_median} ч</strong>
                  </span>
                  {predResult.recommended_group && (
                    <span className="text-emerald-700">
                      Рекомендованная команда: <strong>{predResult.recommended_group}</strong>
                    </span>
                  )}
                </div>
                {predResult.top_factors && predResult.top_factors.length > 0 && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    {predResult.top_factors.map((f, i) => (
                      <span
                        key={i}
                        className="px-2 py-0.5 rounded bg-slate-50 border border-slate-200 text-[10px] text-slate-700"
                      >
                        {f.factor}:{' '}
                        <strong
                          className={f.impactHours >= 0 ? 'text-[#E30611]' : 'text-emerald-700'}
                        >
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
