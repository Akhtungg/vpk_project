package ml

import (
	"testing"
	"time"

	"itsm-dashboard/backend/internal/models"
)

func TestUnifiedRegressionRoutingAndSLA(t *testing.T) {
	eng := &Engine{}
	if err := eng.LoadModels("models_ai"); err != nil {
		t.Fatalf("failed to load models: %v", err)
	}
	if eng.timeModel == nil {
		t.Fatalf("expected unified time_regression_model.json to be loaded")
	}
	if len(eng.timeModel.RoutingProfiles.Teams) == 0 {
		t.Fatalf("expected team centroids inside unified regression model")
	}

	now := time.Now()
	deadline := now.Add(4 * time.Hour)
	inc := &models.Incident{
		ID:                    "TEST-001",
		State:                 "Выполняется",
		SubmitDate:            &now,
		Deadline:              &deadline,
		Header:                "Критический отказ дискового массива SAN в кластере",
		Description:           "Задержки ввода-вывода SAN и ошибки синхронизации лунов, требуется срочное восстановление.",
		AssignedGroupName:     "КЦ ИТ Координаторы Массовых Инцидентов",
		AssignedEmployee:      "Иван Царевич (ivan_carevich)",
		AssignedEmployeeLogin: "ivan_carevich",
		PriorityID:            4,
		Priority:              "Наивысший",
		UrgencyID:             4,
		ImpactID:              4,
		ServiceType:           "Технологический",
		ProductName:           "Акваланг для муравьев",
	}

	ctx := &RuntimeContext{
		TeamWorkloads:        map[string]int{"КЦ ДОИ Администрирование SAN": 4},
		TeamQueueMinutes:     map[string]int{"КЦ ДОИ Администрирование SAN": 360},
		AssigneeWorkloads:    map[string]int{"ivan_carevich": 2},
		AssigneeQueueMinutes: map[string]int{"ivan_carevich": 180},
		HistoryMedians:       map[string]float64{"g|КЦ ДОИ Администрирование SAN": 3.1, "global": 1.8},
		HistoryCounts:        map[string]int{"g|КЦ ДОИ Администрирование SAN": 10, "global": 50},
		AssigneeSpeedFactors: map[string]float64{"ivan_carevich": 1.05, "alice": 0.85},
		GroupEmployees: map[string][]EmployeeCandidate{
			"КЦ ДОИ Администрирование SAN": {
				{Name: "Иван Царевич (ivan_carevich)", Login: "ivan_carevich", Workload: 2, QueueMin: 180, SpeedRatio: 1.05},
				{Name: "Алиса в Стране Чудес (alice)", Login: "alice", Workload: 0, QueueMin: 0, SpeedRatio: 0.85},
			},
		},
	}

	eng.Evaluate(inc, ctx)

	if inc.AIFallbackUsed {
		t.Errorf("expected unified regression model to be used, got fallback")
	}
	if inc.AIRoutingHint == "" {
		t.Errorf("expected team recommendation from unified model")
	}
	if len(inc.AIRoutingTop) == 0 || inc.AIRoutingTop[0].EstimatedMinutes <= 0 {
		t.Errorf("expected Top-3 teams with regression-estimated minutes, got %+v", inc.AIRoutingTop)
	}
	if inc.AIEstimatedMinutes < 15 {
		t.Errorf("expected estimated minutes >= 15, got %d", inc.AIEstimatedMinutes)
	}
	if inc.AIEstimatedMinMinutes > inc.AIEstimatedMinutes || inc.AIEstimatedMaxMinutes < inc.AIEstimatedMinutes {
		t.Errorf("invalid confidence interval [%d, %d] around %d", inc.AIEstimatedMinMinutes, inc.AIEstimatedMaxMinutes, inc.AIEstimatedMinutes)
	}
	if len(inc.AITopFactors) == 0 {
		t.Errorf("expected non-empty top factor contributions")
	}
	if inc.AISLALevel == "SAFE" {
		t.Errorf("expected WARNING or CRITICAL SLA risk with heavy queue and 4h deadline, got SAFE")
	}
}

func TestFallbackAndActualDurationError(t *testing.T) {
	eng := &Engine{} // no models loaded -> fallback path
	submit := time.Now().Add(-3 * time.Hour)
	comp := time.Now()
	inc := &models.Incident{
		ID:                "TEST-002",
		State:             "Закрыт",
		SubmitDate:        &submit,
		CompletionDate:    &comp,
		Header:            "Проверка fallback",
		AssignedGroupName: "КЦ ИТ Active Directory",
		PriorityID:        2,
		Priority:          "Средний",
	}

	eng.Evaluate(inc, nil)

	if !inc.AIFallbackUsed {
		t.Errorf("expected fallback flag when regression model is absent")
	}
	if inc.ActualDurationMinutes == nil || *inc.ActualDurationMinutes < 175 {
		t.Errorf("expected actual duration ~180 min, got %v", inc.ActualDurationMinutes)
	}
	if inc.PredictionErrorMinutes == nil {
		t.Errorf("expected prediction error to be calculated for closed ticket")
	}
}
