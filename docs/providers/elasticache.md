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

候補値の OR、除外、タグの存在・不在は `filters.tag_conditions` で指定する。条件の詳細は、[タグ条件](../configuration.md#タグ条件) を参照。

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
| `RoleKnown` | bool | ノードのロールを取得できた場合に `true` |
| `IsPrimary` | bool | `RoleKnown` が `true` の場合のプライマリ判定。クラスターモード有効時はキーを含まない |
| `CacheClusterID` | string | ノードが属するキャッシュクラスター ID |

## 動作詳細

### リソース検出の流れ

1. **タグによるフィルタリング**: AWS Resource Groups Tagging API を使用して、指定されたタグを持つレプリケーショングループを検索
2. **レプリケーショングループの詳細取得**: ElastiCache API を使用して、各レプリケーショングループの詳細情報を取得
3. **ノードの抽出**: 各レプリケーショングループ内のすべてのノードグループから、プライマリおよびレプリカノードのエンドポイント情報を抽出

### 取得されるノード

- クラスターモードの有効・無効にかかわらず、各シャードの全ノードを取得する
- クラスターモード有効時は `DescribeCacheClusters` のノード情報を取得し、`CacheClusterId` と `CacheNodeId` でメンバーに対応づける
- 各ノードには、そのノードが属するレプリケーショングループのタグがすべて付与されます
- クラスターモード無効時は `ReadEndpoint`、有効時はキャッシュノードの `Endpoint` を使用する
- 接続情報が未取得のノードは警告を出して除外する。API 呼び出しの失敗時は検索全体をエラーとする

`ReadEndpoint` と `CurrentRole` はクラスターモード無効時のみ利用できる。有効時は `RoleKnown` を `false` とし、`IsPrimary` を含めない。プライマリだけを出力するテンプレートでは、ロール不明のノードは出力対象に含まれない。

API の適用範囲とノード接続情報の取得方法は、[NodeGroupMember](https://docs.aws.amazon.com/AmazonElastiCache/latest/APIReference/API_NodeGroupMember.html) と [DescribeCacheClusters](https://docs.aws.amazon.com/AmazonElastiCache/latest/APIReference/API_DescribeCacheClusters.html) を参照。

## 設定例

テンプレートは monitoring/redis の username と password を ENC 参照として出力する。参照名を対象環境に合わせ、Agent 側のシークレットバックエンドを設定する必要がある。条件別の参照方法は、[Agent によるシークレット参照](../../docs/secrets.md) を参照。

### 生成設定ファイル (gen-config.yaml)

```yaml
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

全ノードの接続情報とクラスター・シャードのタグを出力するテンプレートを、以下に示す。

```yaml
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
      - {{ printf "cluster:%s" (index .Metadata "ClusterName") | quote }}
      - {{ printf "shard:%s" (index .Metadata "ShardName") | quote }}
    {{- if index .Tags "Environment" }}
      - {{ printf "env:%s" (index .Tags "Environment") | quote }}
    {{- end }}
{{- end }}
```

#### プライマリノードのみを使用する例

`RoleKnown` と `IsPrimary` が両方 `true` のノードを選択する。対象がない場合は `instances: []` を出力する。

テンプレートと複数出力の設定例は、[redis-primary.yaml.tmpl](../../examples/templates/redis-primary.yaml.tmpl) と [gen-config-redis-roles.yaml](../../examples/gen-config-redis-roles.yaml) を参照。

#### タグとメタデータを組み合わせた例

ロールとクラウドタグを出力するテンプレートを、以下に示す。

```yaml
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
      - {{ printf "cluster:%s" (index .Metadata "ClusterName") | quote }}
      - {{ printf "shard:%s" (index .Metadata "ShardName") | quote }}
      - "role:{{ if not (index .Metadata "RoleKnown") }}unknown{{ else if index .Metadata "IsPrimary" }}primary{{ else }}replica{{ end }}"
    {{- range $key, $value := .Tags }}
      - {{ printf "%s:%s" $key $value | quote }}
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
        "elasticache:DescribeCacheClusters",
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
