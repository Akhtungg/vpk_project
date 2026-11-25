import json
import os
import random
from datetime import datetime, timedelta, timezone

DATA_PATH = os.path.join(os.path.dirname(__file__), "..", "data", "character_problems.json")


def get_base_mttr(group_name: str) -> float:
    g = group_name.lower()
    if any(k in g for k in ["san", "vmware", "dc-net", "цод", "приморье"]):
        return 175.0
    if any(k in g for k in ["streaming", "media", "billing", "foris", "тарификация"]):
        return 85.0
    if any(k in g for k in ["active directory", "vpn", "devops"]):
        return 65.0
    if "autoclosed" in g:
        return 38.0
    return 95.0


def employee_speed_factor(login: str) -> float:
    # Deterministic speed factor per specialist (0.78 .. 1.28)
    if not login:
        return 1.0
    h = sum(ord(c) * (i + 1) for i, c in enumerate(login))
    return 0.78 + (h % 51) / 100.0


def main():
    random.seed(42)
    with open(DATA_PATH, encoding="utf-8") as f:
        data = json.load(f)

    # Keep only original 44 if re-run
    base_records = [r for r in data if not r["id"].startswith("INC00002190")]
    if len(base_records) == 0:
        raise RuntimeError("No base records found")

    # Collect valid templates (non-coordinator groups for historical resolved tickets, plus some coordinator-routed)
    templates = [r for r in base_records if "координатор" not in r["assignedGroupName"].lower()]
    employees = list({
        (r.get("assignedEmployee", ""), r.get("assignedEmployeeLogin", ""))
        for r in base_records
        if r.get("assignedEmployee")
    })

    # Generate 206 historical closed tickets spread across 2026-11-05 .. 2026-11-19 (250 total)
    # Realistic density produces active queues of 0..5 tickets
    start_dt = datetime(2026, 11, 5, 6, 0, tzinfo=timezone.utc)
    end_dt = datetime(2026, 11, 19, 18, 0, tzinfo=timezone.utc)
    total_span_minutes = int((end_dt - start_dt).total_seconds() // 60)

    offsets = sorted(random.randint(0, total_span_minutes) for _ in range(206))

    priorities = [
        (1, "Низкий", 1, "Низкая", 1, "Низкое", 24),
        (2, "Средний", 2, "Средняя", 2, "Среднее", 16),
        (3, "Высокий", 3, "Высокая", 3, "Высокое", 8),
        (4, "Наивысший", 4, "Наивысшая", 4, "Наивысшее", 4),
    ]

    historical = []
    # Track active intervals [(submit_dt, completion_dt, group, emp_login)]
    intervals = []

    for idx, off in enumerate(offsets, start=1):
        tpl = random.choice(templates)
        emp_name, emp_login = random.choice(employees)
        prio_id, prio_name, urg_id, urg_name, imp_id, imp_name, sla_hours = random.choices(
            priorities, weights=[0.30, 0.40, 0.20, 0.10], k=1
        )[0]

        submit_dt = start_dt + timedelta(minutes=off)
        deadline_dt = submit_dt + timedelta(hours=sla_hours)

        group_name = tpl["assignedGroupName"]
        # Count concurrent open tickets at submit_dt for group & employee
        team_q = sum(1 for s, c, g, _ in intervals if g == group_name and s <= submit_dt < c)
        emp_q = sum(1 for s, c, _, e in intervals if e == emp_login and s <= submit_dt < c)

        base_mttr = get_base_mttr(group_name)
        sev_mult = 1.0 + 0.12 * prio_id + 0.04 * urg_id
        queue_mult = 1.0 + 0.22 * team_q + 0.15 * emp_q
        speed_mult = employee_speed_factor(emp_login)

        hour = submit_dt.hour
        is_business = 1 if (submit_dt.weekday() < 5 and 9 <= hour < 18) else 0
        hours_mult = 1.0 if is_business else 1.22

        header = tpl["header"]
        desc = tpl["description"]
        if prio_id == 4 and "отказ" not in header.lower():
            header = f"Критический инцидент: {header}"
            desc = f"Массовый сбой и отказ сервиса. {desc}"

        kw_mult = 1.15 if any(k in (header + " " + desc).lower() for k in ["отказ", "авария", "массовый", "критический"]) else 1.0
        noise = random.uniform(0.88, 1.12)

        duration_min = max(20, int(round(base_mttr * sev_mult * queue_mult * speed_mult * hours_mult * kw_mult * noise)))
        completion_dt = submit_dt + timedelta(minutes=duration_min)
        intervals.append((submit_dt, completion_dt, group_name, emp_login))

        is_resolved_state = idx % 3 == 0
        state = "Завершен" if is_resolved_state else "Закрыт"
        state_enum = "Completed" if is_resolved_state else "Closed"
        state_id = 10 if is_resolved_state else 7

        rec = {
            "sysId": f"1794000000000{idx:05d}",
            "id": f"INC00002190{idx:04d}",
            "state": state,
            "stateEnum": state_enum,
            "stateId": state_id,
            "stateReason": "Решено",
            "stateReasonId": "400",
            "submitDate": submit_dt.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "deadline": deadline_dt.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "completionDate": completion_dt.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "lastAssignmentToTeamDateTime": submit_dt.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "header": header,
            "description": desc,
            "shortDescription": tpl.get("shortDescription", header),
            "assignedGroupName": group_name,
            "assignedEmployee": emp_name,
            "assignedEmployeeLogin": emp_login,
            "initiatorName": tpl.get("initiatorName", "Система мониторинга"),
            "initiatorLogin": tpl.get("initiatorLogin", "monitoring"),
            "isEffect": False,
            "isReason": False,
            "isDouble": False,
            "isMaster": False,
            "incType": "МИ",
            "serviceTypeId": tpl.get("serviceTypeId", 30),
            "serviceType": tpl.get("serviceType", "Технологический"),
            "priorityId": prio_id,
            "priority": prio_name,
            "urgencyId": urg_id,
            "urgency": urg_name,
            "impactId": imp_id,
            "impact": imp_name,
            "waitingExitDate": "0001-01-01T00:00:00",
            "isRootIncident": True,
            "productName": tpl.get("productName", ""),
            "productId": tpl.get("productId", ""),
            "productMasterId": tpl.get("productMasterId", ""),
            "productUrl": None,
            "service": tpl.get("service", ""),
            "serviceId": tpl.get("serviceId", ""),
            "scenarioName": tpl.get("scenarioName", ""),
            "isNeedUp": False,
            "eliminationType": None,
            "eliminationTypeId": None,
            "receivingChannel": tpl.get("receivingChannel", "Мониторинг"),
            "solution": tpl.get("solution", "Инцидент устранен, работоспособность восстановлена."),
        }
        historical.append(rec)

    combined = base_records + historical
    with open(DATA_PATH, "w", encoding="utf-8") as f:
        json.dump(combined, f, ensure_ascii=False, indent=2)

    print(f"Dataset updated: {len(base_records)} base + {len(historical)} historical = {len(combined)} total tickets.")


if __name__ == "__main__":
    main()
