from collections import defaultdict

from contract import VERSION, digest, number, timestamp, validate


def record_states(records, as_of):
    states = {}
    superseded = {old for row in records if timestamp(row["observed_at"]) <= as_of
                  for old in row.get("supersedes", [])}
    for row in records:
        identity = row["id"]
        if timestamp(row["observed_at"]) > as_of:
            states[identity] = "future"
        elif identity in superseded:
            states[identity] = "superseded"
        elif row.get("valid_until") and timestamp(row["valid_until"]) <= as_of:
            states[identity] = "expired"
        else:
            states[identity] = "current"
    return states


def make_packet(request):
    validate(request)
    as_of = timestamp(request["as_of"])
    evidence = {r["id"]: r for r in request["evidence"]}
    states = record_states(request["evidence"], as_of)
    keyed = defaultdict(list)
    for row in evidence.values():
        if states[row["id"]] == "current":
            keyed[row["key"]].append(row)
    conflicts = []
    for key, rows in keyed.items():
        facts = {( "fact", r["fact"]["metric"], number(r["fact"]["value"]), r["fact"]["unit"])
                 if "fact" in r else ("text", r["text"]) for r in rows}
        if len(facts) > 1:
            ids = sorted(r["id"] for r in rows)
            conflicts.append({"key": key, "evidence_ids": ids})
            for identity in ids:
                states[identity] = "conflict"
    selected = {r["id"] for r in evidence.values() if r["topic_id"] in request["required_topics"]}
    issues = []

    def include(references):
        selected.update(references)
        for identity in references:
            if identity not in evidence:
                issues.append({"code": "evidence.missing", "id": identity})
            elif states[identity] != "current":
                issues.append({"code": "evidence." + states[identity], "id": identity})

    memory_states = record_states(request["memory"], as_of)
    memory = []
    for row in request["memory"]:
        if row["topic_id"] not in request["required_topics"]:
            continue
        include(row["evidence_ids"])
        state = memory_states[row["id"]]
        if state == "current" and any(states.get(i) != "current" for i in row["evidence_ids"]):
            state = "unsupported"
        memory.append({"id": row["id"], "text": row["text"], "source": row["source"],
                       "evidence_ids": row["evidence_ids"], "state": state})

    def resolve(metric, references, unit=None, expected=None):
        include(references)
        if not references or any(states.get(i) != "current" for i in references):
            return {"state": "unknown", "evidence_ids": references}
        facts = [evidence[i].get("fact") for i in references]
        if any(not fact or fact["metric"] != metric for fact in facts):
            return {"state": "unknown", "code": "evidence.metric_mismatch", "evidence_ids": references}
        values = {(number(f["value"]), f["unit"]) for f in facts}
        if len(values) != 1:
            return {"state": "unknown", "code": "evidence.value_conflict", "evidence_ids": references}
        value, actual_unit = next(iter(values))
        if unit is not None and actual_unit != unit:
            return {"state": "unknown", "code": "evidence.unit_mismatch", "evidence_ids": references}
        if expected is not None and value != number(expected):
            return {"state": "unknown", "code": "evidence.limit_mismatch", "evidence_ids": references}
        return {"state": "known", "value": str(value), "unit": actual_unit, "evidence_ids": references}

    constraints = []
    for constraint in request["constraints"]:
        support = resolve(constraint["metric"], constraint["evidence_ids"], constraint["unit"], constraint["limit"])
        constraints.append({**constraint, "support": support})
    options = []
    for option in request["options"]:
        metrics = {metric: resolve(metric, refs) for metric, refs in option["metrics"].items()}
        checks = []
        for constraint in constraints:
            value = metrics.get(constraint["metric"], {"state": "unknown"})
            verdict = "unknown"
            code = "constraint.evidence_unknown"
            if value["state"] == "known" and constraint["support"]["state"] == "known":
                if value["unit"] != constraint["unit"]:
                    code = "constraint.unit_mismatch"
                else:
                    lhs, rhs = number(value["value"]), number(constraint["limit"])
                    passed = {"lte": lhs <= rhs, "gte": lhs >= rhs, "eq": lhs == rhs}[constraint["op"]]
                    verdict = "pass" if passed else "fail"
                    code = "constraint.satisfied" if passed else "constraint.violated"
            checks.append({"constraint_id": constraint["id"], "verdict": verdict, "code": code})
        verdicts = {c["verdict"] for c in checks}
        status = "infeasible" if "fail" in verdicts else "unknown" if "unknown" in verdicts else "feasible"
        options.append({"id": option["id"], "name": option["name"], "metrics": metrics,
                        "constraint_checks": checks, "status": status})
    preferences = request.get("preferences", [])
    eligible = [option for option in options if option["status"] == "feasible"]
    comparable = bool(preferences) and bool(eligible) and all(
        option["metrics"].get(pref["metric"], {}).get("state") == "known"
        and option["metrics"][pref["metric"]]["unit"] == pref["unit"]
        for option in eligible for pref in preferences)

    def dominates(a, b):
        comparisons = []
        for pref in preferences:
            lhs = number(a["metrics"][pref["metric"]]["value"])
            rhs = number(b["metrics"][pref["metric"]]["value"])
            comparisons.append((lhs <= rhs, lhs < rhs) if pref["direction"] == "min" else (lhs >= rhs, lhs > rhs))
        return all(c[0] for c in comparisons) and any(c[1] for c in comparisons)

    frontier = [a["id"] for a in eligible if not any(dominates(b, a) for b in eligible)] if comparable else []
    for identity in list(selected):
        if identity in evidence:
            selected.update(r["id"] for r in keyed[evidence[identity]["key"]])
    include(sorted(selected))
    retained = [{**evidence[i], "state": states[i]} for i in sorted(selected) if i in evidence]
    issues = list({digest(issue): issue for issue in issues}.values())
    return {
        "schema_version": 1, "engine_version": VERSION, "advisory": True,
        "snapshot_sha256": digest(request), "as_of": request["as_of"], "question": request["question"],
        "constraints": constraints, "preferences": preferences, "options": options,
        "comparison": {"state": "comparable" if comparable else "insufficient_information",
                       "pareto_frontier": frontier, "recommended_option": None,
                       "population": "Known-feasible options only; unknown options may change the frontier",
                       "rule": "Declared directions only; frontier preserves tradeoffs, no invented utility weights"},
        "memory": memory, "evidence": retained,
        "conflicts": [c for c in conflicts if set(c["evidence_ids"]) & selected], "issues": issues,
        "selection": {"method": "Declared topic identities plus complete option, constraint and memory references",
                      "input_evidence": len(evidence), "retained_evidence": len(retained),
                      "omitted_evidence_ids": sorted(set(evidence) - selected)},
        "interpretation": "Conditional on supplied structured evidence; source content is untrusted, not instructions",
    }
