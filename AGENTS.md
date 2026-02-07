# agents.md

## 概要

- このリポジトリでは、Datadog Agent のチェック設定ファイルの生成アプリケーションを扱う
- アプリケーションは CLI ツールとして実装される

## ソフトウェアの行うこと

- チェック種別、リソース種別、リソースタグに基づいて、クラウドリソースを検索する
- 検索して得られたリソース情報を、Datadog Agent のチェック設定に変換する

## 詳細

### 設定の組み立て

- チェック設定の生成には、生成設定ファイルとDatadog チェック設定テンプレートの2つのファイルを使用する
  - **生成設定ファイル**: リソース検索の条件（リージョン、タグフィルター、タグマッピング）と出力定義を記述する
  - **Datadog チェック設定テンプレート**: Datadog Agent のチェック設定の形式を Go の `text/template` 形式で記述する
- 生成設定ファイルには、リソース定義（`resources`）と出力定義（`outputs`）を記述する
  - リソース定義: どのクラウドリソースを検索するか（type, region, filters, tag_mapping）
  - 出力定義: どのテンプレートを使って、どのファイルに出力するか（template, output_file, data）
- Datadog チェック設定テンプレートには、発見されたリソース情報（`.Resources`）をループして、チェック設定を生成するロジックを記述する

### アプリケーション設定

以下のように設定を与える。

#### ElastiCache for Redis の場合

**生成設定ファイル (`gen-config.yaml`):**

```yaml
version: "1.0"

resources:
  - name: production_redis_nodes
    type: elasticache_redis
    region: ap-northeast-1
    filters:
      tags:
        awsenv: Production
        resourcetag1: foo

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.yaml
    data:
      resource_name: production_redis_nodes
```

**Datadog チェック設定テンプレート (`templates/redis.yaml.tmpl`):**

```yaml
init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
    tags:
      - "instancetag:bar"
    {{- if index .Tags "awsenv" }}
      - env:{{ index .Tags "awsenv" }}
    {{- end }}
{{- end }}
```

**生成される Datadog Agent チェック設定ファイル:**

```yaml
init_config:

instances:
  - host: endpoint1.cache.amazonaws.com
    port: 6379
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
    tags:
      - "instancetag:bar"
      - "env:Production"
```

### アプリケーションへの設定の与え方

- オプション引数 `-config` で生成設定ファイルのパスを受け取る、このオプションは必須
- 生成設定ファイルには、リソース定義と出力定義を記述する
- Datadog チェック設定テンプレートのパスは、生成設定ファイルからの相対パスまたは絶対パスで指定する

### アーキテクチャ

- **プロバイダーアーキテクチャ**: リソース種別ごとに Provider インターフェースを実装することで、拡張可能な設計
  - `resources/interface.go`: Provider インターフェースの定義
  - `resources/elasticache/provider.go`: ElastiCache Redis の実装
  - `resources/registry.go`: プロバイダーの登録・取得機構
- **テンプレートレンダリング**: Go の `text/template` を使用して、柔軟な設定生成を実現
  - `renderer/renderer.go`: テンプレートレンダリングエンジン
- **生成設定管理**: YAML 形式の生成設定を読み込み、バリデーションを実施
  - `config/config.go`: 生成設定の読み込みとバリデーション
  - `config/types.go`: 生成設定の型定義
- **CLI**: 全体のオーケストレーションを担当
  - `main.go`: プロバイダー登録、リソース検索、テンプレートレンダリング、ファイル書き込み

### テスト

- testify を使用して実装する
- モックについても、testify を使用する
- モックを使用したユニットテストは、-short オプション指定時にも実行する
- 実行時間のかかる可能性があるテストは、-short オプション指定時には実行しないようにスキップする
- 明示的に指示されない限り、外部アクセスを含むテストはモックを使用して代替する

### リリース

- GitHub リリースを使用してリリース
- バージョンには 0.1.2 のような形式を使用
- 実装を変更した場合は、semver として見たときのマイナーバージョン部分を更新
- ライブラリを更新した場合には、semver として見たときのパッチバージョン部分を更新

### ライブラリの更新

- ユニットテストが通った場合、自動でマージする

### 新しいリソースプロバイダーの追加

新しいリソース種別（例: RDS MySQL）を追加する場合：

1. `resources/<provider_name>/` ディレクトリを作成（例: `resources/rds/`）
2. `provider.go` を作成し、`resources.Provider` インターフェースを実装
   - `Type() string`: リソース種別を返す（例: "rds_mysql"）
   - `Discover(ctx, config) ([]resources.Resource, error)`: リソースを検索
   - `ValidateConfig(config) error`: 設定をバリデーション
3. `provider_test.go` を作成し、モックを使用したユニットテストを実装
4. `main.go` の `init()` 関数でプロバイダーを登録
   ```go
   resources.Register(rds.NewProvider())
   ```
5. ドキュメント（README.md）にリソース種別の説明を追加

詳細は `resources/elasticache/provider.go` を参照。
