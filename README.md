# dd-conf-gen

Datadog Agent チェック設定ファイルを生成する CLI ツールです。

## 概要

このツールは、クラウドリソース（ElastiCache、RDS など）を検索し、それらのリソース情報を元に Datadog Agent のチェック設定ファイルを生成します。テンプレートベースの柔軟な設定により、様々なリソース種別に対応できます。

## インストール

```bash
go install github.com/moepig/dd-conf-gen@latest
```

または、リポジトリをクローンしてビルド:

```bash
git clone https://github.com/moepig/dd-conf-gen.git
cd dd-conf-gen
go build -o dd-conf-gen
```

## 使い方

### 基本的な使い方

```bash
dd-conf-gen -meta meta-config.yaml
```

### メタ設定ファイル

メタ設定ファイルは YAML 形式で、リソース定義と出力定義を記述します。

**例: `meta-config.yaml`**

```yaml
version: "1.0"

# リソース定義（データソース）
resources:
  - name: production_redis_nodes # テンプレートから参照する識別子
    type: elasticache_redis # リソースプロバイダーの種別
    region: ap-northeast-1
    filters:
      tags:
        awsenv: Production
        service: web

# 出力定義（テンプレートと出力先）
outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.yaml
    data:
      resource_name: production_redis_nodes # resources セクションの name を参照
```

### テンプレートファイル

テンプレートファイルは Go の `text/template` を使用します。

**例: `templates/redis.yaml.tmpl`**

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
    {{- if index .Tags "service" }}
      - team:{{ index .Tags "service" }}
    {{- end }}
{{- end }}
```

**テンプレートで利用可能なデータ:**

- `.Resources`: リソースプロバイダーから取得したリソースのスライス
  - `.Host`: ホスト名
  - `.Port`: ポート番号
  - `.Tags`: リソースの全タグ（map[string]string）
    - テンプレート内で条件分岐してタグをマッピングできます
    - 例: `{{- if index .Tags "awsenv" }}` で特定のタグの存在確認
  - `.Metadata`: リソース種別固有の追加データ（map[string]interface{}）

## サポートしているリソース種別

### elasticache_redis

ElastiCache for Redis のレプリケーショングループを検索します。

**利用可能なフィールド:**

- `Host` (string): Redis エンドポイントのホスト名
- `Port` (int): Redis のポート番号（通常 6379）
- `Tags` (map[string]string): AWS リソースの全タグ（そのまま）
  - テンプレート内で必要なタグを選択・マッピングできます
- `Metadata["ClusterName"]` (string): レプリケーショングループ ID
- `Metadata["ShardName"]` (string): ノードグループ ID
- `Metadata["IsPrimary"]` (bool): プライマリノードかどうか

**フィルター:**

- `tags` (map[string]string): AWS リソースタグでフィルタリング

**タグの扱い方:**

`.Tags` にはリソースの全 AWS タグがそのまま渡されます。テンプレート内で条件分岐して、必要なタグを Datadog タグに変換します。

**例:**

```yaml
resources:
  - name: my_redis
    type: elasticache_redis
    region: ap-northeast-1
    filters:
      tags:
        Environment: Production
```

**テンプレート例（タグのマッピング）:**

```yaml
tags:
{{- if index .Tags "Environment" }}
  - env:{{ index .Tags "Environment" }}
{{- end }}
{{- if index .Tags "Team" }}
  - team:{{ index .Tags "Team" }}
{{- end }}
```

## 開発

### テストの実行

```bash
# 全てのテストを実行
go test ./... -v

# 短いテストのみ実行（モックを使用したユニットテストのみ）
go test ./... -short -v

# カバレッジを確認
go test ./... -short -cover
```

### 新しいリソースプロバイダーの追加

1. `resources/<provider_name>/` ディレクトリを作成
2. `Provider` インターフェースを実装
3. `main.go` の `init()` 関数でプロバイダーを登録

詳細は `resources/elasticache/provider.go` を参照してください。
