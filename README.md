# Energy Controller Codex Plan

このフォルダは、Codexに家庭用蓄電制御アプリを実装させるための設計ファイル一式です。

## 含まれるファイル

- `AGENTS.md`
  - Codexが最初に読むプロジェクト指示ファイルです。
  - 安全ルール、実装ルール、完了条件を書いています。

- `implementation-plan.md`
  - Codexに段階的に実装させるための詳細計画です。
  - プロジェクト構造、DB設計、API設計、実装フェーズ、Codexへの指示文を含みます。

- `implementation-plan.html`
  - ブラウザで確認しやすいHTML版です。

## 使い方

1. 新しいリポジトリを作成します。

```bash
mkdir energy-controller
cd energy-controller
```

2. このフォルダ内の以下2ファイルをリポジトリ直下にコピーします。

```bash
cp AGENTS.md /path/to/energy-controller/
cp implementation-plan.md /path/to/energy-controller/
```

3. Codex CLIを起動します。

```bash
cd /path/to/energy-controller
codex
```

4. `implementation-plan.md` の「最初にCodexへ投げるプロンプト」を貼り付けます。

## 最重要ルール

最初は必ず以下のモードで実装してください。

```env
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false
```

EcoFlowへの実制御は最後のフェーズまで有効化しないでください。
