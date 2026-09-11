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
dd-conf-gen -config gen-config.yaml
```

### 生成設定ファイルの構造

生成設定ファイルは YAML 形式で記述します。

#### トップレベル項目

| 項目        | 型    | 必須 | 説明                                |
| ----------- | ----- | ---- | ----------------------------------- |
| `resources` | array | ○    | リソース定義のリスト（最低1つ必要） |
| `outputs`   | array | ○    | 出力定義のリスト（最低1つ必要）     |

#### resources 項目

各リソース定義には以下の項目を指定します:

| 項目           | 型     | 必須 | 説明                                                  |
| -------------- | ------ | ---- | ----------------------------------------------------- |
| `name`         | string | ○    | リソースの識別子（outputs から参照される）            |
| `type`         | string | ○    | リソースプロバイダーの種別（例: `elasticache_redis`） |
| `region`       | string | ○    | AWS リージョン（例: `ap-northeast-1`）                |
| `filters.tags` | map    | -    | タグによるフィルタリング（key-value のペア）          |

#### outputs 項目

各出力定義には以下の項目を指定します:

| 項目                 | 型     | 必須 | 説明                                                 |
| -------------------- | ------ | ---- | ---------------------------------------------------- |
| `template`           | string | ○    | テンプレートファイルのパス（相対パスまたは絶対パス） |
| `output_file`        | string | ○    | 出力先ファイルのパス                                 |
| `data.resource_name` | string | ○    | 使用するリソースの識別子（resources の name を参照） |

#### 設定例

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

詳細な設定例については、各リソースプロバイダーのドキュメントを参照してください。

### リソース設定の検証

検索を開始する前に、全リソースのプロバイダー登録と設定を検証する。設定に不備がある場合、クラウド API を呼び出さずにエラーで終了する。

ElastiCache のフィルターは `filters.tags` のみを受け付ける。タグ値は文字列で指定すること。数値や真偽値として解釈される YAML の値は引用符で囲む必要がある。未対応のフィルター名や文字列以外のタグ値はエラーとする。

### 出力ファイルの更新

すべてのテンプレートの生成に成功した後、出力定義の順にファイルを保存する。生成中にエラーが発生した場合、出力ファイルは更新しない。

各ファイルは出力先と同じディレクトリに作成した一時ファイルから置換する。既存ファイルのアクセス権を引き継ぎ、既存のシンボリックリンクはリンク先を更新する。リンク先が存在しないシンボリックリンクはエラーとする。

保存中にエラーが発生した場合、その時点で処理を終了する。先に保存が完了したファイルは更新済みとなるため、複数ファイル全体の更新は不可分ではない。

### テンプレートの基本

Datadog チェック設定テンプレートは Go の `text/template` 形式で記述します。

`.Tags.env` や `.Metadata.ClusterName` のような参照でキーが存在しない場合、生成をエラーで終了する。任意のタグは `index` と `if` を組み合わせて参照すること。`index` による参照は欠損キーのエラー対象に含まれない。

**テンプレートで利用可能なデータ:**

- `.Resources`: リソースプロバイダーから取得したリソースのスライス
  - `.Host`: ホスト名またはエンドポイント
  - `.Port`: ポート番号
  - `.Tags`: リソースのタグ（map[string]string）
  - `.Metadata`: リソース種別固有の追加データ（map[string]interface{}）

詳細な使い方は、各リソースプロバイダーのドキュメントを参照してください。

## サポートしているリソースプロバイダー

各リソースプロバイダーの詳細（取得できるデータ、設定例、テンプレート例）については、以下のドキュメントを参照してください:

| リソース種別        | 説明                      | ドキュメント                                                       |
| ------------------- | ------------------------- | ------------------------------------------------------------------ |
| `elasticache_redis` | AWS ElastiCache for Redis | [providers/elasticache/README.md](providers/elasticache/README.md) |

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

新しいリソースプロバイダーの追加手順を、以下に示す。

1. `providers/<provider_name>/` ディレクトリを作成
2. `providers.Provider` インターフェースを実装
3. `main.go` で生成する `providers.Registry` にプロバイダーを登録
4. ドキュメント（README.md）を作成

プロバイダーの登録内容は `Registry` のインスタンスごとに独立する。テストでは専用のインスタンスにモックを登録してアプリケーションへ渡す。

`ValidateConfig` は外部アクセスや設定の変更を行わずに検証すること。プロバイダー内部の型付き設定への変換処理を用意し、`ValidateConfig` と `Discover` で共用する。`Discover` 単独でも不正な設定を拒否する必要がある。

実装の詳細は、既存の実装を参考にしてください:

- [providers/elasticache/provider.go](providers/elasticache/provider.go) - 実装例
- [providers/elasticache/README.md](providers/elasticache/README.md) - ドキュメント例
