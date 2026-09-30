import json
import sqlite3

from contract import DecisionError, VERSION, canonical, digest


class State:
    def __init__(self, path):
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path)
        self.db.execute("CREATE TABLE IF NOT EXISTS records (kind TEXT, id TEXT, body TEXT, PRIMARY KEY(kind,id))")
        self.db.execute("CREATE TABLE IF NOT EXISTS packets (key TEXT PRIMARY KEY, body TEXT)")

    def close(self):
        self.db.close()

    def ingest(self, evidence, memory):
        with self.db:
            for kind, rows in (("evidence", evidence), ("memory", memory)):
                for row in rows:
                    body = canonical(row)
                    old = self.db.execute("SELECT body FROM records WHERE kind=? AND id=?", (kind, row["id"])).fetchone()
                    if old and old[0] != body:
                        raise DecisionError("memory.identity_conflict", row["id"])
                    self.db.execute("INSERT OR IGNORE INTO records VALUES (?,?,?)", (kind, row["id"], body))

    def merge(self, request):
        result = dict(request)
        for kind in ("evidence", "memory"):
            rows = {row["id"]: row for row in (json.loads(item[0]) for item in self.db.execute(
                "SELECT body FROM records WHERE kind=? ORDER BY id", (kind,)))}
            for incoming in request[kind]:
                if incoming["id"] in rows and canonical(rows[incoming["id"]]) != canonical(incoming):
                    raise DecisionError("memory.identity_conflict", incoming["id"])
                rows[incoming["id"]] = incoming
            result[kind] = [rows[identity] for identity in sorted(rows)]
        return result

    def cached(self, request):
        key = digest({"engine": VERSION, "request": request})
        row = self.db.execute("SELECT body FROM packets WHERE key=?", (key,)).fetchone()
        return key, json.loads(row[0]) if row else None

    def save(self, key, packet):
        with self.db:
            self.db.execute("INSERT OR REPLACE INTO packets VALUES (?,?)", (key, canonical(packet)))
