# dd-conf-gen

AWS の Aurora MySQL と ElastiCache for Redis を検索し、Datadog Agent の DBM・Redis integration 用チェック設定を生成する CLI ツールである。

## アプローチ

タグ条件で監視対象を選び、取得した接続先とメタデータを Go テンプレートへ渡して設定ファイルを生成する。複数リージョンの集約や、ロールごとの出力にも対応する。認証情報は ENC 参照として出力し、値の取得は Datadog Agent が行う。

ECS 上の Datadog Agent を含む独自イメージで、Agent 起動前に実行する構成を想定する。設定生成に成功してから Agent を起動する。監視対象や設定を変更した場合は、再実行して Agent に設定を再適用する。

## 使い方

Go でインストールするコマンドを、以下に示す。

```bash
go install github.com/moepig/dd-conf-gen@latest
```

生成設定とテンプレートを用意し、リージョン、タグ条件、出力先を指定する。AWS の検索権限と Agent のシークレット設定を整え、以下のコマンドを実行する。

```bash
dd-conf-gen -config gen-config.yaml
```

生成設定とテンプレートの組み合わせは、[利用例](docs/examples.md) を参照。

## ドキュメント

目的別の詳細資料を、以下に示す。

- [インストールと CLI](docs/usage.md)
- [生成設定とテンプレート](docs/configuration.md)
- [設定生成の設計](docs/architecture.md)
- [Agent によるシークレット参照](docs/secrets.md)
- [開発](docs/development.md)
