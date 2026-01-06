import "server-only";
import { Pool } from "pg";

// Keep a single pool across hot reloads in dev
declare global {
  // eslint-disable-next-line no-var
  var __crowdaudit_pg_pool: Pool | undefined;
}

export function getDb(): Pool {
  if (!process.env.DATABASE_URL) {
    throw new Error("DATABASE_URL is not set");
  }

  if (!globalThis.__crowdaudit_pg_pool) {
    globalThis.__crowdaudit_pg_pool = new Pool({
      connectionString: process.env.DATABASE_URL,
      // reasonable defaults; tune later
      max: 10,
      idleTimeoutMillis: 30_000,
    });
  }

  return globalThis.__crowdaudit_pg_pool;
}
