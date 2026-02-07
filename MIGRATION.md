# Migration Guide from v0.1.x to v0.2.0

## 破壊的変更

v0.2.0 では、アーキテクチャを完全に刷新しました。以下の破壊的変更があります。

## CLI インターフェースの変更

### 旧バージョン (v0.1.x)

```bash
dd-conf-gen -check redisdb -config config.yaml > /etc/datadog-agent/conf.d/redisdb.yaml
```

### 新バージョン (v0.2.0)

```bash
dd-conf-gen -meta meta-config.yaml
```

## 設定ファイル形式の変更

### 旧形式 (v0.1.x)

**config.yaml:**
```yaml
generate_config:
  region: ap-northeast-1
  find_tags:
    awsenv: Production
    resourcetag1: foo
  check_tags:
    env: awsenv

instance_template:
  username: "%%env_REDIS_USERNAME%%"
  password: "%%env_REDIS_PASSWORD%%"
  tags:
    - "instancetag:bar"

init_config:
```

### 新形式 (v0.2.0)

設定を2つのファイルに分離します：

**1. メタ設定ファイル (meta-config.yaml):**
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
    tag_mapping:
      env: awsenv

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.yaml
    data:
      resource_name: production_redis_nodes
      static:
        username: "%%env_REDIS_USERNAME%%"
        password: "%%env_REDIS_PASSWORD%%"
        tags:
          - "instancetag:bar"
```

**2. テンプレートファイル (templates/redis.yaml.tmpl):**
```yaml
init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    username: {{ $.Static.username }}
    password: {{ $.Static.password }}
    tags:
    {{- range $.Static.tags }}
      - {{ . }}
    {{- end }}
    {{- range $key, $value := .Tags }}
      - {{ $key }}:{{ $value }}
    {{- end }}
{{- end }}
```

## 移行手順

### ステップ 1: メタ設定ファイルの作成

既存の `config.yaml` を元に、メタ設定ファイルを作成します：

1. `generate_config.region` → `resources[].region`
2. `generate_config.find_tags` → `resources[].filters.tags`
3. `generate_config.check_tags` → `resources[].tag_mapping`
4. 出力先ファイルパスを `outputs[].output_file` に記述
5. `instance_template` の内容を `outputs[].data.static` に移動

### ステップ 2: テンプレートファイルの作成

`instance_template` と `init_config` の内容をテンプレート化します：

1. `templates/` ディレクトリを作成
2. Go の `text/template` 構文でテンプレートを作成
3. `.Resources` でリソースをループ
4. `.Static` で静的な設定値を参照

### ステップ 3: コマンドの更新

CI/CD パイプラインや運用スクリプトのコマンドを更新します：

```bash
# 旧
dd-conf-gen -check redisdb -config config.yaml > /etc/datadog-agent/conf.d/redisdb.yaml

# 新
dd-conf-gen -meta meta-config.yaml
```

**注意:** 新バージョンはファイルに直接書き込むため、標準出力へのリダイレクトは不要です。

## 主な新機能

### 1. 複数リソース種別のサポート

1つのメタ設定で複数の種類のリソースを扱えます：

```yaml
resources:
  - name: prod_redis
    type: elasticache_redis
    region: ap-northeast-1
    # ...

  - name: prod_mysql
    type: rds_mysql
    region: ap-northeast-1
    # ...

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.yaml
    data:
      resource_name: prod_redis
      # ...

  - template: templates/mysql.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/mysql.yaml
    data:
      resource_name: prod_mysql
      # ...
```

### 2. テンプレートベースの柔軟な設定

Go の `text/template` により、条件分岐やループなど柔軟な設定が可能になりました。

### 3. 複数出力ファイル

1回の実行で複数の設定ファイルを生成できます。

## トラブルシューティング

### Q: テンプレートのパスはどう解決されますか？

A: 相対パスの場合、メタ設定ファイルのディレクトリからの相対パスとして解決されます。

### Q: 既存の環境変数ベースの設定は使えますか？

A: v0.2.0 では環境変数からの読み込みには対応していません。メタ設定ファイルが必須です。

### Q: 出力ディレクトリが存在しない場合は？

A: 自動的に作成されます。

## サポート

問題が発生した場合は、GitHub Issues で報告してください：
https://github.com/moepig/dd-conf-gen/issues
