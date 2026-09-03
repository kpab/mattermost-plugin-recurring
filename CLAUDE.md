# mattermost-plugin-recurring

Mattermost に繰り返しリマインダー(「毎週月曜10時」など)を足すプラグイン。
組み込み機能は単発のみ、既存の remind プラグインはアーカイブ済み、という空席を埋める。

要件・計画の詳細は docs/REQUIREMENTS.md を参照。

## 技術スタック

- サーバー: Go 1.25 / `github.com/mattermost/mattermost/server/public/pluginapi`
- webapp: React + TypeScript
- 永続化: プラグイン KV Store のみ(独自テーブルは作らない)
- plugin id: `com.github.kpab.recurring` / ライセンス Apache-2.0

## ディレクトリ構成

Mattermost 公式の plugin starter template がベース。

```
server/                 Go サーバー側
  plugin.go             OnActivate 等のフック。ボット確保とスケジューラ起動
  scheduler.go          JobOnceScheduler の配線。発火 → DM → 次回を再予約
  command.go            スラッシュコマンド /recurring
  api.go                ServeHTTP。/plugins/com.github.kpab.recurring/api/v1/ 配下
  reminder/             ドメイン。Mattermost に依存させない
    reminder.go         Reminder / Schedule 型と検証
    schedule.go         次回発火時刻の計算
    parse.go            日英の自然語パーサー
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

- **配信は `cluster.Schedule` の定期スイープで行う**。1分ごとに全リマインダーを見て
  `NextRunAt` を過ぎたものを配信し、その場で次回を計算して書き戻す。
  KV のリマインダーだけが唯一の状態で、ジョブという別状態を持たない。
- **`cluster.JobOnceScheduler` を繰り返しに使ってはいけない**。一度試して失敗している:
  コールバックはそのキーのクラスタミューテックスを**保持したまま**呼ばれるため、
  同じキーを `ScheduleOnce` で再予約すると自分自身のロックを待って恒久デッドロックする。
  さらに `saveMetadata` は既存キーを上書きできず、コールバックから戻った直後に
  ジョブレコードが削除される。名前のとおり「一度きり」のジョブ専用。
- **スケジューラ経由の処理は必ずクラスタセーフに書く**。`cluster.Schedule` は複数インスタンスでも
  同時に1つしか走らないことを保証するので、自前の time.Ticker やゴルーチンで代替しない。
- **配信の書き戻しは `UpdateReminder`(存在時のみ更新)を使う**。`SaveReminder` は upsert なので、
  配信中にユーザーが削除したリマインダーを復活させてしまう。
- **時刻はユーザーのタイムゾーンで解釈する**。サーバーのローカル時刻を使わない。
  内部保存は UTC(Unix ミリ秒)、表示と次回計算はリマインダーが持つ TZ。
  TZ はユーザー設定を都度引くのではなくリマインダー自身に持たせてある。
  ユーザーが後から TZ を変えたときに既存分が黙ってズレないようにするため。
- **次回発火時刻の計算に 24h 加算を使わない**。`server/reminder/schedule.go` のように
  日付を1日ずつ進めて `time.Date` で組み直す。DST のある TZ で 24h 加算は1時間ズレる。
  また DST で消える時刻(春の 02:30 など)は `time.Date` が1時間**前**に丸めてしまうため、
  ギャップの向こう側へ繰り上げている(早く鳴るより遅く鳴るほうがマシなため)。
- **文字列の長さはルーンで数える**。日本語は1文字3バイトで、バイト長で上限を課すと
  日本語ユーザーだけ実質3分の1の制限になる。
- **`strings.ToLower` した文字列のオフセットで元文字列をスライスしない**。ToLower は
  バイト長を保存しない(U+212A ケルビン記号は3バイト→1バイト)。
  大小無視のマッチは正規表現の `(?i)` で行う。
- **`_ "time/tzdata"` を消さない**。`CGO_ENABLED=0` でクロスビルドするため、
  これが無いと zoneinfo の無い実行環境で全リマインダーが黙って UTC 扱いになる。
- **モバイルアプリでは webapp プラグインが一切動かない**。RHS もチャンネルヘッダーの
  ボタンも表示されない。全プラットフォームで使える操作面は
  **スラッシュコマンドと、投稿に付けた attachment actions のボタンだけ**
  (ボタンがモバイルで動くことは実機で確認済み)。
  したがって機能を RHS に集中させてはいけない。RHS はデスクトップ向けの補助であり、
  主たる操作面はコマンドとボタン。
- **UI 文言とドキュメントは英語が正、日本語は同梱の翻訳**。Marketplace の主戦場が英語圏のため。
- **Marketplace 掲載を常に意識する**: `min_server_version` を満たさない API を使わない、
  リリースは linux-amd64/arm64・darwin・windows のマルチアーキで出す。

## 検証環境

自サーバー mm.w4ta.com (`~/dev/mattermost` の docker compose, mattermost-enterprise-edition 11.7.0)。
プラグインを有効化するときは、システムコンソールだけでなく `~/dev/mattermost/.env` の
`MM_PLUGINSETTINGS_PLUGINSTATES` にも `"com.github.kpab.recurring":{"Enable":true}` を追記すること。
この環境変数が PluginStates 全体を上書きするため、追記しないと再起動で無効に戻る。
