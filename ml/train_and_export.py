import base64
import heapq
import json
import math
import os
import re
from datetime import datetime
import numpy as np
import pandas as pd
import torch
import torch.nn as nn
import torch.nn.functional as F
from sklearn.decomposition import TruncatedSVD
from sklearn.ensemble import GradientBoostingRegressor
from sklearn.metrics import mean_absolute_error, mean_squared_error, r2_score
from sklearn.model_selection import train_test_split
from skl2onnx import convert_sklearn
from skl2onnx.common.data_types import FloatTensorType
from transformers import AutoModel, AutoTokenizer

BASE_DIR = os.path.dirname(__file__)
DATA_PATH = os.path.join(BASE_DIR, "..", "data", "character_problems.json")
UCI_PATH = os.path.join(BASE_DIR, "..", "data", "external", "incident_event_log.csv")
ITSM_PATH = os.path.join(BASE_DIR, "..", "data", "external", "itsm.csv")
OUTPUT_DIR = os.path.join(BASE_DIR, "..", "backend", "models_ai")
os.makedirs(OUTPUT_DIR, exist_ok=True)

for old_file in [
    "risk_assessment.onnx",
    "risk_assessment_model.json",
    "routing_hint.onnx",
    "routing_hint_model.json",
]:
    old_path = os.path.join(OUTPUT_DIR, old_file)
    if os.path.exists(old_path):
        os.remove(old_path)

with open(DATA_PATH, encoding="utf-8") as f:
    raw_data = json.load(f)

print(f"Loaded {len(raw_data)} local technical incidents from {DATA_PATH}")

CRITICAL_KEYWORDS = [
    "отказ", "авария", "массовый", "недоступен", "падение",
    "критический", "срочно", "блок", "деградация", "обрыв", "dead", "fatal", "outage"
]

MODEL_ID = "cointegrated/rubert-tiny2"
print(f"Loading Russian transformer model: {MODEL_ID}")
tokenizer = AutoTokenizer.from_pretrained(MODEL_ID)
bert = AutoModel.from_pretrained(MODEL_ID)
bert.eval()


def encode_bert(text_list: list[str], batch_size: int = 64) -> np.ndarray:
    embs = []
    with torch.no_grad():
        for i in range(0, len(text_list), batch_size):
            enc = tokenizer(
                text_list[i : i + batch_size],
                padding=True,
                truncation=True,
                max_length=128,
                return_tensors="pt",
            )
            out = bert(**enc)
            mask = enc["attention_mask"].unsqueeze(-1).float()
            pooled = (out.last_hidden_state * mask).sum(dim=1) / mask.sum(dim=1).clamp(min=1e-9)
            pooled = F.normalize(pooled, p=2, dim=1)
            embs.append(pooled.cpu().numpy())
    return np.vstack(embs)


def ticket_text(d: dict) -> str:
    return f"{d.get('productName', '')}. {d.get('service', '')}. {d.get('scenarioName', '')}. {d.get('header', '')}. {d.get('description', '')}"


route_texts = []
route_groups = []
for d in raw_data:
    g = d.get("assignedGroupName", "")
    if not g or "координатор" in g.lower():
        continue
    route_texts.append(ticket_text(d))
    route_groups.append(g)

aug_texts = [
    "Отказ дискового массива SAN, задержки ввода-вывода Fibre Channel, ошибки репликации LUN и СХД.",
    "Деградация дискового хранилища, ошибки синхронизации томов LUN.",
    "Высокая нагрузка на гипервизор VMWare ESXi, медленная работа виртуальных машин, ошибки vMotion.",
    "Сбой кластера виртуализации, недоступны виртуальные серверы и снапшоты ESXi.",
    "Сотрудники не могут войти по VPN, обрыв туннеля IPsec и WireGuard, ошибки TLS рукопожатия.",
    "Проблема удаленного доступа VPN, сброс сессий удаленных пользователей.",
    "Ошибки авторизации в Active Directory, блокировка учетной записи, сбой Kerberos и LDAP.",
    "Недоступен контроллер домена AD, пользователи не могут войти в домен Windows.",
    "Отказ воспроизведения видеопотока и плеера, ошибки транскодирования видео и CDN стриминга.",
    "Буферизация и обрывы трансляции видео на клиентском веб-плеере MediaCore.",
    "Ошибки загрузки личного кабинета на клиентском веб-портале, долгий отклик интерфейса.",
    "Сбой фронтенда веб-портала и сервиса пользовательских сессий.",
    "Задержка проведения платежей в биллинге, очередь транзакций платежного шлюза BillingCore.",
    "Сбой онлайн-тарификации и списания балансов в системе Foris.",
    "Остановка пайплайнов сборки Docker и деплоя сервисов в кластере Kubernetes и GitLab CI.",
    "Ошибка загрузки отчетов автотестов в Allure TestOps, таймаут CI-пайплайна.",
    "Зависание сценариев согласования заявок и обновления записей CMDB в SimpleOne.",
    "Потеря пакетов, флапы BGP-сессий на магистральных коммутаторах сети ЦОД DC-NET.",
    "Авария электропитания на распределительном щите, сработал ИБП и охлаждение стойки в ЦОД СПб.",
    "Пропадание питания на региональном узле связи ОГЭ Приморье, переход на дизель-генератор.",
    "Рост ошибок 503 на API-шлюзе микросервисов MSCP и балансировщике нагрузки.",
    "Рост отставания lag в топиках Kafka интеграционной шины данных Enablement.",
    "Блокировки и медленные запросы в кластере баз данных PostgreSQL и очередей ОРД МедиаСкаут.",
    "Рассинхронизация витрины цен и номенклатуры в API каталога продуктов Product Catalog.",
    "Ошибки применения правил фильтрации трафика и подписок в сервисе Membrana.",
    "Кратковременный скачок задержки пинга, узел восстановился автоматически.",
]

all_ticket_texts = [ticket_text(d) for d in raw_data]
all_teacher_texts = route_texts + aug_texts

E_teacher_312 = encode_bert(all_teacher_texts)
svd = TruncatedSVD(n_components=64, random_state=42)
E_teacher_64 = svd.fit_transform(E_teacher_312)
E_teacher_64 /= np.linalg.norm(E_teacher_64, axis=1, keepdims=True) + 1e-9

E_all_tickets_64 = svd.transform(encode_bert(all_ticket_texts))
E_all_tickets_64 /= np.linalg.norm(E_all_tickets_64, axis=1, keepdims=True) + 1e-9

raw_word_emb = bert.embeddings.word_embeddings.weight.detach().cpu().numpy()
init_64 = svd.transform(raw_word_emb)

student_emb = nn.Embedding.from_pretrained(torch.tensor(init_64, dtype=torch.float32), freeze=False)
proj = nn.Linear(64, 64)
nn.init.eye_(proj.weight)
nn.init.zeros_(proj.bias)

opt = torch.optim.Adam(list(student_emb.parameters()) + list(proj.parameters()), lr=0.02)
token_lists = [tokenizer.encode(t, add_special_tokens=False, truncation=True, max_length=128) for t in all_teacher_texts]
target_t = torch.tensor(E_teacher_64, dtype=torch.float32)

for epoch in range(50):
    opt.zero_grad()
    preds = []
    for ids in token_lists:
        idx_t = torch.tensor(ids if ids else [0], dtype=torch.long)
        preds.append(student_emb(idx_t).mean(dim=0))
    P = F.normalize(proj(torch.stack(preds)), p=2, dim=1)
    loss = (1.0 - (P * target_t).sum(dim=1)).mean()
    loss.backward()
    opt.step()

print(f"Distilled rubert-tiny2 WordPiece encoder -> cosine loss={loss.item():.4f}")

unique_teams = sorted(set(route_groups))
team_centroids = {}
for t in unique_teams:
    idxs = [i for i, g in enumerate(route_groups) if g == t]
    c = np.mean(E_teacher_64[idxs], axis=0)
    c /= np.linalg.norm(c) + 1e-9
    team_centroids[t] = [round(float(v), 6) for v in c]

vocab = tokenizer.get_vocab()
keep_tokens = []
used_ids = set(i for ids in token_lists for i in ids)
for tok, tid in vocab.items():
    if tok.startswith("[") and tok.endswith("]"):
        continue
    if tid in used_ids or (tid < 35000 and re.match(r"^(##)?[a-zA-Zа-яА-ЯёЁ0-9_-]{2,}$", tok)):
        keep_tokens.append((tok, tid))
keep_tokens.sort(key=lambda x: x[1])

emb_weights = student_emb.weight.detach().cpu().numpy()
kept_vecs = emb_weights[[tid for _, tid in keep_tokens]]
scales = np.max(np.abs(kept_vecs), axis=1).astype(np.float32)
scales = np.maximum(scales, 1e-6)
quantized = np.clip(np.round((kept_vecs / scales[:, None]) * 127.0), -127, 127).astype(np.int8)


def parse_dt(val):
    if not val:
        return None
    try:
        return datetime.fromisoformat(val.replace("Z", "+00:00"))
    except Exception:
        return None


closed_with_idx = []
for idx, d in enumerate(raw_data):
    s_dt = parse_dt(d.get("submitDate"))
    c_dt = parse_dt(d.get("completionDate"))
    if not s_dt or not c_dt:
        continue
    dur_hours = (c_dt - s_dt).total_seconds() / 3600.0
    if dur_hours <= 0.05:
        continue
    d_dt = parse_dt(d.get("deadline"))
    sla_hours = (d_dt - s_dt).total_seconds() / 3600.0 if d_dt else 16.0
    closed_with_idx.append((s_dt, c_dt, max(1.0, sla_hours), dur_hours, d, E_all_tickets_64[idx]))

closed_with_idx.sort(key=lambda x: x[0])

durs_arr = np.array([x[3] for x in closed_with_idx])
hi_thr = np.percentile(durs_arr, 70)
lo_thr = np.percentile(durs_arr, 30)
hi_embs = np.mean([x[5] for x in closed_with_idx if x[3] >= hi_thr], axis=0)
lo_embs = np.mean([x[5] for x in closed_with_idx if x[3] <= lo_thr], axis=0)
complexity_anchor = hi_embs - lo_embs
complexity_anchor /= np.linalg.norm(complexity_anchor) + 1e-9

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
    "semantic_complexity",
]

past_intervals = []
Xl, yl, sl = [], [], []
for s_dt, c_dt, sla_hours, dur_hours, d, emb64 in closed_with_idx:
    group = d.get("assignedGroupName", "")
    emp = d.get("assignedEmployeeLogin", "")
    product = d.get("productName", "")
    prio = int(d.get("priorityId") or 2)
    urg = int(d.get("urgencyId") or prio)
    imp = int(d.get("impactId") or prio)
    is_client = 1.0 if "клиент" in (d.get("serviceType") or "").lower() else 0.0

    team_q = sum(1 for ps, pc, pg, *_ in past_intervals if pg == group and ps <= s_dt < pc)
    emp_q = sum(1 for ps, pc, _, pe, *_ in past_intervals if emp and pe == emp and ps <= s_dt < pc)

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
    speed_factor = float(np.clip(np.median(emp_durs) / global_med, 0.65, 1.50)) if (len(emp_durs) >= 2 and global_med > 0) else 1.0

    hour = s_dt.hour
    is_biz = 1.0 if (s_dt.weekday() < 5 and 9 <= hour < 18) else 0.0
    text = f"{d.get('header', '')} {d.get('description', '')}"
    has_kw = 1.0 if any(k in text.lower() for k in CRITICAL_KEYWORDS) else 0.0
    sem_comp = float(np.dot(emb64, complexity_anchor))

    Xl.append([
        float(prio), float(urg), float(imp), is_client,
        float(team_q), float(emp_q), hist_med, speed_factor,
        float(sla_hours), float(hour), is_biz, float(len(text)), has_kw, sem_comp
    ])
    yl.append(dur_hours)
    sl.append(sla_hours)
    past_intervals.append((s_dt, c_dt, group, emp, dur_hours, product, prio))

Xl = np.array(Xl, dtype=np.float32)
yl = np.array(yl, dtype=np.float32)
sl = np.array(sl, dtype=np.float32)

Xl_train, Xl_test, yl_train, yl_test, _, sl_test = train_test_split(
    Xl, yl, sl, test_size=0.2, random_state=42
)

ext_X_parts, ext_y_parts = [], []
ext_counts = {"itsm": 0, "uci": 0}

if os.path.exists(ITSM_PATH):
    itsm = pd.read_csv(ITSM_PATH, low_memory=False)
    itsm["opened_dt"] = pd.to_datetime(itsm["Open_Time"], dayfirst=True, errors="coerce")
    itsm["resolved_dt"] = pd.to_datetime(itsm["Resolved_Time"], dayfirst=True, errors="coerce")
    itsm["dur_hours"] = (itsm["resolved_dt"] - itsm["opened_dt"]).dt.total_seconds() / 3600.0
    itsm["Priority"] = pd.to_numeric(itsm["Priority"], errors="coerce")
    itsm["Urgency"] = pd.to_numeric(itsm["Urgency"], errors="coerce")
    itsm["Impact"] = pd.to_numeric(itsm["Impact"], errors="coerce")
    itsm_clean = itsm[
        (itsm["No_of_Reassignments"].fillna(0) <= 1) & (itsm["dur_hours"] >= 0.35) & (itsm["dur_hours"] <= 12.0)
    ].sort_values("opened_dt")

    g_act, gp_d, g_d, all_d = {}, {}, {}, []
    Xi, yi = [], []
    for r in itsm_clean.itertuples():
        st, ct = r.opened_dt.timestamp(), r.resolved_dt.timestamp()
        grp = str(r.CI_Subcat if pd.notna(r.CI_Subcat) else r.CI_Cat)
        p_raw = float(r.Priority) if pd.notna(r.Priority) else 4.0
        prio = 4 if p_raw <= 2 else (3 if p_raw == 3 else (2 if p_raw == 4 else 1))
        u_raw = float(r.Urgency) if pd.notna(r.Urgency) else p_raw
        urg = 4 if u_raw <= 2 else (3 if u_raw == 3 else (2 if u_raw == 4 else 1))
        i_raw = float(r.Impact) if pd.notna(r.Impact) else p_raw
        imp = 4 if i_raw <= 2 else (3 if i_raw == 3 else (2 if i_raw == 4 else 1))

        gh = g_act.setdefault(grp, [])
        while gh and gh[0] <= st:
            heapq.heappop(gh)
        tq = len(gh)
        heapq.heappush(gh, ct)

        gmed = float(np.median(all_d[-500:])) if all_d else 1.4
        if len(gp_d.get((grp, prio), [])) >= 3:
            hm = float(np.median(gp_d[(grp, prio)][-30:]))
        elif len(g_d.get(grp, [])) >= 3:
            hm = float(np.median(g_d[grp][-30:]))
        else:
            hm = gmed
        sla_h = {4: 4.0, 3: 8.0, 2: 16.0, 1: 24.0}[prio]
        hr = r.opened_dt.hour
        biz = 1.0 if (r.opened_dt.weekday() < 5 and 9 <= hr < 18) else 0.0
        isc = 1.0 if str(r.CI_Cat) in ("application", "subapplication") else 0.0
        Xi.append([prio, urg, imp, isc, min(tq, 12), 0.0, hm, 1.0, sla_h, hr, biz, 80.0, 1.0 if prio >= 3 else 0.0, 0.0])
        yi.append(r.dur_hours)
        all_d.append(r.dur_hours)
        gp_d.setdefault((grp, prio), []).append(r.dur_hours)
        g_d.setdefault(grp, []).append(r.dur_hours)
        if len(Xi) >= 1000:
            break

    ext_X_parts.append(np.array(Xi, dtype=np.float32))
    ext_y_parts.append(np.array(yi, dtype=np.float32))
    ext_counts["itsm"] = len(Xi)

if os.path.exists(UCI_PATH):
    df_u = pd.read_csv(UCI_PATH, low_memory=False)
    first = df_u.sort_values("sys_mod_count").groupby("number", as_index=False).first()
    last = df_u.sort_values("sys_mod_count").groupby("number", as_index=False).last()
    m = first[["number", "opened_at", "priority", "urgency", "impact", "category", "contact_type"]].merge(
        last[["number", "resolved_at", "assignment_group", "assigned_to", "reassignment_count"]],
        on="number",
    )
    m["opened_dt"] = pd.to_datetime(m["opened_at"], dayfirst=True, errors="coerce")
    m["resolved_dt"] = pd.to_datetime(m["resolved_at"], dayfirst=True, errors="coerce")
    m["dur_hours"] = (m["resolved_dt"] - m["opened_dt"]).dt.total_seconds() / 3600.0
    uci_clean = m[(m["reassignment_count"] == 0) & (m["dur_hours"] >= 0.35) & (m["dur_hours"] <= 12.0)].sort_values("opened_dt")

    g_act, e_act, gp_d, g_d, c_d, e_d, all_d = {}, {}, {}, {}, {}, {}, []
    Xu, yu = [], []
    for r in uci_clean.itertuples():
        st, ct = r.opened_dt.timestamp(), r.resolved_dt.timestamp()
        grp, cat = str(r.assignment_group), str(r.category)
        emp = "" if str(r.assigned_to) == "?" else str(r.assigned_to)
        p_s = str(r.priority)
        prio = 4 if "1" in p_s else (3 if "2" in p_s else (2 if "3" in p_s else 1))
        u_s = str(r.urgency)
        urg = 4 if "1" in u_s else (2 if "2" in u_s else 1)
        i_s = str(r.impact)
        imp = 4 if "1" in i_s else (2 if "2" in i_s else 1)

        gh = g_act.setdefault(grp, [])
        while gh and gh[0] <= st:
            heapq.heappop(gh)
        tq = len(gh)
        heapq.heappush(gh, ct)

        eq = 0
        if emp:
            eh = e_act.setdefault(emp, [])
            while eh and eh[0] <= st:
                heapq.heappop(eh)
            eq = len(eh)
            heapq.heappush(eh, ct)

        gmed = float(np.median(all_d[-500:])) if all_d else 0.9
        if len(gp_d.get((grp, prio), [])) >= 3:
            hm = float(np.median(gp_d[(grp, prio)][-30:]))
        elif len(g_d.get(grp, [])) >= 3:
            hm = float(np.median(g_d[grp][-30:]))
        elif len(c_d.get(cat, [])) >= 3:
            hm = float(np.median(c_d[cat][-30:]))
        else:
            hm = gmed
        el = e_d.get(emp, []) if emp else []
        sf = float(np.clip(np.median(el[-20:]) / gmed, 0.65, 1.50)) if (len(el) >= 3 and gmed > 0) else 1.0
        sla_h = {4: 4.0, 3: 8.0, 2: 16.0, 1: 24.0}[prio]
        hr = r.opened_dt.hour
        biz = 1.0 if (r.opened_dt.weekday() < 5 and 9 <= hr < 18) else 0.0
        isc = 1.0 if str(r.contact_type) == "Phone" else 0.0
        Xu.append([prio, urg, imp, isc, min(tq, 12), min(eq, 8), hm, sf, sla_h, hr, biz, 70.0, 1.0 if prio >= 3 else 0.0, 0.0])
        yu.append(r.dur_hours)
        all_d.append(r.dur_hours)
        gp_d.setdefault((grp, prio), []).append(r.dur_hours)
        g_d.setdefault(grp, []).append(r.dur_hours)
        c_d.setdefault(cat, []).append(r.dur_hours)
        if emp:
            e_d.setdefault(emp, []).append(r.dur_hours)
        if len(Xu) >= 250:
            break

    ext_X_parts.append(np.array(Xu, dtype=np.float32))
    ext_y_parts.append(np.array(yu, dtype=np.float32))
    ext_counts["uci"] = len(Xu)

if ext_X_parts:
    X_ext = np.vstack(ext_X_parts)
    y_ext = np.concatenate(ext_y_parts)
    X_train_full = np.vstack([Xl_train, X_ext])
    y_train_full = np.concatenate([yl_train, y_ext])
    sample_weights = np.concatenate([np.full(len(Xl_train), 25.0), np.ones(len(X_ext))])
else:
    X_train_full = Xl_train
    y_train_full = yl_train
    sample_weights = np.ones(len(Xl_train))

y_train_log = np.log1p(y_train_full)

point_model = GradientBoostingRegressor(
    loss="huber", alpha=0.85, n_estimators=220, max_depth=4, learning_rate=0.04,
    min_samples_leaf=4, subsample=0.85, random_state=42
)
lower_model = GradientBoostingRegressor(
    loss="quantile", alpha=0.10, n_estimators=120, max_depth=3, learning_rate=0.05, random_state=42
)
upper_model = GradientBoostingRegressor(
    loss="quantile", alpha=0.90, n_estimators=120, max_depth=3, learning_rate=0.05, random_state=42
)

point_model.fit(X_train_full, y_train_log, sample_weight=sample_weights)
lower_model.fit(X_train_full, y_train_log, sample_weight=sample_weights)
upper_model.fit(X_train_full, y_train_log, sample_weight=sample_weights)

y_pred = np.maximum(0.25, np.expm1(point_model.predict(Xl_test)))
mae = float(mean_absolute_error(yl_test, y_pred))
rmse = float(math.sqrt(mean_squared_error(yl_test, y_pred)))
mape = float(np.mean(np.abs((yl_test - y_pred) / np.maximum(0.25, yl_test))) * 100.0)
r2 = float(r2_score(yl_test, y_pred))
residual_std = float(np.std(yl_test - y_pred))

pred_breach = y_pred >= (0.70 * sl_test)
actual_breach = yl_test >= (0.70 * sl_test)
tp = int(np.sum(pred_breach & actual_breach))
fp = int(np.sum(pred_breach & ~actual_breach))
sla_precision = float(tp / (tp + fp)) if (tp + fp) > 0 else 1.0

print(
    f"Unified Regression + Russian Embeddings v3.0 (train={len(X_train_full)}, test={len(Xl_test)}) -> "
    f"MAE: {mae:.3f}h, RMSE: {rmse:.3f}h, MAPE: {mape:.1f}%, R2: {r2:.3f}, SLA Precision: {sla_precision:.3f}"
)


def export_gbdt(model: GradientBoostingRegressor) -> dict:
    init_val = float(model.init_.predict(np.zeros((1, X_train_full.shape[1]), dtype=np.float32))[0])
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
        "output_transform": "expm1",
        "trees": trees,
    }


baseline_values = [round(float(v), 4) for v in np.median(Xl_train, axis=0)]
feat_importances = {
    name: round(float(imp), 4)
    for name, imp in zip(REGRESSION_FEATURES, point_model.feature_importances_)
}

reg_json = {
    "model_name": "unified_regression_rubert_v3",
    "model_version": "gbdt_rubert_v3.0",
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
        "train_samples": int(len(X_train_full)),
        "test_samples": int(len(Xl_test)),
        "total_closed_samples": int(len(Xl) + ext_counts["itsm"] + ext_counts["uci"]),
    },
    "point_ensemble": export_gbdt(point_model),
    "lower_ensemble": export_gbdt(lower_model),
    "upper_ensemble": export_gbdt(upper_model),
}

with open(os.path.join(OUTPUT_DIR, "time_regression_model.json"), "w", encoding="utf-8") as f:
    json.dump(reg_json, f, ensure_ascii=False, indent=2)
print("Exported time_regression_model.json (gbdt_rubert_v3.0)")

ru_emb_json = {
    "teacher_model": MODEL_ID,
    "dim": 64,
    "vocab": {tok: idx for idx, (tok, _) in enumerate(keep_tokens)},
    "scales": [round(float(s), 6) for s in scales.tolist()],
    "vectors_b64": base64.b64encode(quantized.tobytes()).decode("ascii"),
    "proj_weight": [[round(float(x), 6) for x in row] for row in proj.weight.detach().cpu().numpy()],
    "proj_bias": [round(float(x), 6) for x in proj.bias.detach().cpu().numpy()],
    "complexity_anchor": [round(float(x), 6) for x in complexity_anchor.tolist()],
    "teams": unique_teams,
    "team_centroids": team_centroids,
}

with open(os.path.join(OUTPUT_DIR, "ru_embeddings.json"), "w", encoding="utf-8") as f:
    json.dump(ru_emb_json, f, ensure_ascii=False)
print(f"Exported ru_embeddings.json (tokens={len(keep_tokens)}, size={os.path.getsize(os.path.join(OUTPUT_DIR, 'ru_embeddings.json'))//1024} KB)")

onnx_reg = convert_sklearn(
    point_model, initial_types=[("float_input", FloatTensorType([None, X_train_full.shape[1]]))]
)
with open(os.path.join(OUTPUT_DIR, "time_regression.onnx"), "wb") as f:
    f.write(onnx_reg.SerializeToString())
print("Exported time_regression.onnx")
