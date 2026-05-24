const strategyStateLabels: Record<string, string> = {
  IDLE: "待機",
  READY: "実行候補",
  CHARGING: "充電調整中",
  RECOVERING: "買電回復中",
  PASSTHROUGH: "パススルー調整",
  HOLD: "保留",
  WAIT_PRO3: "DELTA Pro 3優先待ち",
  DISABLED: "無効",
  UNAVAILABLE: "取得不可",
  FULL: "満充電付近",
  DAYTIME_OBSERVE: "日中観測",
  NIGHT_PLAN_READY: "深夜計画準備",
  NIGHT_CHARGE_WINDOW: "深夜充電中",
  NIGHT_RECOVER: "通常復帰"
};

const commandKindLabels: Record<string, string> = {
  none: "操作なし",
  ac_charge: "AC充電上限",
  backup_reserve: "バックアップリザーブ",
  energy_modes: "動作モード",
  combined: "複合操作"
};

const exactGuardReasonLabels: Record<string, string> = {
  "no command candidate": "送信候補なし",
  "mock mode, EcoFlow write disabled": "モックモードのため送信しません",
  "simulation mode, EcoFlow write disabled": "シミュレーションのため送信しません",
  "ENABLE_REAL_CONTROL=false, EcoFlow write disabled": "実制御が無効のため送信しません",
  "auto control disabled, EcoFlow write disabled": "自動制御が無効のため送信しません",
  "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND": "確認文字列が未設定のため送信しません",
  "real control trial window inactive": "実制御の試験期限外のため送信しません",
  "ECOFLOW_DELTA3_READ_ENABLED=false": "DELTA 3 Plus読取が無効です",
  "ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=false": "DELTA 3 Plus自動書き込み許可が無効です",
  "ECOFLOW_DELTA3_EXECUTE=false": "DELTA 3 Plus書き込み実行が無効です",
  "ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=false": "DELTA 3 Plus private API書き込み許可が無効です",
  "duplicate command candidate": "同じ送信候補のため抑制",
  "command suppressed by minimum interval": "最小送信間隔内のため抑制",
  "command retry suppressed after previous error": "直前エラー後の再試行間隔内のため抑制",
  "current AC charge limit is unavailable": "現在のAC充電上限が取得できません",
  "target AC charge limit is unavailable": "目標AC充電上限がありません",
  "target AC charge limit diff is below threshold": "AC充電上限の差分がしきい値未満です",
  "backup reserve status unavailable": "バックアップリザーブ値が取得できません",
  "mode status verified": "動作モード確認済み",
  "outside night charge window": "深夜充電時間外です",
  "night charge not needed": "深夜充電は不要です",
  "night charge command suppressed": "深夜充電コマンドを抑制しています"
};

const exactReasonLabels: Record<string, string> = {
  "DELTA3_AUX_ENABLED=false": "DELTA 3 Plus補助制御が無効です",
  "DELTA 3 Plus status unavailable": "DELTA 3 Plus状態を取得できません",
  "DELTA 3 Plus SOC or AC charge limit is unavailable": "DELTA 3 PlusのSOCまたはAC充電上限が取得できません",
  "importing from grid; reduce DELTA 3 Plus auxiliary charge toward safe minimum": "買電中のためDELTA 3 Plus補助充電を安全な最小値へ下げます",
  "waiting for recent DELTA Pro 3 command to settle": "直近のDELTA Pro 3制御の反映待ちです",
  "DELTA Pro 3 still has priority surplus absorption candidate": "DELTA Pro 3でまだ余剰吸収できるため待機します",
  "residual export is below DELTA 3 Plus auxiliary adjustment threshold": "残余売電がDELTA 3 Plus補助調整のしきい値未満です",
  "DELTA 3 Plus auxiliary target is within command diff threshold": "DELTA 3 Plus補助目標が変更しきい値内です",
  "DELTA Pro 3 priority is satisfied; use DELTA 3 Plus to absorb residual export": "DELTA Pro 3優先後の残余売電をDELTA 3 Plusで吸収します"
};

const partialReasonLabels: Array<[string, string]> = [
  ["is near max charge SOC", "DELTA 3 Plusが最大充電SOC付近です"],
  ["write client is unavailable", "書き込みクライアントが利用できません"],
  ["set DELTA 3 Plus AC charge power", "DELTA 3 Plus AC充電上限の送信に失敗しました"],
  ["set AC charge power", "AC充電上限の送信に失敗しました"],
  ["disable energy strategy modes", "動作モードOFFの送信に失敗しました"],
  ["enable TOU mode", "TOUモードONの送信に失敗しました"],
  ["set backup reserve SOC", "バックアップリザーブSOCの送信に失敗しました"]
];

export function strategyStateLabel(value: string | null | undefined) {
  if (!value) {
    return "-";
  }
  return strategyStateLabels[value] || value;
}

export function commandKindLabel(value: string | null | undefined) {
  if (!value) {
    return "操作なし";
  }
  return commandKindLabels[value] || value;
}

export function commandResultLabel({
  commandSent,
  dryRun
}: {
  commandSent?: boolean;
  dryRun?: boolean;
}) {
  if (commandSent) {
    return "送信済み";
  }
  if (dryRun) {
    return "未送信";
  }
  return "送信なし";
}

export function writeCandidateLabel(wouldWrite: boolean | null | undefined) {
  return wouldWrite ? "送信候補あり" : "送信なし";
}

export function guardReasonLabel(value: string | null | undefined) {
  if (!value) {
    return "-";
  }
  return translateReason(value, exactGuardReasonLabels);
}

export function decisionReasonLabel(value: string | null | undefined) {
  if (!value) {
    return "-";
  }
  return translateReason(value, exactReasonLabels);
}

function translateReason(value: string, exactLabels: Record<string, string>) {
  if (exactLabels[value]) {
    return exactLabels[value];
  }
  const partial = partialReasonLabels.find(([needle]) => value.includes(needle));
  if (partial) {
    return partial[1];
  }
  return value;
}
