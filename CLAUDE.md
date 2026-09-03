# mattermost-plugin-recurring

Mattermost に繰り返しリマインダー(「毎週月曜10時」など)を足すプラグイン。
組み込み機能は単発のみ、既存の remind プラグインはアーカイブ済み、という空席を埋める。

要件・計画の詳細は docs/REQUIREMENTS.md を参照。

## 技術スタック

- サーバー: Go 1.24 / `github.com/mattermost/mattermost/server/public/pluginapi`
- webapp: React + TypeScript
- 永続化: プラグイン KV Store のみ(独自テーブルは作らない)
- plugin id: `com.github.kpab.recurring` / ライセンス Apache-2.0

## ディレクトリ構成

Mattermost 公式の plugin starter template がベース。

```
server/                 Go サーバー側
  plugin.go             OnActivate 等のフック。ここでスケジューラを起動する
  job.go                定期実行のエントリポイント
  api.go                ServeHTTP。/plugins/com.github.kpab.recurring/api/v1/ 配下
  command/              スラッシュコマンド
  store/kvstore/        KV Store のラッパー。永続化は全部ここを通す
webapp/                 React + TypeScript。RHS はここに登録する
  src/                  実装
  i18n/                 翻訳リソース(en が正)
build/                  ビルド補助。pluginctl がサーバーへの deploy を行う
assets/icon.svg         プラグインアイコン(Marketplace 用に 512x512)
docs/REQUIREMENTS.md    要件・マイルストーン
```

## コマンド

```sh
make dist          # プラグインバンドルをビルド → dist/*.tar.gz
make test          # サーバー + webapp のテスト
make check-style   # lint
make deploy        # ビルドして稼働中のサーバーへインストール
make watch         # deploy 後、webapp の変更を自動で再デプロイ
make logs          # プラグインのサーバーログを追う
make mock          # command のモックを再生成
```

`make deploy` は環境変数で対象サーバーを指定する。mm.w4ta.com は MFA 強制のため
ユーザー名+パスワードでは通らない。システム管理者の Personal Access Token を使う:

```sh
export MM_SERVICESETTINGS_SITEURL=https://mm.w4ta.com
export MM_ADMIN_TOKEN=<token>
```

**注意**: ビルド・テスト系コマンド(`make dist` / `make test` / `go build` 等)は
ユーザーの明示的な指示があるまで実行しない。

## 規約・注意点

- **繰り返しは `cluster.JobOnceScheduler` の再予約で実現する**。発火したコールバックの中で
  次回時刻を計算し `ScheduleOnce` を再登録する。`cluster.Schedule` は固定インターバル専用で
  「毎週月曜10時」を表現できないため使わない。
- **スケジューラ経由の処理は必ずクラスタセーフに書く**。`JobOnceScheduler` は複数インスタンスでも
  1回だけ発火することを保証するので、自前の time.Ticker やゴルーチンで代替しない。
- **時刻はユーザーのタイムゾーンで解釈する**。サーバーのローカル時刻を使わない。
  内部保存は UTC、表示と次回計算はユーザー TZ。
- **UI 文言とドキュメントは英語が正、日本語は同梱の翻訳**。Marketplace の主戦場が英語圏のため。
- **Marketplace 掲載を常に意識する**: `min_server_version` を満たさない API を使わない、
  リリースは linux-amd64/arm64・darwin・windows のマルチアーキで出す。

## 検証環境

自サーバー mm.w4ta.com (`~/dev/mattermost` の docker compose, mattermost-enterprise-edition 11.7.0)。
プラグインを有効化するときは、システムコンソールだけでなく `~/dev/mattermost/.env` の
`MM_PLUGINSETTINGS_PLUGINSTATES` にも `"com.github.kpab.recurring":{"Enable":true}` を追記すること。
この環境変数が PluginStates 全体を上書きするため、追記しないと再起動で無効に戻る。
