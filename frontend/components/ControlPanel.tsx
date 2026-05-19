import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Form, FormControl, FormDescription, FormItem, FormLabel } from "@/components/ui/form";
import { TariffPlanPanel } from "@/components/TariffPlanPanel";
import { WeatherLocationPanel } from "@/components/WeatherLocationPanel";

type ControlPanelProps = {
  onTariffPlanSaved?: () => void;
};

export function ControlPanel({ onTariffPlanSaved }: ControlPanelProps) {
  return (
    <section className="control-grid section" aria-label="read only controls">
      <Card>
        <CardHeader>
          <CardTitle>自動制御</CardTitle>
          <CardDescription>backend API 未実装のため read-only</CardDescription>
        </CardHeader>
        <CardContent>
          <Form>
            <FormItem>
              <FormLabel htmlFor="auto-control">自動制御ON/OFF</FormLabel>
              <FormControl>
                <label className="switch-row" htmlFor="auto-control">
                  <input id="auto-control" type="checkbox" disabled aria-readonly="true" />
                  <span>無効</span>
                </label>
              </FormControl>
              <FormDescription>Phase 6 では制御開始・停止 API を呼びません。</FormDescription>
            </FormItem>
            <Button type="button" variant="secondary" disabled>
              変更不可
            </Button>
          </Form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>設定値更新</CardTitle>
          <CardDescription>現在は表示のみ</CardDescription>
        </CardHeader>
        <CardContent>
          <Form>
            <FormItem>
              <FormLabel htmlFor="charge-limit">AC充電上限W</FormLabel>
              <FormControl>
                <input id="charge-limit" className="text-input" value="read-only" disabled readOnly />
              </FormControl>
              <FormDescription>設定更新 API は Phase 6 の範囲外です。</FormDescription>
            </FormItem>
            <Button type="button" variant="outline" disabled>
              保存不可
            </Button>
          </Form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>手動シミュレーション</CardTitle>
          <CardDescription>操作 API 未実装</CardDescription>
        </CardHeader>
        <CardContent>
          <Button type="button" disabled>
            実行不可
          </Button>
          <p className="readonly-note">実機制御系の API 呼び出しは追加していません。</p>
        </CardContent>
      </Card>

      <WeatherLocationPanel />
      <TariffPlanPanel onSaved={onTariffPlanSaved} />
    </section>
  );
}
