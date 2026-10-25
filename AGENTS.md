# AGENTS.md — CORTEX Development Guidelines & Agent Workflow

このドキュメントは、`GOAT-Foundry/CORTEX` リポジトリで開発を行うすべての AI コーディングエージェントおよびコントリビューターが遵守すべき標準開発フローと設計規約を定義します。

---

## 1. 標準開発ワークフロー（必須プロセス）

すべての機能追加・修正・ドキュメント更新は、以下の **6 ステップ** に従って進めてください。

1. **Issue 登録ファースト（Issue-First）**
   - 実装・変更作業を始める前に、必ず対象作業の GitHub Issue を登録（または既存 Issue を特定）してから作業を開始する。
2. **ワークツリー・別ブランチでの分離作業（Worktree & Branch Isolation）**
   - `main` ブランチへ直接コミットせず、必ず独立した Git Worktree および専用ブランチを作成して作業を行う。
   - Copilot App の Worktree セッションを使用している場合は、割り当てられたワークツリー環境（または作業用ワークツリー）上でブランチを分離して作業する。
3. **Rubber Duck レビューの実施（Rubber Duck Review）**
   - 実装およびテストが完了したら、PR を作成する前に必ず `rubber-duck` サブエージェント（利用できない実行環境では `code-review` 等の独立レビューエージェント）を起動し、設計・ロジック・エッジケース・規約準拠の観点でコードレビューを受ける。
4. **レビュー指摘の採否判断（Review Triage）**
   - `rubber-duck` の各指摘事項を精査し、**適切であれば採用してコードを修正**する。
   - **不要・過剰・事実誤認である場合は理由を明確にして棄却**する（盲目的にすべて従う必要はない）。
5. **レビュー結果を含めた PR 作成（Pull Request with Review Log）**
   - PR 本文には必ず以下を含めて送信する：
     - `Closes #<Issue番号>` による Issue 自動クローズ紐付け
     - 変更概要と検証結果（テスト・ビルド）
     - **Rubber Duck レビュー結果一覧（指摘内容 / 採用・棄却 / 判断理由・対応内容）**
6. **マージ完了報告後のクリーンアップ（Post-Merge Cleanup）**
   - ユーザーからのマージ完了報告を受け取り次第、対象の PR がマージ済みであることを確認した上で、マージ済みリモートブランチや不要になった追加ワークツリー・一時クローン（リポジトリ作業領域）を速やかに削除・整理する。
   - ※ `main` ブランチは保護対象とし削除しない。また、Copilot App セッション自身のアクティブなワークツリー上で稼働している場合は、稼働中ディレクトリ自体を強制削除せず、リモートブランチの削除と `origin/main` への同期（またはセッションのアーカイブ）で安全に整理する。

---

## 2. 品質ゲート・プロジェクト規約

### 2.1 クリーンルーム実装 & ライセンス遵守
- **ライセンス**: 本リポジトリは **MIT License** で公開される。
- **クリーンルーム原則**: AGPL-3.0 / GPL 系ライセンスの外部コードは一切コピー・翻案・流用してはならない。新規に Go モジュールや npm パッケージを追加する際も、MIT / Apache-2.0 / BSD 等の互換ライセンスであることを確認する。
- **Agent SDK**: Apache-2.0 ライセンスの `github.com/h-morozumi/Norma v0.4.5` を使用する。
  - `h-morozumi/Norma` の `go.mod` モジュール宣言に合わせて、パッケージ import は `github.com/Autumn-27/norma/...` と記述し、`go.mod` の `replace github.com/Autumn-27/norma => github.com/h-morozumi/Norma v0.4.5` で解決する。

### 2.2 ビルド & テスト検証ゲート（PR 作成前必須）
コード変更を伴う PR を送る前に、必ず以下の両方が成功することを確認する：
- **Go バックエンド**: `go test ./...` および `go build ./cmd/cortex`
  - ※ ホスト環境に Go が直接インストールされていない場合（Windows 環境など）は、`docker run --rm -v "${PWD}:/src" -w /src golang:1.26-alpine sh -c "go test ./... && go build -o /tmp/cortex ./cmd/cortex"` で検証する。
- **Next.js フロントエンド (`web/`)**: `cd web && npm run build`（`output: "export"` による Static Export と TypeScript 型チェックが通ること）
  - ※ `internal/webui` は実行時に `./web/out` が存在すればそちらを優先配信し、存在しない場合は `//go:embed all:dist`（`internal/webui/dist`）を配信する。Git リポジトリ上では `internal/webui/dist/index.html`（プレースホルダー）のみをコミット対象とし、ビルド生成物一式を `internal/webui/dist/` にコミットしないこと（Docker ビルド時または単一バイナリ配布ビルド時に `web/out/*` を `internal/webui/dist/` へコピーする）。

### 2.3 多言語対応（i18n）とドキュメント同期
- **UI 翻訳辞書**: フロントエンドに画面や文言を追加・変更する際は、`web/src/i18n/en.json`（英語）と `web/src/i18n/ja.json`（日本語）の**両方を必ず同時に更新**し、キーの欠落がないようにする。
- **エージェント出力言語連動**: UI で選択された言語（`en` / `ja`）が `internal/i18n` を通じて AI エージェントのシステムプロンプト／応答言語設定に反映される構造を維持する。
- **README 同期**: アーキテクチャ・機能・起動手順・ロードマップを変更する場合は、`README.md`（英語）と `README.ja.md`（日本語）の**両方を同期更新**する。

---

## 3. PR テンプレート（Rubber Duck レビュー記載・必須フォーマット）

PR 作成時は以下のセクション構成を必須とします：

```markdown
## Summary / 概要
Closes #<Issue番号>

- 変更点 1
- 変更点 2

## Verification / 検証結果
- [x] `go test ./...`
- [x] `cd web && npm run build`

## Rubber Duck Review / レビュー結果

| # | 指摘内容 (Finding) | 採否 (Decision) | 理由・対応内容 (Reason / Action) |
|---|---|---|---|
| 1 | ... | 採用 (Adopted) | ...を修正 |
| 2 | ... | 棄却 (Rejected) | ...のため現状維持 |
```
