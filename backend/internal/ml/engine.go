package ml

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"itsm-dashboard/backend/internal/models"
)

type TreeNode struct {
	ChildrenLeft  []int     `json:"children_left"`
	ChildrenRight []int     `json:"children_right"`
	Feature       []int     `json:"feature"`
	Threshold     []float64 `json:"threshold"`
	Value         []float64 `json:"value"`
}

type GBDTEnsemble struct {
	Init            float64    `json:"init"`
	LearningRate    float64    `json:"learning_rate"`
	OutputTransform string     `json:"output_transform"`
	Trees           []TreeNode `json:"trees"`
}

type TimeModelMetrics struct {
	MAEHours           float64 `json:"mae_hours"`
	RMSEHours          float64 `json:"rmse_hours"`
	MAPEPercent        float64 `json:"mape_percent"`
	R2                 float64 `json:"r2"`
	SLAPrecision       float64 `json:"sla_precision"`
	ResidualStdHours   float64 `json:"residual_std_hours"`
	TrainSamples       int     `json:"train_samples"`
	TestSamples        int     `json:"test_samples"`
	TotalClosedSamples int     `json:"total_closed_samples"`
}

type TimeRegressionModelData struct {
	ModelName          string             `json:"model_name"`
	ModelVersion       string             `json:"model_version"`
	Features           []string           `json:"features"`
	BaselineValues     []float64          `json:"baseline_values"`
	FeatureImportances map[string]float64 `json:"feature_importances"`
	CriticalKeywords   []string           `json:"critical_keywords"`
	Metrics            TimeModelMetrics   `json:"metrics"`
	PointEnsemble      GBDTEnsemble       `json:"point_ensemble"`
	LowerEnsemble      GBDTEnsemble       `json:"lower_ensemble"`
	UpperEnsemble      GBDTEnsemble       `json:"upper_ensemble"`
}

type RuEmbeddingsData struct {
	TeacherModel     string               `json:"teacher_model"`
	Dim              int                  `json:"dim"`
	Vocab            map[string]int       `json:"vocab"`
	Scales           []float64            `json:"scales"`
	VectorsB64       string               `json:"vectors_b64"`
	ProjWeight       [][]float64          `json:"proj_weight"`
	ProjBias         []float64            `json:"proj_bias"`
	ComplexityAnchor []float64            `json:"complexity_anchor"`
	Teams            []string             `json:"teams"`
	TeamCentroids    map[string][]float64 `json:"team_centroids"`

	rawVectors []int8
}

type EmployeeCandidate struct {
	Name       string
	Login      string
	Group      string
	Workload   int
	QueueMin   int
	SpeedRatio float64
}

type RuntimeContext struct {
	TeamWorkloads        map[string]int
	TeamQueueMinutes     map[string]int
	AssigneeWorkloads    map[string]int
	AssigneeQueueMinutes map[string]int
	HistoryMedians       map[string]float64
	HistoryCounts        map[string]int
	AssigneeSpeedFactors map[string]float64
	GroupEmployees       map[string][]EmployeeCandidate
}

type Engine struct {
	mu          sync.RWMutex
	timeModel   *TimeRegressionModelData
	ruEmbed     *RuEmbeddingsData
	initialized bool
}

var defaultEngine *Engine
var once sync.Once

func GetEngine() *Engine {
	once.Do(func() {
		defaultEngine = &Engine{}
		_ = defaultEngine.LoadModels("models_ai")
	})
	return defaultEngine
}

func (e *Engine) LoadModels(baseDir string) error {
	dirs := []string{
		baseDir,
		filepath.Join("..", baseDir),
		filepath.Join("..", "..", baseDir),
		filepath.Join("backend", baseDir),
		filepath.Join("..", "..", "backend", "models_ai"),
	}

	var modelDir string
	for _, d := range dirs {
		p := filepath.Join(d, "time_regression_model.json")
		if _, err := os.Stat(p); err == nil {
			modelDir = d
			break
		}
	}

	if modelDir == "" {
		return nil
	}

	if data, err := os.ReadFile(filepath.Join(modelDir, "time_regression_model.json")); err == nil {
		var tm TimeRegressionModelData
		if err := json.Unmarshal(data, &tm); err == nil {
			e.timeModel = &tm
			e.initialized = true
		}
	}

	if data, err := os.ReadFile(filepath.Join(modelDir, "ru_embeddings.json")); err == nil {
		var re RuEmbeddingsData
		if err := json.Unmarshal(data, &re); err == nil && re.VectorsB64 != "" {
			if rawBytes, err := base64.StdEncoding.DecodeString(re.VectorsB64); err == nil {
				vecs := make([]int8, len(rawBytes))
				for i, b := range rawBytes {
					vecs[i] = int8(b)
				}
				re.rawVectors = vecs
				re.VectorsB64 = ""
				e.ruEmbed = &re
			}
		}
	}

	return nil
}

func (e *Engine) GetOfflineMetrics() (string, string, []string, map[string]float64, models.RegressionMetrics) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.timeModel == nil {
		return "unified_regression_rubert_v3", "heuristic_fallback", nil, nil, models.RegressionMetrics{}
	}
	m := e.timeModel.Metrics
	return e.timeModel.ModelName, e.timeModel.ModelVersion, e.timeModel.Features, e.timeModel.FeatureImportances, models.RegressionMetrics{
		MAEHours:           m.MAEHours,
		RMSEHours:          m.RMSEHours,
		MAPEPercent:        m.MAPEPercent,
		R2:                 m.R2,
		SLAPrecision:       m.SLAPrecision,
		ResidualStdHours:   m.ResidualStdHours,
		TrainSamples:       m.TrainSamples,
		TestSamples:        m.TestSamples,
		TotalClosedSamples: m.TotalClosedSamples,
	}
}

func (e *Engine) Evaluate(inc *models.Incident, ctxs ...*RuntimeContext) {
	var ctx *RuntimeContext
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}

	fullText := fmt.Sprintf("%s. %s. %s. %s. %s",
		inc.ProductName, inc.Service, inc.ScenarioName, inc.Header, inc.Description)
	emb := e.encodeRussianText(fullText)

	e.evaluateTeamRoutingWithEmbeddings(inc, ctx, emb, fullText)
	e.evaluateRegressionAndSLA(inc, ctx, emb)
}

func (e *Engine) hasCriticalKeyword(text string) bool {
	lower := strings.ToLower(text)
	keywords := []string{"авария", "массовый", "отказ", "недоступен", "падение", "критический", "срочно", "блок", "деградация", "обрыв", "down", "error"}
	if e.timeModel != nil && len(e.timeModel.CriticalKeywords) > 0 {
		keywords = e.timeModel.CriticalKeywords
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func normalizePriority(inc *models.Incident) int {
	if inc.PriorityID >= 1 && inc.PriorityID <= 4 {
		return inc.PriorityID
	}
	switch strings.ToLower(strings.TrimSpace(inc.Priority)) {
	case "наивысший", "критический", "critical":
		return 4
	case "высокий", "high":
		return 3
	case "средний", "medium":
		return 2
	default:
		return 1
	}
}

func splitBasicTokens(text string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func wordPieceSplitWord(word string, vocab map[string]int) ([]int, bool) {
	runes := []rune(word)
	n := len(runes)
	if n == 0 || n > 64 {
		return nil, false
	}
	var ids []int
	start := 0
	for start < n {
		end := n
		matched := -1
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := vocab[sub]; ok {
				matched = id
				break
			}
			end--
		}
		if matched == -1 {
			return nil, false
		}
		ids = append(ids, matched)
		start = end
	}
	return ids, true
}

func (e *Engine) encodeRussianText(text string) []float64 {
	if e.ruEmbed == nil || e.ruEmbed.Dim == 0 || len(e.ruEmbed.rawVectors) == 0 {
		return nil
	}
	re := e.ruEmbed
	dim := re.Dim

	words := splitBasicTokens(text)
	var tokenIDs []int
	for _, w := range words {
		if len([]rune(w)) < 2 {
			continue
		}
		if ids, ok := wordPieceSplitWord(w, re.Vocab); ok {
			tokenIDs = append(tokenIDs, ids...)
		} else if ids, ok := wordPieceSplitWord(strings.ToLower(w), re.Vocab); ok {
			tokenIDs = append(tokenIDs, ids...)
		}
	}

	if len(tokenIDs) == 0 {
		return nil
	}

	mean := make([]float64, dim)
	validCount := 0
	for _, tid := range tokenIDs {
		if tid < 0 || tid >= len(re.Scales) {
			continue
		}
		scale := re.Scales[tid] / 127.0
		offset := tid * dim
		if offset+dim > len(re.rawVectors) {
			continue
		}
		for j := 0; j < dim; j++ {
			mean[j] += float64(re.rawVectors[offset+j]) * scale
		}
		validCount++
	}
	if validCount == 0 {
		return nil
	}
	invN := 1.0 / float64(validCount)
	for j := 0; j < dim; j++ {
		mean[j] *= invN
	}

	out := make([]float64, dim)
	norm := 0.0
	for i := 0; i < dim; i++ {
		val := 0.0
		if i < len(re.ProjBias) {
			val = re.ProjBias[i]
		}
		if i < len(re.ProjWeight) {
			row := re.ProjWeight[i]
			for j := 0; j < dim && j < len(row); j++ {
				val += row[j] * mean[j]
			}
		}
		out[i] = val
		norm += val * val
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range out {
			out[i] /= norm
		}
	}
	return out
}

func dotProduct(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

func evalTree(t TreeNode, x []float64) float64 {
	node := 0
	for node >= 0 && node < len(t.ChildrenLeft) {
		left := t.ChildrenLeft[node]
		if left == -1 {
			if node < len(t.Value) {
				return t.Value[node]
			}
			return 0.0
		}
		featIdx := t.Feature[node]
		thr := t.Threshold[node]
		val := 0.0
		if featIdx >= 0 && featIdx < len(x) {
			val = x[featIdx]
		}
		if val <= thr {
			node = left
		} else {
			node = t.ChildrenRight[node]
		}
	}
	return 0.0
}

func evalEnsemble(ens GBDTEnsemble, x []float64) float64 {
	sum := ens.Init
	for _, tree := range ens.Trees {
		sum += ens.LearningRate * evalTree(tree, x)
	}
	if ens.OutputTransform == "expm1" {
		return math.Expm1(sum)
	}
	return sum
}

func defaultTeamMTTRHours(team string) float64 {
	teamLower := strings.ToLower(team)
	switch {
	case strings.Contains(teamLower, "san"), strings.Contains(teamLower, "vmware"), strings.Contains(teamLower, "dc-net"), strings.Contains(teamLower, "цод"), strings.Contains(teamLower, "приморье"):
		return 2.9
	case strings.Contains(teamLower, "streaming"), strings.Contains(teamLower, "media"), strings.Contains(teamLower, "billing"):
		return 1.4
	case strings.Contains(teamLower, "active directory"), strings.Contains(teamLower, "vpn"), strings.Contains(teamLower, "devops"):
		return 1.1
	case strings.Contains(teamLower, "autoclosed"):
		return 0.65
	default:
		return 1.6
	}
}

func lookupHistory(ctx *RuntimeContext, targetTeam, product string, prio int) (float64, int) {
	if ctx == nil || len(ctx.HistoryMedians) == 0 {
		return defaultTeamMTTRHours(targetTeam), 0
	}
	gpKey := fmt.Sprintf("gp|%s|%d", targetTeam, prio)
	if v, ok := ctx.HistoryMedians[gpKey]; ok && v > 0 {
		return v, ctx.HistoryCounts[gpKey]
	}
	gKey := fmt.Sprintf("g|%s", targetTeam)
	if v, ok := ctx.HistoryMedians[gKey]; ok && v > 0 {
		return v, ctx.HistoryCounts[gKey]
	}
	pKey := fmt.Sprintf("p|%s", product)
	if v, ok := ctx.HistoryMedians[pKey]; ok && v > 0 {
		return v, ctx.HistoryCounts[pKey]
	}
	if v, ok := ctx.HistoryMedians["global"]; ok && v > 0 {
		return v, ctx.HistoryCounts["global"]
	}
	return defaultTeamMTTRHours(targetTeam), 0
}

func computeSLATargetAndRemaining(inc *models.Incident, prio int) (float64, float64) {
	slaTargetHours := 16.0
	if inc.Deadline != nil && !inc.Deadline.IsZero() && inc.SubmitDate != nil && !inc.SubmitDate.IsZero() {
		diff := inc.Deadline.Sub(*inc.SubmitDate).Hours()
		if diff > 0.5 {
			slaTargetHours = diff
		}
	} else {
		switch prio {
		case 4:
			slaTargetHours = 4.0
		case 3:
			slaTargetHours = 8.0
		case 2:
			slaTargetHours = 16.0
		default:
			slaTargetHours = 24.0
		}
	}

	now := time.Now()
	var remainingMinutes float64
	if inc.Deadline != nil && !inc.Deadline.IsZero() {
		remainingMinutes = inc.Deadline.Sub(now).Minutes()
	} else if inc.SubmitDate != nil && !inc.SubmitDate.IsZero() {
		syntheticDeadline := inc.SubmitDate.Add(time.Duration(slaTargetHours) * time.Hour)
		remainingMinutes = syntheticDeadline.Sub(now).Minutes()
	} else {
		remainingMinutes = slaTargetHours * 60.0
	}
	return slaTargetHours, remainingMinutes
}

func computeSLARiskFromMinutes(totalExpectedMinutes, remainingMinutes float64) (float64, string) {
	if remainingMinutes <= 0 {
		return 99.0, "CRITICAL"
	}
	ratio := totalExpectedMinutes / remainingMinutes
	switch {
	case ratio >= 1.0:
		risk := 75.0 + math.Min(24.0, (ratio-1.0)*35.0)
		return math.Round(risk*10) / 10, "CRITICAL"
	case ratio >= 0.70:
		risk := 50.0 + ((ratio-0.70)/0.30)*25.0
		return math.Round(risk*10) / 10, "WARNING"
	default:
		risk := math.Max(5.0, ratio*50.0)
		return math.Round(risk*10) / 10, "SAFE"
	}
}

func (e *Engine) evaluateTeamRoutingWithEmbeddings(inc *models.Incident, ctx *RuntimeContext, emb []float64, fullText string) {
	if e.ruEmbed == nil || len(e.ruEmbed.Teams) == 0 || len(emb) == 0 {
		inc.AIRoutingHint = "КЦ ИТ Координаторы Массовых Инцидентов"
		inc.AIRoutingConfidence = 50.0
		return
	}

	prio := normalizePriority(inc)
	slaTargetHours, remainingMinutes := computeSLATargetAndRemaining(inc, prio)
	semComplexity := dotProduct(emb, e.ruEmbed.ComplexityAnchor)
	lowerText := strings.ToLower(fullText)

	type teamEval struct {
		group      string
		sim        float64
		estMinutes int
		slaRisk    float64
		workload   int
		score      float64
	}

	var rawSims []teamEval
	for _, team := range e.ruEmbed.Teams {
		if prio >= 3 && strings.Contains(strings.ToLower(team), "autoclosed") {
			continue
		}
		centroid := e.ruEmbed.TeamCentroids[team]
		sim := dotProduct(emb, centroid)

		teamLower := strings.ToLower(team)
		for _, kw := range []string{"san", "vmware", "streaming", "mediacore", "allure", "active directory", "vpn", "foris", "simpleone", "membrana", "dc-net", "mscp", "billing", "приморье"} {
			if strings.Contains(lowerText, kw) && strings.Contains(teamLower, kw) {
				sim += 0.08
				break
			}
		}
		rawSims = append(rawSims, teamEval{group: team, sim: sim})
	}

	sort.Slice(rawSims, func(i, j int) bool {
		return rawSims[i].sim > rawSims[j].sim
	})
	limit := 5
	if len(rawSims) < limit {
		limit = len(rawSims)
	}

	var evaluated []teamEval
	maxSim := rawSims[0].sim
	expSum := 0.0
	for i := 0; i < limit; i++ {
		cand := rawSims[i]
		wl := 0
		qMin := 0
		if ctx != nil {
			wl = ctx.TeamWorkloads[cand.group]
			qMin = ctx.TeamQueueMinutes[cand.group]
		}
		histMed, _ := lookupHistory(ctx, cand.group, inc.ProductName, prio)

		estMin := int(math.Round(histMed * 60.0))
		if e.timeModel != nil && len(e.timeModel.PointEnsemble.Trees) > 0 {
			x := e.buildRegressionFeatures(inc, cand.group, "", wl, 0, histMed, 1.0, slaTargetHours, semComplexity)
			estHours := math.Max(0.25, evalEnsemble(e.timeModel.PointEnsemble, x))
			estMin = int(math.Round(estHours * 60))
		}
		totalMin := float64(estMin) + float64(qMin)*0.35
		slaRisk, _ := computeSLARiskFromMinutes(totalMin, remainingMinutes)

		cand.estMinutes = estMin
		cand.slaRisk = slaRisk
		cand.workload = wl
		cand.score = cand.sim - (slaRisk/100.0)*0.03
		evaluated = append(evaluated, cand)

		expSum += math.Exp((cand.sim - maxSim) * 10.0)
	}

	sort.Slice(evaluated, func(i, j int) bool {
		return evaluated[i].score > evaluated[j].score
	})

	if len(evaluated) > 0 {
		best := evaluated[0]
		inc.AIRoutingHint = best.group

		bestProb := (math.Exp((best.sim-maxSim)*10.0) / expSum) * 100.0
		conf := 60.0 + bestProb*0.35
		if best.sim >= 0.80 {
			conf += 8.0
		}
		if conf > 98.0 {
			conf = 98.0
		}
		inc.AIRoutingConfidence = math.Round(conf*10) / 10

		topCount := 3
		if len(evaluated) < topCount {
			topCount = len(evaluated)
		}
		inc.AIRoutingTop = nil
		for i := 0; i < topCount; i++ {
			c := evaluated[i]
			prob := (math.Exp((c.sim-maxSim)*10.0) / expSum) * 100.0
			inc.AIRoutingTop = append(inc.AIRoutingTop, models.GroupScore{
				Group:            c.group,
				Confidence:       math.Round(prob*10) / 10,
				EstimatedMinutes: c.estMinutes,
				SLARisk:          c.slaRisk,
				Workload:         c.workload,
			})
		}
	}
}

func (e *Engine) buildRegressionFeatures(
	inc *models.Incident,
	targetTeam string,
	assigneeKey string,
	teamWorkload int,
	assigneeWorkload int,
	histMedianHours float64,
	speedFactor float64,
	slaTargetHours float64,
	semComplexity float64,
) []float64 {
	prio := float64(normalizePriority(inc))
	urg := float64(inc.UrgencyID)
	if urg == 0 {
		urg = prio
	}
	imp := float64(inc.ImpactID)
	if imp == 0 {
		imp = prio
	}
	isClient := 0.0
	if strings.Contains(strings.ToLower(inc.ServiceType), "клиент") {
		isClient = 1.0
	}

	submitTime := time.Now()
	if inc.SubmitDate != nil && !inc.SubmitDate.IsZero() {
		submitTime = *inc.SubmitDate
	}
	hour := float64(submitTime.Hour())
	wd := submitTime.Weekday()
	isBiz := 0.0
	if wd != time.Saturday && wd != time.Sunday && submitTime.Hour() >= 9 && submitTime.Hour() < 18 {
		isBiz = 1.0
	}

	text := inc.Header + " " + inc.Description
	titleLen := float64(len([]rune(text)))
	hasKw := 0.0
	if e.hasCriticalKeyword(text) {
		hasKw = 1.0
	}

	return []float64{
		prio,
		urg,
		imp,
		isClient,
		float64(teamWorkload),
		float64(assigneeWorkload),
		histMedianHours,
		speedFactor,
		slaTargetHours,
		hour,
		isBiz,
		titleLen,
		hasKw,
		semComplexity,
	}
}

func (e *Engine) computeTopFactors(x []float64, actualPredHours float64) []models.FactorContribution {
	if e.timeModel == nil || len(e.timeModel.BaselineValues) != len(x) {
		return nil
	}

	type factorGroup struct {
		name    string
		indices []int
	}
	groups := []factorGroup{
		{name: "История аналогичных заявок", indices: []int{6}},
		{name: "Загруженность команды (очередь)", indices: []int{4, 5}},
		{name: "Семантика текста (ruBERT)", indices: []int{11, 12, 13}},
		{name: "Приоритет и критичность", indices: []int{0, 1, 2}},
		{name: "Профиль производительности команды", indices: []int{7}},
		{name: "Регламент SLA и категория", indices: []int{3, 8}},
		{name: "Время суток и рабочие часы", indices: []int{9, 10}},
	}

	var contribs []models.FactorContribution
	for _, g := range groups {
		cf := make([]float64, len(x))
		copy(cf, x)
		for _, idx := range g.indices {
			cf[idx] = e.timeModel.BaselineValues[idx]
		}
		cfPred := math.Max(0.25, evalEnsemble(e.timeModel.PointEnsemble, cf))
		diff := math.Round((actualPredHours-cfPred)*100) / 100
		if math.Abs(diff) >= 0.03 {
			contribs = append(contribs, models.FactorContribution{
				Factor:      g.name,
				ImpactHours: diff,
			})
		}
	}

	sort.Slice(contribs, func(i, j int) bool {
		return math.Abs(contribs[i].ImpactHours) > math.Abs(contribs[j].ImpactHours)
	})
	if len(contribs) > 3 {
		contribs = contribs[:3]
	}
	return contribs
}

func (e *Engine) evaluateRegressionAndSLA(inc *models.Incident, ctx *RuntimeContext, emb []float64) {
	prio := normalizePriority(inc)

	targetTeam := inc.AssignedGroupName
	if targetTeam == "КЦ ИТ Координаторы Массовых Инцидентов" && inc.AIRoutingHint != "" {
		targetTeam = inc.AIRoutingHint
	}

	teamWorkload := 0
	teamQueueMin := 0
	assigneeWorkload := 0
	assigneeQueueMin := 0
	speedFactor := 1.0

	assigneeKey := strings.ToLower(strings.TrimSpace(inc.AssignedEmployeeLogin))
	if assigneeKey == "" {
		assigneeKey = strings.ToLower(strings.TrimSpace(inc.AssignedEmployee))
	}

	if ctx != nil {
		teamWorkload = ctx.TeamWorkloads[targetTeam]
		teamQueueMin = ctx.TeamQueueMinutes[targetTeam]
		if assigneeKey != "" {
			assigneeWorkload = ctx.AssigneeWorkloads[assigneeKey]
			assigneeQueueMin = ctx.AssigneeQueueMinutes[assigneeKey]
			if sf, ok := ctx.AssigneeSpeedFactors[assigneeKey]; ok && sf > 0 {
				speedFactor = sf
			}
		}
	}

	isResolved := inc.State == "Завершен" || inc.State == "Закрыт"
	if !isResolved && inc.AssignedGroupName == targetTeam && teamWorkload > 0 {
		teamWorkload--
		if teamQueueMin > 0 {
			avg := teamQueueMin / (teamWorkload + 1)
			teamQueueMin = maxInt(0, teamQueueMin-avg)
		}
	}
	if !isResolved && assigneeWorkload > 0 {
		assigneeWorkload--
		if assigneeQueueMin > 0 {
			avg := assigneeQueueMin / (assigneeWorkload + 1)
			assigneeQueueMin = maxInt(0, assigneeQueueMin-avg)
		}
	}

	inc.AITeamWorkload = teamWorkload
	inc.AIAssigneeWorkload = assigneeWorkload
	inc.AIAssigneeSpeedFactor = math.Round(speedFactor*100) / 100

	effectiveQueueMin := teamQueueMin
	if assigneeQueueMin > 0 {
		effectiveQueueMin = int(math.Round(float64(assigneeQueueMin)*0.6 + float64(teamQueueMin)*0.4))
	} else if teamQueueMin == 0 && teamWorkload > 0 {
		effectiveQueueMin = teamWorkload * 60
	}
	inc.AIQueueRemainingMinutes = effectiveQueueMin

	histMedianHours, histCount := lookupHistory(ctx, targetTeam, inc.ProductName, prio)
	inc.AIHistoryMedianMinutes = int(math.Round(histMedianHours * 60))
	inc.AILowConfidence = histCount < 3

	slaTargetHours, remainingMinutes := computeSLATargetAndRemaining(inc, prio)
	semComplexity := 0.0
	if e.ruEmbed != nil && len(emb) > 0 {
		semComplexity = dotProduct(emb, e.ruEmbed.ComplexityAnchor)
	}

	x := e.buildRegressionFeatures(
		inc, targetTeam, assigneeKey,
		teamWorkload, assigneeWorkload,
		histMedianHours, speedFactor, slaTargetHours, semComplexity,
	)

	var estHours, minHours, maxHours float64
	if e.timeModel != nil && len(e.timeModel.PointEnsemble.Trees) > 0 {
		estHours = math.Max(0.25, evalEnsemble(e.timeModel.PointEnsemble, x))
		minHours = math.Max(0.20, evalEnsemble(e.timeModel.LowerEnsemble, x))
		maxHours = math.Max(estHours, evalEnsemble(e.timeModel.UpperEnsemble, x))
		if minHours > estHours {
			minHours = math.Max(0.20, estHours*0.8)
		}
		if inc.AILowConfidence {
			minHours = math.Min(minHours, estHours*0.70)
			maxHours = math.Max(maxHours, estHours*1.50)
		}
		inc.AIFallbackUsed = false
		inc.AIModelVersion = e.timeModel.ModelVersion
		inc.AITopFactors = e.computeTopFactors(x, estHours)
	} else {
		estHours = math.Max(0.25, histMedianHours*(1.0+0.12*float64(prio))*(1.0+0.18*float64(teamWorkload))*speedFactor)
		minHours = math.Max(0.20, estHours*0.70)
		maxHours = estHours * 1.40
		inc.AIFallbackUsed = true
		inc.AIModelVersion = "heuristic_fallback"
	}

	inc.AIEstimatedMinutes = int(math.Round(estHours * 60))
	inc.AIEstimatedMinMinutes = int(math.Round(minHours * 60))
	inc.AIEstimatedMaxMinutes = int(math.Round(maxHours * 60))

	if isResolved && inc.CompletionDate != nil && inc.SubmitDate != nil {
		actualMin := int(math.Round(inc.CompletionDate.Sub(*inc.SubmitDate).Minutes()))
		if actualMin > 0 {
			inc.ActualDurationMinutes = &actualMin
			errMin := actualMin - inc.AIEstimatedMinutes
			inc.PredictionErrorMinutes = &errMin
		}
	}

	if isResolved {
		inc.AISLARisk = 0.0
		inc.AISLALevel = "SAFE"
		inc.AISLAMarginMinutes = 0
		return
	}

	effectivePureMinutes := float64(inc.AIEstimatedMinutes)
	if inc.ManualOverrideMinutes != nil && *inc.ManualOverrideMinutes > 0 {
		effectivePureMinutes = float64(*inc.ManualOverrideMinutes)
	}

	totalExpectedMinutes := effectivePureMinutes + float64(effectiveQueueMin)*0.35
	margin := remainingMinutes - totalExpectedMinutes
	inc.AISLAMarginMinutes = int(math.Round(margin))

	inc.AISLARisk, inc.AISLALevel = computeSLARiskFromMinutes(totalExpectedMinutes, remainingMinutes)

	inc.AIAltAssignee = ""
	inc.AIAltEstimatedMinutes = 0
	if (inc.AISLALevel == "WARNING" || inc.AISLALevel == "CRITICAL") && ctx != nil && e.timeModel != nil {
		bestName := ""
		bestMinutes := int(math.Round(totalExpectedMinutes))
		candidates := ctx.GroupEmployees[targetTeam]
		for _, cand := range candidates {
			if strings.EqualFold(cand.Login, inc.AssignedEmployeeLogin) || strings.EqualFold(cand.Name, inc.AssignedEmployee) {
				continue
			}
			candX := e.buildRegressionFeatures(
				inc, targetTeam, cand.Login,
				teamWorkload, cand.Workload,
				histMedianHours, cand.SpeedRatio, slaTargetHours, semComplexity,
			)
			candPure := math.Max(0.25, evalEnsemble(e.timeModel.PointEnsemble, candX)) * 60.0
			candTotal := int(math.Round(candPure + float64(cand.QueueMin)*0.35))
			if candTotal+10 < bestMinutes {
				bestMinutes = candTotal
				bestName = cand.Name
			}
		}
		if bestName != "" {
			inc.AIAltAssignee = bestName
			inc.AIAltEstimatedMinutes = bestMinutes
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
