import "server-only";
import Link from "next/link";
import { getDb } from "@/lib/db";
import { requireAdmin } from "@/lib/admin-auth";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";

export const dynamic = "force-dynamic";
export const revalidate = 0;

function isSafeIdentifier(name: string) {
  // allow: letters, numbers, underscore only
  return /^[a-zA-Z0-9_]+$/.test(name);
}

type SearchParams = { limit?: string; offset?: string };

export default async function TablePage(props: {
  params: Promise<{ table: string }>;
  searchParams?: Promise<SearchParams>;
}) {
  await requireAdmin();

  const { table: tableParam } = await props.params;
  const sp = props.searchParams ? await props.searchParams : {};

  const table = decodeURIComponent(tableParam);

  if (!isSafeIdentifier(table)) {
    return (
      <main className="mx-auto max-w-4xl p-6">
        <Card>
          <CardHeader>
            <CardTitle>Invalid table name</CardTitle>
          </CardHeader>
          <CardContent>
            <Link className="underline" href="/admin/db">
              Back
            </Link>
          </CardContent>
        </Card>
      </main>
    );
  }

  const limit = Math.min(Math.max(Number(sp.limit ?? 100) || 100, 1), 500);
  const offset = Math.max(Number(sp.offset ?? 0) || 0, 0);

  const db = getDb();

  // Get columns (public schema only)
  const colsRes = await db.query<{ column_name: string }>(
    `
      select column_name
      from information_schema.columns
      where table_schema='public' and table_name=$1
      order by ordinal_position asc;
    `,
    [table],
  );

  const columns = colsRes.rows.map((r) => r.column_name);

  if (columns.length === 0) {
    return (
      <main className="mx-auto max-w-4xl p-6 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Table not found</CardTitle>
          </CardHeader>
          <CardContent>
            <Link className="underline" href="/admin/db">
              Back to tables
            </Link>
          </CardContent>
        </Card>
      </main>
    );
  }

  // Identifiers cannot be parameterized in pg, so we validate and then interpolate.
  const dataRes = await db.query(
    `select * from public.${table} order by 1 asc limit $1 offset $2`,
    [limit, offset],
  );

  const rows = dataRes.rows as Record<string, unknown>[];

  const prevOffset = Math.max(offset - limit, 0);
  const nextOffset = offset + limit;

  return (
    <main className="mx-auto max-w-6xl p-6 space-y-4">
      <div className="flex items-center gap-3">
        <Link className="underline" href="/admin/db">
          ← Back
        </Link>
        <h1 className="text-xl font-semibold">{table}</h1>

        <div className="ml-auto flex items-center gap-2">
          <Button asChild variant="secondary">
            <Link
              href={`/admin/db/${encodeURIComponent(table)}?limit=${limit}&offset=${prevOffset}`}
            >
              Prev
            </Link>
          </Button>
          <Button asChild variant="secondary">
            <Link
              href={`/admin/db/${encodeURIComponent(table)}?limit=${limit}&offset=${nextOffset}`}
            >
              Next
            </Link>
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>
            Rows (limit {limit}, offset {offset})
          </CardTitle>
        </CardHeader>
        <CardContent className="overflow-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {columns.map((c) => (
                  <TableHead key={c}>{c}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r, i) => (
                <TableRow key={i}>
                  {columns.map((c) => (
                    <TableCell key={c} className="align-top">
                      <Cell v={r[c]} />
                    </TableCell>
                  ))}
                </TableRow>
              ))}
              {rows.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={columns.length}
                    className="text-muted-foreground"
                  >
                    No rows.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </main>
  );
}

function Cell({ v }: { v: unknown }) {
  if (v === null || v === undefined)
    return <span className="text-muted-foreground">null</span>;
  if (typeof v === "object")
    return (
      <pre className="text-xs whitespace-pre-wrap">
        {JSON.stringify(v, null, 2)}
      </pre>
    );
  return <span className="text-sm whitespace-pre-wrap">{String(v)}</span>;
}
