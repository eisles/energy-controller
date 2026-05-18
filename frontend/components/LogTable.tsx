import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { PowerLog } from "@/lib/types";

export function LogTable({ logs, error }: { logs: PowerLog[]; error: string | null }) {
  return (
    <Card className="section">
      <CardHeader>
        <CardTitle>直近ログ</CardTitle>
        <CardDescription>/api/logs?limit=100</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <div className="table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Measured</TableHead>
                <TableHead>Grid</TableHead>
                <TableHead>Import</TableHead>
                <TableHead>Export</TableHead>
                <TableHead>SOC</TableHead>
                <TableHead>In</TableHead>
                <TableHead>Out</TableHead>
                <TableHead>AC limit</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>Command</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead>Error</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={13} className="empty-cell">
                    ログはまだありません。
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>{watt(log.gridW)}</TableCell>
                    <TableCell>{watt(log.importW)}</TableCell>
                    <TableCell>{watt(log.exportW)}</TableCell>
                    <TableCell>{nullableUnit(log.batterySoc, "%")}</TableCell>
                    <TableCell>{nullableUnit(log.batteryInputW, "W")}</TableCell>
                    <TableCell>{nullableUnit(log.batteryOutputW, "W")}</TableCell>
                    <TableCell>{nullableUnit(log.acChargeLimitW, "W")}</TableCell>
                    <TableCell>{watt(log.targetChargeW)}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{log.mode || "-"}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={log.commandSent ? "warning" : "success"}>{log.commandSent ? "sent" : "not sent"}</Badge>
                    </TableCell>
                    <TableCell className="reason-cell">{log.decisionReason || "-"}</TableCell>
                    <TableCell className="reason-cell">{log.errorMessage || "-"}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}

function watt(value: number) {
  return `${value} W`;
}

function nullableUnit(value: number | null, unit: string) {
  return value === null ? "-" : `${value} ${unit}`;
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
