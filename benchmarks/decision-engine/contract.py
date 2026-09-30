import hashlib
import json
from datetime import datetime
from decimal import Decimal, InvalidOperation


VERSION = "decision-packet-v1"


class DecisionError(ValueError):
    def __init__(self, code, identity=None):
        self.code = code
        self.identity = identity
        super().__init__(code)


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False)


def digest(value):
    return hashlib.sha256(canonical(value).encode("utf-8")).hexdigest()


def model_json(raw):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise DecisionError("model.json_duplicate_key", key)
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=unique)


def timestamp(value):
    try:
        result = datetime.fromisoformat(value.replace("Z", "+00:00"))
        if result.tzinfo is None:
            raise ValueError()
        return result
    except (ValueError, TypeError, AttributeError):
        raise DecisionError("input.timestamp_invalid") from None


def number(value):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        raise DecisionError("input.number_invalid")
    try:
        result = Decimal(str(value))
        if not result.is_finite():
            raise InvalidOperation()
        return result
    except InvalidOperation:
        raise DecisionError("input.number_invalid") from None


def text(value):
    if not isinstance(value, str) or not value.strip():
        raise DecisionError("input.text_invalid")
    return value


def identities(rows):
    seen = set()
    for row in rows:
        identity = text(row["id"])
        if identity in seen:
            raise DecisionError("input.duplicate_identity", identity)
        seen.add(identity)
    return seen


def validate(request):
    try:
        if type(request["schema_version"]) is not int or request["schema_version"] != 1:
            raise DecisionError("input.schema_invalid")
        text(request["question"])
        timestamp(request["as_of"])
        for key, maximum in (("evidence", 5000), ("memory", 5000), ("options", 100), ("constraints", 100)):
            if not isinstance(request[key], list) or len(request[key]) > maximum:
                raise DecisionError("input.collection_invalid", key)
            identities(request[key])
        if not request["options"]:
            raise DecisionError("input.options_empty")
        if not isinstance(request["required_topics"], list):
            raise DecisionError("input.topics_invalid")
        for topic in request["required_topics"]:
            text(topic)
        for record in request["evidence"] + request["memory"]:
            text(record["text"])
            text(record["topic_id"])
            text(record["source"]["uri"])
            text(record["source"]["version"])
            observed = timestamp(record["observed_at"])
            if record.get("valid_until") and timestamp(record["valid_until"]) <= observed:
                raise DecisionError("input.validity_invalid", record["id"])
            if (not isinstance(record.get("supersedes", []), list)
                    or any(not isinstance(identity, str) for identity in record.get("supersedes", []))):
                raise DecisionError("input.supersession_invalid", record["id"])
        for evidence in request["evidence"]:
            text(evidence["key"])
            if "fact" in evidence:
                text(evidence["fact"]["metric"])
                text(evidence["fact"]["unit"])
                number(evidence["fact"]["value"])
        evidence_ids = identities(request["evidence"])
        for kind in ("evidence", "memory"):
            rows = {r["id"]: r for r in request[kind]}
            def visit(identity, ancestors):
                if identity in ancestors:
                    raise DecisionError("input.supersession_cycle", identity)
                for old in rows[identity].get("supersedes", []):
                    if old not in rows:
                        raise DecisionError("input.supersession_missing", old)
                    if timestamp(rows[old]["observed_at"]) > timestamp(rows[identity]["observed_at"]):
                        raise DecisionError("input.supersession_time_invalid", identity)
                    if kind == "evidence" and rows[old]["key"] != rows[identity]["key"]:
                        raise DecisionError("input.supersession_key_invalid", identity)
                    visit(old, ancestors | {identity})
            for identity in rows:
                visit(identity, set())
        for memory in request["memory"]:
            if (not isinstance(memory["evidence_ids"], list) or not memory["evidence_ids"]
                    or any(not isinstance(identity, str) for identity in memory["evidence_ids"])):
                raise DecisionError("input.memory_support_empty", memory["id"])
        for option in request["options"]:
            text(option["name"])
            if not isinstance(option["metrics"], dict):
                raise DecisionError("input.metrics_invalid", option["id"])
            for metric, references in option["metrics"].items():
                text(metric)
                if (not isinstance(references, list) or not references
                        or any(not isinstance(identity, str) for identity in references)):
                    raise DecisionError("input.metric_support_empty", metric)
        for constraint in request["constraints"]:
            text(constraint["metric"])
            text(constraint["unit"])
            number(constraint["limit"])
            if constraint["op"] not in ("lte", "gte", "eq"):
                raise DecisionError("input.constraint_operator_invalid", constraint["id"])
            if (not isinstance(constraint["evidence_ids"], list) or not constraint["evidence_ids"]
                    or any(not isinstance(identity, str) for identity in constraint["evidence_ids"])
                    or not set(constraint["evidence_ids"]) <= evidence_ids):
                raise DecisionError("input.constraint_support_missing", constraint["id"])
        if not isinstance(request.get("preferences", []), list):
            raise DecisionError("input.preferences_invalid")
        for preference in request.get("preferences", []):
            text(preference["metric"])
            text(preference["unit"])
            if preference["direction"] not in ("min", "max"):
                raise DecisionError("input.preference_invalid")
        try:
            canonical(request)
        except ValueError:
            raise DecisionError("input.json_nonfinite") from None
    except (KeyError, TypeError, RecursionError):
        raise DecisionError("input.structure_invalid") from None
