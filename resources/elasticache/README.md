# ElastiCache Redis Provider

## 概要

ElastiCache Redis プロバイダーは、AWS ElastiCache for Redis のレプリケーショングループからノード情報を取得します。

## リソース種別

- **Type**: `elasticache_redis`

## 設定

### 必須パラメータ

- **region** (string): AWS リージョン（例: `ap-northeast-1`）

### オプションパラメータ

#### filters

- **tags** (map[string]string): タグによるフィルタリング
  - レプリケーショングループに付与されているタグでフィルタリングします
  - 複数のタグを指定した場合、すべてのタグが一致するリソースのみが取得されます（AND 条件）

## 取得されるリソース情報

### 基本情報

| フィールド | 型 | 説明 |
|-----------|-----|------|
| `Host` | string | ノードのエンドポイント（例: `endpoint1.cache.amazonaws.com`） |
| `Port` | int | ノードのポート番号（通常は 6379） |
| `Tags` | map[string]string | レプリケーショングループに付与されているすべてのタグ |

### メタデータ (Metadata)

テンプレート内で `{{ index .Metadata "キー名" }}` の形式でアクセスできる追加情報です。

| キー | 型 | 説明 |
|------|-----|------|
| `ClusterName` | string | レプリケーショングループ ID（クラスタ名） |
| `ShardName` | string | ノードグループ ID（シャード名） |
| `IsPrimary` | bool | プライマリノードかどうか（`true`: プライマリ、`false`: レプリカ） |

## 動作詳細

### リソース検出の流れ

1. **タグによるフィルタリング**: AWS Resource Groups Tagging API を使用して、指定されたタグを持つレプリケーショングループを検索
2. **レプリケーショングループの詳細取得**: ElastiCache API を使用して、各レプリケーショングループの詳細情報を取得
3. **ノードの抽出**: 各レプリケーショングループ内のすべてのノードグループから、プライマリおよびレプリカノードのエンドポイント情報を抽出

### 取得されるノード

- クラスタモード有効/無効に関わらず、すべてのノード（プライマリ + レプリカ）を取得します
- 各ノードには、そのノードが属するレプリケーショングループのタグがすべて付与されます
- ReadEndpoint が存在するノードのみが取得されます

## 設定例

### 生成設定ファイル (gen-config.yaml)

```yaml
version: "1.0"

resources:
  - name: production_redis_nodes
    type: elasticache_redis
    region: ap-northeast-1
    filters:
      tags:
        Environment: Production
        Service: api

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.yaml
    data:
      resource_name: production_redis_nodes
```

### テンプレート例 (templates/redis.yaml.tmpl)

#### 基本的な使用例

```yaml
init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
    tags:
      - "cluster:{{ index .Metadata "ClusterName" }}"
      - "shard:{{ index .Metadata "ShardName" }}"
    {{- if index .Tags "Environment" }}
      - "env:{{ index .Tags "Environment" }}"
    {{- end }}
{{- end }}
```

#### プライマリノードのみを使用する例

```yaml
init_config:

instances:
{{- range .Resources }}
  {{- if index .Metadata "IsPrimary" }}
  - host: {{ .Host }}
    port: {{ .Port }}
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
    tags:
      - "role:primary"
      - "cluster:{{ index .Metadata "ClusterName" }}"
      - "shard:{{ index .Metadata "ShardName" }}"
  {{- end }}
{{- end }}
```

#### タグとメタデータを組み合わせた例

```yaml
init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
    tags:
      - "cluster:{{ index .Metadata "ClusterName" }}"
      - "shard:{{ index .Metadata "ShardName" }}"
      - "role:{{ if index .Metadata "IsPrimary" }}primary{{ else }}replica{{ end }}"
    {{- range $key, $value := .Tags }}
      - "{{ $key }}:{{ $value }}"
    {{- end }}
{{- end }}
```

## 必要な AWS 権限

このプロバイダーを使用するには、以下の IAM 権限が必要です:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "elasticache:DescribeReplicationGroups",
        "tag:GetResources"
      ],
      "Resource": "*"
    }
  ]
}
```

## トラブルシューティング

### リソースが取得されない場合

1. **タグフィルターの確認**: 指定したタグがレプリケーショングループに正しく付与されているか確認してください
2. **リージョンの確認**: 正しいリージョンを指定しているか確認してください
3. **IAM 権限の確認**: 必要な権限が付与されているか確認してください
4. **ReadEndpoint の有効性**: ノードが ReadEndpoint を持っているか確認してください

### ログレベルの変更

デバッグ情報を出力するには、アプリケーションのログレベルを DEBUG に設定してください。以下の情報が出力されます:

- レプリケーショングループの処理状況
- ノードグループの処理状況
- 抽出された各ノードの詳細（Host, Port, IsPrimary, ShardName）
- ReadEndpoint が存在しないノードの警告
