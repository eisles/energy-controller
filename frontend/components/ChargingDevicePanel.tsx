"use client";

import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Form, FormControl, FormDescription, FormItem, FormLabel } from "@/components/ui/form";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { deleteChargingDevice, fetchChargingDevices, saveChargingDevice } from "@/lib/api";
import type { ChargingDevice } from "@/lib/types";

const emptyDevice: ChargingDevice = {
  name: "",
  kind: "ecoflow_delta3_plus",
  provider: "ecoflow",
  role: "auxiliary",
  credentialRef: "",
  deviceSn: "",
  deviceType: "DELTA_3",
  statusSource: "ecoflow_private_mqtt",
  enabled: true,
  controlEnabled: false,
  priority: 50,
  minChargeW: 100,
  maxChargeW: 1500,
  chargeStepW: 100,
  capacityWh: 2048,
  targetSoc: 90,
  reserveSoc: 20,
  backupReserveMinSoc: 20,
  backupReserveMaxSoc: 90,
  expectedDaytimeLoadW: 400,
  supportsSocRead: true,
  supportsAcChargeLimit: true,
  supportsOnOff: true,
  notes: ""
};

export function ChargingDevicePanel() {
  const [listOpen, setListOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [devices, setDevices] = useState<ChargingDevice[]>([]);
  const [editing, setEditing] = useState<ChargingDevice>({ ...emptyDevice });
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [deletingDeviceId, setDeletingDeviceId] = useState<number | null>(null);

  const summary = useMemo(() => {
    const enabledCount = devices.filter((device) => device.enabled).length;
    const controlCount = devices.filter((device) => device.enabled && device.controlEnabled).length;
    return { enabledCount, controlCount };
  }, [devices]);

  useEffect(() => {
    void loadDevices();
  }, []);

  async function loadDevices() {
    try {
      const nextDevices = await fetchChargingDevices();
      setDevices(nextDevices);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "charging devices request failed");
    }
  }

  async function submitDevice() {
    setSaving(true);
    setMessage(null);
    setError(null);
    try {
      const payload = normalizeDeviceForSave(editing);
      validateDevice(payload);
      const saved = await saveChargingDevice(payload);
      await loadDevices();
      setEditing(saved);
      setEditOpen(false);
      setMessage("充電機器マスタを保存しました。既存の実機制御条件は変更していません。");
    } catch (err) {
      setError(err instanceof Error ? err.message : "charging device save failed");
    } finally {
      setSaving(false);
    }
  }

  async function removeDevice(device: ChargingDevice) {
    if (!device.id) {
      setError("削除対象のIDがありません。");
      return;
    }
    const confirmed = window.confirm(`${device.name} を削除します。過去ログや制御ログは削除されません。`);
    if (!confirmed) {
      return;
    }
    setDeletingDeviceId(device.id);
    setMessage(null);
    setError(null);
    try {
      await deleteChargingDevice(device.id);
      await loadDevices();
      if (editing.id === device.id) {
        setEditing({ ...emptyDevice });
        setEditOpen(false);
      }
      setMessage("充電機器マスタを削除しました。");
    } catch (err) {
      setError(err instanceof Error ? err.message : "charging device delete failed");
    } finally {
      setDeletingDeviceId(null);
    }
  }

  function editDevice(device: ChargingDevice) {
    setEditing({ ...device });
    setMessage(null);
    setError(null);
    setEditOpen(true);
  }

  function startNewDevice() {
    setEditing({ ...emptyDevice });
    setMessage(null);
    setError(null);
    setEditOpen(true);
  }

  function closeListDrawer() {
    setListOpen(false);
    setEditOpen(false);
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>充電機器マスタ</CardTitle>
          <CardDescription>DELTA Pro 3 / DELTA 3 Plus / 手動補助機器</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="readonly-note">
            登録 {devices.length} 台 / 有効 {summary.enabledCount} 台 / 自動制御候補 {summary.controlCount} 台
          </p>
          <p className="readonly-note">SN はローカルDBのマスタ、認証情報は .env で管理します。token や password は保存しません。</p>
          {error ? <p className="inline-error">{error}</p> : null}
          <Button type="button" variant="outline" onClick={() => setListOpen(true)}>
            充電機器一覧を開く
          </Button>
        </CardContent>
      </Card>

      {listOpen ? (
        <div className="drawer-backdrop" role="presentation">
          <aside className="settings-drawer charging-device-list-drawer" aria-label="charging device list">
            <div className="drawer-header">
              <div>
                <p className="eyebrow">Charging devices</p>
                <h2>充電機器マスタ</h2>
              </div>
              <Button type="button" variant="outline" onClick={closeListDrawer}>
                閉じる
              </Button>
            </div>

            {error ? <p className="inline-error">{error}</p> : null}
            {message ? <p className="inline-success">{message}</p> : null}
            <p className="readonly-note">
              一覧では登録内容の確認だけを行います。追加・編集は行ごとの「編集」または「新規追加」から別ドロワーで開きます。
            </p>
            <div className="charging-device-list-actions">
              <Button type="button" onClick={startNewDevice}>
                新規追加
              </Button>
            </div>

            <div className="table-wrap charging-device-table-wrap">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>優先</TableHead>
                    <TableHead>機器</TableHead>
                    <TableHead>充電範囲</TableHead>
                    <TableHead>想定負荷</TableHead>
                    <TableHead>状態</TableHead>
                    <TableHead>識別</TableHead>
                    <TableHead>取得方式</TableHead>
                    <TableHead>認証参照</TableHead>
                    <TableHead>操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {devices.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={9} className="empty-cell">
                        充電機器マスタがありません。
                      </TableCell>
                    </TableRow>
                  ) : (
                    devices.map((device) => (
                      <TableRow key={device.id ?? device.credentialRef}>
                        <TableCell>{device.priority}</TableCell>
                        <TableCell>
                          <strong>{device.name}</strong>
                          <br />
                          <span className="readonly-note">{deviceKindLabel(device.kind)} / {deviceRoleLabel(device.role)}</span>
                        </TableCell>
                        <TableCell>
                          {device.minChargeW}-{device.maxChargeW} W
                          <br />
                          <span className="readonly-note">{device.chargeStepW} W刻み / {formatCapacity(device.capacityWh)}</span>
                        </TableCell>
                        <TableCell>
                          {device.expectedDaytimeLoadW > 0 ? `${device.expectedDaytimeLoadW} W` : "-"}
                          <br />
                          <span className="readonly-note">日中必要量の推定に使用</span>
                        </TableCell>
                        <TableCell>
                          <Badge variant={device.enabled ? "success" : "secondary"}>{device.enabled ? "有効" : "無効"}</Badge>
                          <Badge className="charging-device-badge" variant={device.controlEnabled ? "warning" : "secondary"}>
                            {device.controlEnabled ? "制御候補" : "制御対象外"}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          {maskDeviceSn(device.deviceSn)}
                          <br />
                          <span className="readonly-note">{device.deviceType || "type未設定"}</span>
                        </TableCell>
                        <TableCell>{statusSourceLabel(device.statusSource)}</TableCell>
                        <TableCell>{device.credentialRef}</TableCell>
                        <TableCell>
                          <div className="tariff-plan-row-actions">
                            <Button type="button" variant="outline" onClick={() => editDevice(device)}>
                              編集
                            </Button>
                            <Button type="button" variant="outline" disabled={deletingDeviceId === device.id} onClick={() => void removeDevice(device)}>
                              削除
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </aside>

          {editOpen ? (
            <div className="drawer-backdrop drawer-backdrop-nested" role="presentation">
              <aside className="settings-drawer charging-device-edit-drawer" aria-label="charging device editor">
                <div className="drawer-header">
                  <div>
                    <p className="eyebrow">{editing.id ? `ID ${editing.id}` : "New device"}</p>
                    <h2>{editing.id ? `${editing.name || "充電機器"}を編集` : "充電機器を追加"}</h2>
                  </div>
                  <Button type="button" variant="outline" onClick={() => setEditOpen(false)}>
                    一覧へ戻る
                  </Button>
                </div>

                {error ? <p className="inline-error">{error}</p> : null}
                <p className="readonly-note">
                  保存しても、この画面から EcoFlow / SwitchBot へ直接送信しません。SN はローカルDBのマスタに保存します。
                </p>

                <Form
                  onSubmit={(event) => {
                    event.preventDefault();
                    void submitDevice();
                  }}
                >
                  <div className="drawer-section-title">機器設定</div>
                  <div className="charging-device-form-grid">
                    <TextField id="charging-device-name" label="表示名" value={editing.name} onChange={(value) => setEditing({ ...editing, name: value })} />
                    <TextField id="charging-device-kind" label="種別" value={editing.kind} onChange={(value) => setEditing({ ...editing, kind: value })} />
                    <TextField id="charging-device-provider" label="Provider" value={editing.provider} onChange={(value) => setEditing({ ...editing, provider: value })} />
                    <TextField id="charging-device-role" label="Role" value={editing.role} onChange={(value) => setEditing({ ...editing, role: value })} />
                    <TextField
                      id="charging-device-credential-ref"
                      label="認証参照名"
                      value={editing.credentialRef}
                      onChange={(value) => setEditing({ ...editing, credentialRef: value })}
                      description="認証情報の参照名です。token、password は入力しません。"
                    />
                    <TextField
                      id="charging-device-sn"
                      label="シリアル番号"
                      value={editing.deviceSn}
                      onChange={(value) => setEditing({ ...editing, deviceSn: value })}
                      description="EcoFlow実機のSNです。一覧では末尾だけ表示します。"
                    />
                    <TextField
                      id="charging-device-type"
                      label="Device Type"
                      value={editing.deviceType}
                      onChange={(value) => setEditing({ ...editing, deviceType: value })}
                      description="例: DELTA_3 / DELTA_3_PLUS / DELTA_3_MAX_PLUS"
                    />
                    <TextField
                      id="charging-device-status-source"
                      label="取得方式"
                      value={editing.statusSource}
                      onChange={(value) => setEditing({ ...editing, statusSource: value })}
                      description="例: ecoflow_cloud / ecoflow_private_mqtt / switchbot_cloud / manual"
                    />
                    <NumberField id="charging-device-priority" label="優先順位" value={editing.priority} onChange={(value) => setEditing({ ...editing, priority: value })} />
                    <NumberField id="charging-device-min-w" label="最小充電W" value={editing.minChargeW} onChange={(value) => setEditing({ ...editing, minChargeW: value })} />
                    <NumberField id="charging-device-max-w" label="最大充電W" value={editing.maxChargeW} onChange={(value) => setEditing({ ...editing, maxChargeW: value })} />
                    <NumberField id="charging-device-step-w" label="刻みW" value={editing.chargeStepW} onChange={(value) => setEditing({ ...editing, chargeStepW: value })} />
                    <NumberField id="charging-device-capacity" label="容量Wh" value={editing.capacityWh} onChange={(value) => setEditing({ ...editing, capacityWh: value })} />
                    <NumberField
                      id="charging-device-expected-daytime-load-w"
                      label="日中想定負荷W"
                      value={editing.expectedDaytimeLoadW}
                      onChange={(value) => setEditing({ ...editing, expectedDaytimeLoadW: value })}
                    />
                    <NumberField id="charging-device-backup-reserve-min-soc" label="バックアップリザーブ最小%" value={editing.backupReserveMinSoc} onChange={(value) => setEditing({ ...editing, backupReserveMinSoc: value })} />
                    <NumberField id="charging-device-backup-reserve-max-soc" label="バックアップリザーブ最大%" value={editing.backupReserveMaxSoc} onChange={(value) => setEditing({ ...editing, backupReserveMaxSoc: value })} />
                  </div>
                  <div className="charging-device-switch-grid">
                    <CheckboxField label="有効" checked={editing.enabled} onChange={(checked) => setEditing({ ...editing, enabled: checked })} />
                    <CheckboxField label="自動制御候補" checked={editing.controlEnabled} onChange={(checked) => setEditing({ ...editing, controlEnabled: checked })} />
                    <CheckboxField label="SOC読み取り" checked={editing.supportsSocRead} onChange={(checked) => setEditing({ ...editing, supportsSocRead: checked })} />
                    <CheckboxField label="AC上限設定" checked={editing.supportsAcChargeLimit} onChange={(checked) => setEditing({ ...editing, supportsAcChargeLimit: checked })} />
                    <CheckboxField label="ON/OFF制御" checked={editing.supportsOnOff} onChange={(checked) => setEditing({ ...editing, supportsOnOff: checked })} />
                  </div>
                  <FormItem>
                    <FormLabel htmlFor="charging-device-notes">メモ</FormLabel>
                    <FormControl>
                      <textarea id="charging-device-notes" className="text-input" rows={3} value={editing.notes} onChange={(event) => setEditing({ ...editing, notes: event.target.value })} />
                    </FormControl>
                  </FormItem>
                  <div className="drawer-actions">
                    <Button type="submit" disabled={saving}>
                      {saving ? "保存中" : "保存"}
                    </Button>
                    <Button type="button" variant="outline" onClick={() => setEditOpen(false)}>
                      キャンセル
                    </Button>
                  </div>
                </Form>
              </aside>
            </div>
          ) : null}
        </div>
      ) : null}
    </>
  );
}

function TextField({ id, label, value, description, onChange }: { id: string; label: string; value: string; description?: string; onChange: (value: string) => void }) {
  return (
    <FormItem>
      <FormLabel htmlFor={id}>{label}</FormLabel>
      <FormControl>
        <input id={id} className="text-input" value={value} onChange={(event) => onChange(event.target.value)} />
      </FormControl>
      {description ? <FormDescription>{description}</FormDescription> : null}
    </FormItem>
  );
}

function NumberField({ id, label, value, onChange }: { id: string; label: string; value: number; onChange: (value: number) => void }) {
  return (
    <FormItem>
      <FormLabel htmlFor={id}>{label}</FormLabel>
      <FormControl>
        <input id={id} className="text-input" type="number" value={value} onChange={(event) => onChange(Number(event.target.value))} />
      </FormControl>
    </FormItem>
  );
}

function CheckboxField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="switch-row">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span>{label}</span>
    </label>
  );
}

function normalizeDeviceForSave(device: ChargingDevice): ChargingDevice {
  const backupReserveMinSoc = device.backupReserveMinSoc === 0 ? clampSoc(device.reserveSoc) : device.backupReserveMinSoc;
  const backupReserveMaxSoc = device.backupReserveMaxSoc === 0 ? clampSoc(device.targetSoc) : device.backupReserveMaxSoc;
  return {
    ...device,
    name: device.name.trim(),
    kind: device.kind.trim(),
    provider: device.provider.trim(),
    role: device.role.trim(),
    credentialRef: device.credentialRef.trim(),
    deviceSn: device.deviceSn.trim(),
    deviceType: defaultDeviceType(device.kind.trim(), device.deviceType.trim()),
    statusSource: defaultStatusSource(device.kind.trim(), device.statusSource.trim()),
    targetSoc: backupReserveMaxSoc,
    reserveSoc: backupReserveMinSoc,
    backupReserveMinSoc,
    backupReserveMaxSoc,
    expectedDaytimeLoadW: Math.max(0, Math.round(device.expectedDaytimeLoadW || 0)),
    notes: device.notes.trim()
  };
}

function validateDevice(device: ChargingDevice) {
  if (!device.name || !device.kind || !device.provider || !device.role || !device.credentialRef) {
    throw new Error("表示名、種別、Provider、Role、認証参照名は必須です。");
  }
  if (device.priority < 1 || device.minChargeW < 0 || device.maxChargeW < device.minChargeW || device.chargeStepW < 1 || device.capacityWh < 0) {
    throw new Error("優先順位または充電W範囲が不正です。");
  }
  if (device.expectedDaytimeLoadW < 0) {
    throw new Error("日中想定負荷Wは0以上で入力してください。");
  }
  if (device.targetSoc < 0 || device.targetSoc > 100 || device.reserveSoc < 0 || device.reserveSoc > 100) {
    throw new Error("SOCは0-100の範囲で入力してください。");
  }
  if (device.backupReserveMinSoc < 5 || device.backupReserveMinSoc > 100 || device.backupReserveMaxSoc < 5 || device.backupReserveMaxSoc > 100) {
    throw new Error("バックアップリザーブ範囲は5-100の範囲で入力してください。");
  }
  if (device.backupReserveMaxSoc < device.backupReserveMinSoc) {
    throw new Error("バックアップリザーブ最大%は最小%以上で入力してください。");
  }
  if (/\s|[\x00-\x1f\x7f]/.test(device.deviceSn)) {
    throw new Error("シリアル番号に空白や制御文字は使えません。");
  }
  if (device.provider === "ecoflow" && !device.deviceType) {
    throw new Error("EcoFlow機器はDevice Typeが必要です。");
  }
  if (!validDeviceTypeForKind(device.kind, device.deviceType)) {
    throw new Error("種別とDevice Typeの組み合わせが不正です。");
  }
  if (!validStatusSourceForKind(device.kind, device.statusSource)) {
    throw new Error("種別と取得方式の組み合わせが不正です。");
  }
}

function clampSoc(value: number) {
  if (value < 5) {
    return 5;
  }
  if (value > 100) {
    return 100;
  }
  return value;
}

function defaultDeviceType(kind: string, value: string) {
  if (value) {
    return value;
  }
  if (kind === "ecoflow_delta_pro3") {
    return "DELTA_PRO3";
  }
  if (kind === "ecoflow_delta3_plus") {
    return "DELTA_3";
  }
  return "";
}

function maskDeviceSn(value: string) {
  if (!value) {
    return "SN未設定";
  }
  if (value.length <= 4) {
    return `***${value}`;
  }
  return `***${value.slice(-4)}`;
}

function defaultStatusSource(kind: string, value: string) {
  if (value) {
    return value;
  }
  if (kind === "ecoflow_delta_pro3") {
    return "ecoflow_cloud";
  }
  if (kind === "ecoflow_delta3_plus") {
    return "ecoflow_private_mqtt";
  }
  if (kind === "switchbot_plug") {
    return "switchbot_cloud";
  }
  return "manual";
}

function validDeviceTypeForKind(kind: string, deviceType: string) {
  if (kind === "ecoflow_delta_pro3") {
    return deviceType === "DELTA_PRO3";
  }
  if (kind === "ecoflow_delta3_plus") {
    return ["DELTA_3", "DELTA_3_PLUS", "DELTA_3_1500", "DELTA_3_MAX_PLUS"].includes(deviceType);
  }
  return deviceType === "";
}

function validStatusSourceForKind(kind: string, statusSource: string) {
  if (kind === "ecoflow_delta_pro3") {
    return statusSource === "ecoflow_cloud";
  }
  if (kind === "ecoflow_delta3_plus") {
    return statusSource === "ecoflow_private_mqtt";
  }
  if (kind === "switchbot_plug") {
    return statusSource === "switchbot_cloud";
  }
  return statusSource === "manual";
}

function deviceKindLabel(value: string) {
  const labels: Record<string, string> = {
    ecoflow_delta_pro3: "DELTA Pro 3",
    ecoflow_delta3_plus: "DELTA 3 Plus",
    switchbot_plug: "SwitchBot Plug",
    manual: "手動"
  };
  return labels[value] || value;
}

function statusSourceLabel(value: string) {
  const labels: Record<string, string> = {
    ecoflow_cloud: "EcoFlow Cloud API",
    ecoflow_private_mqtt: "EcoFlow private MQTT",
    switchbot_cloud: "SwitchBot Cloud API",
    manual: "手動"
  };
  return labels[value] || value || "未設定";
}

function deviceRoleLabel(value: string) {
  const labels: Record<string, string> = {
    primary: "主バッテリー",
    auxiliary: "補助バッテリー",
    manual_auxiliary: "手動補助"
  };
  return labels[value] || value;
}

function formatCapacity(value: number) {
  if (value <= 0) {
    return "容量未設定";
  }
  return `${new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 1 }).format(value / 1000)} kWh`;
}
