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

## 設定と出力の例

ElastiCache for Redis のノードを検索し、/tmp/redisdb.yaml にチェック設定を生成する例を示す。ap-northeast-1 に awsenv: Production と service: web のタグを持つレプリケーショングループがあり、検索結果がホスト名 redis-001.example.com、ポート番号 6379 の 1 ノードであると仮定する。

生成設定を gen-config.yaml として保存する。内容は次のとおりである。

```yaml
resources:
  - name: production_redis_nodes
    type: elasticache_redis
    region: ap-northeast-1
    filters:
      tags:
        awsenv: Production
        service: web

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /tmp/redisdb.yaml
    data:
      resource_name: production_redis_nodes
```

gen-config.yaml と同じディレクトリに templates ディレクトリを作成し、次の内容を templates/redis.yaml.tmpl として保存する。

```gotemplate
init_config:

instances:
{{- if not .Resources }} []
{{- end }}
{{- range .Resources }}
  - host: {{ .Host | quote }}
    port: {{ .Port }}
    username: "ENC[monitoring/redis;username]"
    password: "ENC[monitoring/redis;password]"
    tags:
      - "instancetag:bar"
    {{- if index .Tags "awsenv" }}
      - {{ printf "env:%s" (index .Tags "awsenv") | quote }}
    {{- end }}
    {{- if index .Tags "service" }}
      - {{ printf "team:%s" (index .Tags "service") | quote }}
    {{- end }}
{{- end }}
```

AWS 認証情報と ElastiCache の検索権限を設定し、gen-config.yaml を保存したディレクトリで次のコマンドを実行する。

```bash
dd-conf-gen -config gen-config.yaml
```

上記のノードが取得できた場合、/tmp/redisdb.yaml に次の内容が出力される。

```yaml
init_config:

instances:
  - host: "redis-001.example.com"
    port: 6379
    username: "ENC[monitoring/redis;username]"
    password: "ENC[monitoring/redis;password]"
    tags:
      - "instancetag:bar"
      - "env:Production"
      - "team:web"
```

接続先は検索結果から埋め込み、awsenv タグは env、service タグは team として出力する。複数のノードが見つかれば、instances にノードごとの設定が追加される。

Datadog Agent に読み込ませる場合は、output_file を /etc/datadog-agent/conf.d/redisdb.d/conf.yaml などの Agent の設定配置先へ変更し、書き込み権限を確保する。ENC[...] はそのまま出力され、認証情報の取得は Agent が行う。参照名は環境に合わせて変更する必要がある。Agent 側の設定方法は、[Agent によるシークレット参照](docs/secrets.md) を参照。

Aurora MySQL、複数リージョン、ロール別の生成設定とテンプレートの組み合わせは、[利用例](docs/examples.md) を参照。

## ドキュメント

目的別の詳細資料を、以下に示す。

- [インストールと CLI](docs/usage.md)
- [生成設定とテンプレート](docs/configuration.md)
- [設定生成の設計](docs/architecture.md)
- [プロバイダーとレジストリの関係](docs/provider-modularity.md)
- [Agent によるシークレット参照](docs/secrets.md)
- [開発](docs/development.md)
