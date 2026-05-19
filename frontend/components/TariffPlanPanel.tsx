"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Form, FormControl, FormDescription, FormItem, FormLabel } from "@/components/ui/form";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { deleteTariffPlan, fetchTariffPlans, saveTariffPlan } from "@/lib/api";
import type { TariffPlan } from "@/lib/types";

type TariffPlanPanelProps = {
  onSaved?: () => void;
};

const defaultPlanName = "中部電力 Eライフプラン（3時間帯別電灯）";

export function TariffPlanPanel({ onSaved }: TariffPlanPanelProps) {
  const [open, setOpen] = useState(false);
  const [plans, setPlans] = useState<TariffPlan[]>([]);
  const [planName, setPlanName] = useState(defaultPlanName);
  const [dayRate, setDayRate] = useState("34.06");
  const [homeRate, setHomeRate] = useState("26.00");
  const [nightRate, setNightRate] = useState("16.11");
  const [exportRate, setExportRate] = useState("7.00");
  const [timezone, setTimezone] = useState("Asia/Tokyo");
  const [effectiveFrom, setEffectiveFrom] = useState(() => toDatetimeLocal(new Date()));
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [deletingPlanId, setDeletingPlanId] = useState<number | null>(null);
  const [editingPlanId, setEditingPlanId] = useState<number | null>(null);

  const currentPlan = useMemo(() => plans.find((plan) => !plan.effectiveTo) ?? plans[0], [plans]);

  useEffect(() => {
    void loadPlans();
  }, []);

  useEffect(() => {
    if (!currentPlan) {
      return;
    }
    setPlanName(currentPlan.planName);
    setDayRate(String(currentPlan.dayRateYen));
    setHomeRate(String(currentPlan.homeRateYen));
    setNightRate(String(currentPlan.nightRateYen));
    setExportRate(String(currentPlan.exportRateYen));
    setTimezone(currentPlan.timezone);
  }, [currentPlan]);

  async function loadPlans() {
    try {
      const nextPlans = await fetchTariffPlans();
      setPlans(nextPlans);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "tariff plans request failed");
    }
  }

  async function submitPlan() {
    setSaving(true);
    setMessage(null);
    setError(null);
    try {
      const parsedEffectiveFrom = datetimeLocalToISOString(effectiveFrom);
      if (!parsedEffectiveFrom) {
        throw new Error("適用開始日時が不正です");
      }
      const parsedDayRate = parseRate(dayRate, "デイタイム");
      const parsedHomeRate = parseRate(homeRate, "ホームタイム");
      const parsedNightRate = parseRate(nightRate, "ナイトタイム");
      const parsedExportRate = parseRate(exportRate, "売電");
      if (!timezone.trim()) {
        throw new Error("Timezone を入力してください");
      }
      await saveTariffPlan({
        planName,
        dayRateYen: parsedDayRate,
        homeRateYen: parsedHomeRate,
        nightRateYen: parsedNightRate,
        exportRateYen: parsedExportRate,
        timezone: timezone.trim(),
        effectiveFrom: parsedEffectiveFrom
      });
      await loadPlans();
      setMessage("料金プランを保存しました。過去の計算は適用開始日時ごとの単価で再計算されます。");
      setEditingPlanId(null);
      onSaved?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "tariff plan update failed");
    } finally {
      setSaving(false);
    }
  }

  function editPlan(plan: TariffPlan) {
    setPlanName(plan.planName);
    setDayRate(String(plan.dayRateYen));
    setHomeRate(String(plan.homeRateYen));
    setNightRate(String(plan.nightRateYen));
    setExportRate(String(plan.exportRateYen));
    setTimezone(plan.timezone);
    setEffectiveFrom(toDatetimeLocal(new Date(plan.effectiveFrom)));
    setEditingPlanId(plan.id ?? null);
    setMessage("選択した料金プランをフォームへ読み込みました。保存すると同じ適用開始日時の履歴を更新します。");
    setError(null);
  }

  function resetFormToCurrentPlan() {
    if (currentPlan) {
      editPlan(currentPlan);
    } else {
      setPlanName(defaultPlanName);
      setDayRate("34.06");
      setHomeRate("26.00");
      setNightRate("16.11");
      setExportRate("7.00");
      setTimezone("Asia/Tokyo");
      setEffectiveFrom(toDatetimeLocal(new Date()));
      setEditingPlanId(null);
      setMessage(null);
      setError(null);
    }
  }

  async function removePlan(plan: TariffPlan) {
    if (!plan.id) {
      setError("削除対象の料金プランIDがありません。");
      return;
    }
    if (plans.length <= 1) {
      setError("最後の料金プランは削除できません。");
      return;
    }
    const confirmed = window.confirm(`${plan.planName} (${formatDateTime(plan.effectiveFrom)}) を削除します。料金概算は残ったプランで再計算されます。`);
    if (!confirmed) {
      return;
    }
    setDeletingPlanId(plan.id);
    setMessage(null);
    setError(null);
    try {
      await deleteTariffPlan(plan.id);
      await loadPlans();
      if (editingPlanId === plan.id) {
        setEditingPlanId(null);
      }
      setMessage("料金プランを削除しました。過去の料金概算は残ったプランで再計算されます。");
      onSaved?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "tariff plan delete failed");
    } finally {
      setDeletingPlanId(null);
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>料金プラン</CardTitle>
          <CardDescription>料金概算に使う単価履歴</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="readonly-note">{currentPlan ? currentPlan.planName : "未読込"}</p>
          <p className="readonly-note">
            {currentPlan
              ? `デイ ${yenPerKwh(currentPlan.dayRateYen)} / ホーム ${yenPerKwh(currentPlan.homeRateYen)} / ナイト ${yenPerKwh(currentPlan.nightRateYen)} / 売電 ${yenPerKwh(currentPlan.exportRateYen)}`
              : "料金プランを読み込んでいます。"}
          </p>
          <p className="readonly-note">履歴 {plans.length} 件</p>
          <Button type="button" variant="outline" onClick={() => setOpen(true)}>
            料金プランを編集
          </Button>
        </CardContent>
      </Card>

      {open ? (
        <div className="drawer-backdrop" role="presentation">
          <aside className="settings-drawer tariff-plan-drawer" aria-label="tariff plan settings">
            <div className="drawer-header">
              <div>
                <p className="eyebrow">Tariff</p>
                <h2>料金プラン</h2>
              </div>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                閉じる
              </Button>
            </div>

            {error ? <p className="inline-error">{error}</p> : null}
            {message ? <p className="inline-success">{message}</p> : null}
            <Form
              onSubmit={(event) => {
                event.preventDefault();
                void submitPlan();
              }}
            >
              <div className="drawer-section-title">単価</div>
              <div className="tariff-plan-form-grid">
                <FormItem>
                  <FormLabel htmlFor="tariff-plan-name">プラン名</FormLabel>
                  <FormControl>
                    <input id="tariff-plan-name" className="text-input" value={planName} onChange={(event) => setPlanName(event.target.value)} />
                  </FormControl>
                </FormItem>
                <FormItem>
                  <FormLabel htmlFor="tariff-effective-from">適用開始</FormLabel>
                  <FormControl>
                    <input
                      id="tariff-effective-from"
                      className="text-input"
                      type="datetime-local"
                      value={effectiveFrom}
                      onChange={(event) => setEffectiveFrom(event.target.value)}
                    />
                  </FormControl>
                </FormItem>
                <FormItem>
                  <FormLabel htmlFor="tariff-day-rate">デイタイム 円/kWh</FormLabel>
                  <FormControl>
                    <input id="tariff-day-rate" className="text-input" type="number" step="0.01" value={dayRate} onChange={(event) => setDayRate(event.target.value)} />
                  </FormControl>
                </FormItem>
                <FormItem>
                  <FormLabel htmlFor="tariff-home-rate">ホームタイム 円/kWh</FormLabel>
                  <FormControl>
                    <input id="tariff-home-rate" className="text-input" type="number" step="0.01" value={homeRate} onChange={(event) => setHomeRate(event.target.value)} />
                  </FormControl>
                </FormItem>
                <FormItem>
                  <FormLabel htmlFor="tariff-night-rate">ナイトタイム 円/kWh</FormLabel>
                  <FormControl>
                    <input id="tariff-night-rate" className="text-input" type="number" step="0.01" value={nightRate} onChange={(event) => setNightRate(event.target.value)} />
                  </FormControl>
                </FormItem>
                <FormItem>
                  <FormLabel htmlFor="tariff-export-rate">売電 円/kWh</FormLabel>
                  <FormControl>
                    <input id="tariff-export-rate" className="text-input" type="number" step="0.01" value={exportRate} onChange={(event) => setExportRate(event.target.value)} />
                  </FormControl>
                </FormItem>
                <FormItem>
                  <FormLabel htmlFor="tariff-timezone">Timezone</FormLabel>
                  <FormControl>
                    <input id="tariff-timezone" className="text-input" value={timezone} onChange={(event) => setTimezone(event.target.value)} />
                  </FormControl>
                </FormItem>
              </div>
              <FormDescription>
                {editingPlanId ? "編集中の履歴行を同じ適用開始日時で更新します。" : "同じ適用開始日時で保存するとその行を更新します。新しい開始日時を入れると、それ以前のプランは自動的に終了日時が入ります。"}
              </FormDescription>
              <div className="tariff-plan-actions">
                <Button type="submit" disabled={saving}>
                  {saving ? "保存中" : editingPlanId ? "編集中の料金プランを保存" : "料金プランを保存"}
                </Button>
                {editingPlanId ? (
                  <Button type="button" variant="outline" onClick={resetFormToCurrentPlan}>
                    編集を解除
                  </Button>
                ) : null}
              </div>
            </Form>

            <div className="drawer-section-title">履歴</div>
            <div className="table-wrap tariff-plan-table-wrap">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>操作</TableHead>
                    <TableHead>適用開始</TableHead>
                    <TableHead>適用終了</TableHead>
                    <TableHead>プラン</TableHead>
                    <TableHead>デイ</TableHead>
                    <TableHead>ホーム</TableHead>
                    <TableHead>ナイト</TableHead>
                    <TableHead>売電</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {plans.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="empty-cell">
                        料金プランはまだありません。
                      </TableCell>
                    </TableRow>
                  ) : (
                    plans.map((plan) => (
                      <TableRow key={plan.id ?? plan.effectiveFrom}>
                        <TableCell>
                          <div className="tariff-plan-row-actions">
                            <Button type="button" variant="outline" onClick={() => editPlan(plan)}>
                              編集
                            </Button>
                            <Button type="button" variant="outline" onClick={() => void removePlan(plan)} disabled={plans.length <= 1 || deletingPlanId === plan.id}>
                              {deletingPlanId === plan.id ? "削除中" : "削除"}
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell>{formatDateTime(plan.effectiveFrom)}</TableCell>
                        <TableCell>{plan.effectiveTo ? formatDateTime(plan.effectiveTo) : "終了未定"}</TableCell>
                        <TableCell>{plan.planName}</TableCell>
                        <TableCell>{yenPerKwh(plan.dayRateYen)}</TableCell>
                        <TableCell>{yenPerKwh(plan.homeRateYen)}</TableCell>
                        <TableCell>{yenPerKwh(plan.nightRateYen)}</TableCell>
                        <TableCell>{yenPerKwh(plan.exportRateYen)}</TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </aside>
        </div>
      ) : null}
    </>
  );
}

function datetimeLocalToISOString(value: string) {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return date.toISOString();
}

function parseRate(value: string, label: string) {
  if (!value.trim()) {
    throw new Error(`${label}単価を入力してください`);
  }
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0 || parsed > 500) {
    throw new Error(`${label}単価は 0 より大きく 500 以下で入力してください`);
  }
  return parsed;
}

function toDatetimeLocal(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}

function yenPerKwh(value: number) {
  return `${new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value)} 円/kWh`;
}
