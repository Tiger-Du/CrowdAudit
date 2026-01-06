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
import { Badge } from "@/components/ui/badge";

export const dynamic = "force-dynamic";

type TableRowInfo = {
  table_name: string;
  approx_rows: number | null;
};

export default async function DbHome() {
  await requireAdmin();

  const db = getDb();

  // “Approx rows” from pg_class is fast; exact COUNT(*) can be expensive.
  const { rows } = await db.query<TableRowInfo>(`
    select
      c.relname as table_name,
      c.reltuples::bigint as approx_rows
    from pg_class c
    join pg_namespace n on n.oid = c.relnamespace
    where c.relkind = 'r'
      and n.nspname = 'public'
    order by c.relname asc;
  `);

  return (
    <main className="mx-auto max-w-4xl p-6 space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Database Browser</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-sm text-muted-foreground">
            Showing public schema tables (row counts are approximate).
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Tables</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Table</TableHead>
                <TableHead className="text-right">Approx rows</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((t) => (
                <TableRow key={t.table_name}>
                  <TableCell>
                    <Link
                      className="underline"
                      href={`/admin/db/${encodeURIComponent(t.table_name)}`}
                    >
                      {t.table_name}
                    </Link>
                  </TableCell>
                  <TableCell className="text-right">
                    <Badge variant="secondary">{t.approx_rows ?? 0}</Badge>
                  </TableCell>
                </TableRow>
              ))}
              {rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={2} className="text-muted-foreground">
                    No tables found in public schema.
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
