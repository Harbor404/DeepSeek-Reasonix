// @ts-expect-error Node 22+ provides node:sqlite; Worker production code does not import it.
import type { DatabaseSync } from "node:sqlite";

export function d1(db: DatabaseSync): D1Database {
  const stmt = (sql: string, args: unknown[] = []): any => ({
    sql,
    args,
    bind: (...a: unknown[]) => stmt(sql, a),
    first: async () => db.prepare(sql).get(...args) ?? null,
    all: async () => ({ results: db.prepare(sql).all(...args) }),
    run: async () => {
      const r = db.prepare(sql).run(...args);
      return { meta: { changes: Number(r.changes), last_row_id: Number(r.lastInsertRowid) } };
    },
  });
  const batch = async (list: { sql: string; args: unknown[] }[]) => {
    db.exec("BEGIN");
    try {
      const out = list.map((st) => {
        const r = db.prepare(st.sql).run(...st.args);
        return { meta: { changes: Number(r.changes), last_row_id: Number(r.lastInsertRowid) } };
      });
      db.exec("COMMIT");
      return out;
    } catch (err) {
      db.exec("ROLLBACK");
      throw err;
    }
  };
  return { prepare: (sql: string) => stmt(sql), batch } as unknown as D1Database;
}

export function fakeR2() {
  const objects = new Map<string, { body: Uint8Array; contentType: string }>();
  const bucket = {
    put: async (k: string, body: Uint8Array, o: { httpMetadata: { contentType: string } }) => void objects.set(k, { body, contentType: o.httpMetadata.contentType }),
    delete: async (ks: string | string[]) => void [ks].flat().forEach((k) => objects.delete(k)),
    get: async (k: string) => {
      const o = objects.get(k);
      return o ? { body: o.body, httpMetadata: { contentType: o.contentType } } : null;
    },
  };
  return { bucket: bucket as unknown as R2Bucket, objects };
}
