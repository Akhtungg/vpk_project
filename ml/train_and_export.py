import json
import math
import os
from datetime import datetime
import numpy as np
from sklearn.ensemble import GradientBoostingRegressor
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics import mean_absolute_error, mean_squared_error, r2_score
from sklearn.model_selection import train_test_split
from skl2onnx import convert_sklearn
from skl2onnx.common.data_types import FloatTensorType

DATA_PATH = os.path.join(os.path.dirname(__file__), "..", "data", "character_problems.json")
OUTPUT_DIR = os.path.join(os.path.dirname(__file__), "..", "backend", "models_ai")
os.makedirs(OUTPUT_DIR, exist_ok=True)

# Clean up deprecated standalone classification model artifacts
for old_file in [
    "risk_assessment.onnx",
    "risk_assessment_model.json",
    "routing_hint.onnx",
    "routing_hint_model.json",
]:
    old_path = os.path.join(OUTPUT_DIR, old_file)
    if os.path.exists(old_path):
        os.remove(old_path)
        print(f"Removed deprecated model file: {old_file}")

with open(DATA_PATH, encoding="utf-8") as f:
    raw_data = json.load(f)

print(f"Loaded {len(raw_data)} incidents from {DATA_PATH}")

CRITICAL_KEYWORDS = [
    "отказ", "авария", "массовый", "недоступен", "падение",
    "критический", "срочно", "блок", "dead", "fatal", "outage"
]

# -------------------------------------------------------------
# 1. TEAM TEXT CENTROIDS (Domain profile vectors for routing inside the unified model)
# -------------------------------------------------------------
texts = []
groups = []
for d in raw_data:
    g = d.get("assignedGroupName", "")
    if not g or "координатор" in g.lower():
        continue
    txt = f"{d.get('productName', '')} {d.get('service', '')} {d.get('scenarioName', '')} {d.get('header', '')} {d.get('description', '')}"
    texts.append(txt)
    groups.append(g)

vectorizer = TfidfVectorizer(max_features=300, ngram_range=(1, 2))
X_tfidf = vectorizer.fit_transform(texts).toarray()

unique_teams = sorted(set(groups))
team_centroids = {}
for team in unique_teams:
    idxs = [i for i, g in enumerate(groups) if g == team]
    centroid = np.mean(X_tfidf[idxs], axis=0)
    norm = np.linalg.norm(centroid)
    if norm > 0:
        centroid = centroid / norm
    team_centroids[team] = [round(float(v), 6) for v in centroid]

print(f"Computed TF-IDF domain centroids for {len(unique_teams)} execution teams")

# -------------------------------------------------------------
# 2. UNIFIED REGRESSION MODEL (Time Prediction + 80% CI + SLA Risk + Team Selection)
# -------------------------------------------------------------
def parse_dt(val):
    if not val:
        return None
    try:
        return datetime.fromisoformat(val.replace("Z", "+00:00"))
    except Exception:
        return None


closed_tickets = []
for d in raw_data:
    s_dt = parse_dt(d.get("submitDate"))
    c_dt = parse_dt(d.get("completionDate"))
    if not s_dt or not c_dt:
        continue
    dur_hours = (c_dt - s_dt).total_seconds() / 3600.0
    if dur_hours <= 0.05:
        continue
    d_dt = parse_dt(d.get("deadline"))
    sla_hours = (d_dt - s_dt).total_seconds() / 3600.0 if d_dt else 16.0
    closed_tickets.append((s_dt, c_dt, max(1.0, sla_hours), dur_hours, d))

closed_tickets.sort(key=lambda x: x[0])
print(f"Closed tickets for regression: {len(closed_tickets)}")

REGRESSION_FEATURES = [
    "priority_id",
    "urgency_id",
    "impact_id",
    "is_client_service",
    "team_workload",
    "assignee_workload",
    "history_median_hours",
    "assignee_speed_factor",
    "sla_target_hours",
    "created_hour",
    "is_business_hours",
    "title_length",
    "has_critical_keywords",
]

past_intervals = []
X_reg = []
y_reg = []
sla_targets = []

for s_dt, c_dt, sla_hours, dur_hours, d in closed_tickets:
    group = d.get("assignedGroupName", "")
    emp = d.get("assignedEmployeeLogin", "")
    product = d.get("productName", "")
    prio = int(d.get("priorityId") or 2)
    urg = int(d.get("urgencyId") or prio)
    imp = int(d.get("impactId") or prio)
    is_client = 1.0 if "клиент" in (d.get("serviceType") or "").lower() else 0.0

    team_q = sum(1 for ps, pc, pg, _, _, _, _ in past_intervals if pg == group and ps <= s_dt < pc)
    emp_q = sum(1 for ps, pc, _, pe, _, _, _ in past_intervals if emp and pe == emp and ps <= s_dt < pc)

    finished_past = [row for row in past_intervals if row[1] <= s_dt]
    global_durs = [row[4] for row in finished_past]
    global_med = float(np.median(global_durs)) if global_durs else 2.5

    gp_durs = [row[4] for row in finished_past if row[2] == group and row[6] == prio]
    g_durs = [row[4] for row in finished_past if row[2] == group]
    p_durs = [row[4] for row in finished_past if row[5] == product]
    if len(gp_durs) >= 1:
        hist_med = float(np.median(gp_durs))
    elif len(g_durs) >= 1:
        hist_med = float(np.median(g_durs))
    elif len(p_durs) >= 1:
        hist_med = float(np.median(p_durs))
    else:
        hist_med = global_med

    emp_durs = [row[4] for row in finished_past if emp and row[3] == emp]
    if len(emp_durs) >= 2 and global_med > 0:
        speed_factor = float(np.clip(np.median(emp_durs) / global_med, 0.65, 1.50))
    else:
        speed_factor = 1.0

    hour = s_dt.hour
    is_biz = 1.0 if (s_dt.weekday() < 5 and 9 <= hour < 18) else 0.0
    text = f"{d.get('header', '')} {d.get('description', '')}"
    title_len = float(len(text))
    text_low = text.lower()
    has_kw = 1.0 if any(k in text_low for k in CRITICAL_KEYWORDS) else 0.0

    X_reg.append([
        float(prio), float(urg), float(imp), is_client,
        float(team_q), float(emp_q), hist_med, speed_factor,
        float(sla_hours), float(hour), is_biz, title_len, has_kw
    ])
    y_reg.append(dur_hours)
    sla_targets.append(sla_hours)

    past_intervals.append((s_dt, c_dt, group, emp, dur_hours, product, prio))

X_reg = np.array(X_reg, dtype=np.float32)
y_reg = np.array(y_reg, dtype=np.float32)
sla_arr = np.array(sla_targets, dtype=np.float32)

X_train, X_test, y_train, y_test, _, sla_test = train_test_split(
    X_reg, y_reg, sla_arr, test_size=0.2, random_state=42
)

point_model = GradientBoostingRegressor(
    loss="huber", alpha=0.85, n_estimators=100, max_depth=3, learning_rate=0.07, random_state=42
)
lower_model = GradientBoostingRegressor(
    loss="quantile", alpha=0.10, n_estimators=60, max_depth=3, learning_rate=0.08, random_state=42
)
upper_model = GradientBoostingRegressor(
    loss="quantile", alpha=0.90, n_estimators=60, max_depth=3, learning_rate=0.08, random_state=42
)

point_model.fit(X_train, y_train)
lower_model.fit(X_train, y_train)
upper_model.fit(X_train, y_train)

y_pred = np.maximum(0.25, point_model.predict(X_test))
mae = float(mean_absolute_error(y_test, y_pred))
rmse = float(math.sqrt(mean_squared_error(y_test, y_pred)))
mape = float(np.mean(np.abs((y_test - y_pred) / np.maximum(0.25, y_test))) * 100.0)
r2 = float(r2_score(y_test, y_pred))
residual_std = float(np.std(y_test - y_pred))

pred_breach = y_pred >= (0.70 * sla_test)
actual_breach = y_test >= (0.70 * sla_test)
tp = int(np.sum(pred_breach & actual_breach))
fp = int(np.sum(pred_breach & ~actual_breach))
sla_precision = float(tp / (tp + fp)) if (tp + fp) > 0 else 1.0

print(f"Unified Regression Metrics -> MAE: {mae:.3f}h, RMSE: {rmse:.3f}h, MAPE: {mape:.1f}%, R2: {r2:.3f}, SLA Precision: {sla_precision:.3f}")


def export_gbdt(model: GradientBoostingRegressor) -> dict:
    init_val = float(model.init_.predict(np.zeros((1, X_reg.shape[1]), dtype=np.float32))[0])
    trees = []
    for est_arr in model.estimators_:
        t = est_arr[0].tree_
        trees.append({
            "children_left": t.children_left.tolist(),
            "children_right": t.children_right.tolist(),
            "feature": t.feature.tolist(),
            "threshold": [round(float(x), 6) for x in t.threshold.tolist()],
            "value": [round(float(v[0][0]), 6) for v in t.value],
        })
    return {
        "init": round(init_val, 6),
        "learning_rate": float(model.learning_rate),
        "trees": trees,
    }


baseline_values = [round(float(v), 4) for v in np.median(X_train, axis=0)]
feat_importances = {
    name: round(float(imp), 4)
    for name, imp in zip(REGRESSION_FEATURES, point_model.feature_importances_)
}

reg_json = {
    "model_name": "unified_regression_sla_routing",
    "model_version": "gbdt_reg_v2.0",
    "features": REGRESSION_FEATURES,
    "baseline_values": baseline_values,
    "feature_importances": feat_importances,
    "critical_keywords": CRITICAL_KEYWORDS,
    "metrics": {
        "mae_hours": round(mae, 3),
        "rmse_hours": round(rmse, 3),
        "mape_percent": round(mape, 2),
        "r2": round(r2, 3),
        "sla_precision": round(sla_precision, 3),
        "residual_std_hours": round(residual_std, 3),
        "train_samples": int(len(X_train)),
        "test_samples": int(len(X_test)),
        "total_closed_samples": int(len(X_reg)),
    },
    "point_ensemble": export_gbdt(point_model),
    "lower_ensemble": export_gbdt(lower_model),
    "upper_ensemble": export_gbdt(upper_model),
    "routing_profiles": {
        "teams": unique_teams,
        "vocab": {k: int(v) for k, v in vectorizer.vocabulary_.items()},
        "idf": [round(float(x), 6) for x in vectorizer.idf_.tolist()],
        "centroids": team_centroids,
    },
}

with open(os.path.join(OUTPUT_DIR, "time_regression_model.json"), "w", encoding="utf-8") as f:
    json.dump(reg_json, f, ensure_ascii=False, indent=2)
print("Exported unified time_regression_model.json")

onnx_reg = convert_sklearn(
    point_model, initial_types=[("float_input", FloatTensorType([None, X_reg.shape[1]]))]
)
with open(os.path.join(OUTPUT_DIR, "time_regression.onnx"), "wb") as f:
    f.write(onnx_reg.SerializeToString())
print("Exported time_regression.onnx")
print("Unified regression ML training and export completed successfully!")
