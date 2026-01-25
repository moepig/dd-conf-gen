# agents.md

## 概要

- このリポジトリでは、Datadog Agent のチェック設定ファイルの生成アプリケーションを扱う
- アプリケーションは CLI ツールとして実装される

## ソフトウェアの行うこと

- チェック種別、リソース種別、リソースタグに基づいて、クラウドリソースを検索する
- 検索して得られたリソース情報を、Datadog Agent のチェック設定に変換する

## 詳細

### 設定の組み立て

- チェック設定で静的に与えたい内容は、このアプリケーションに読み込ませる YAML ファイルに記述しておく
  - 静的に与えたい内容は、Datadog Agent のチェック設定ファイルと同一形式で記述する
  - 形式のチェックは行わず、存在する要素を転記することで、同一形式による記述を実現する
- リソース検索の動作に関する内容も、同一の YAML ファイルに `.generate_config` の子要素として記述しておく
  - この要素はアプリケーションの制御にのみ使用され、最終的な設定ファイルには含まれない

### アプリケーション設定

以下のように設定を与える。

#### ElastiCache for Redis の場合

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

生成される Datadog Agent チェック設定ファイルは以下のようになる。

```yaml
init_config:

instances:
  host: endpoint1
  port: 6379
  username: "%%env_REDIS_USERNAME%%"
  password: "%%env_REDIS_PASSWORD%%"
  tags:
    - "env:Production"
    - "instancetag:bar"
```

### アプリケーションへの設定の与え方

- オプション引数で生成対象のチェック構成の名前を受け取る、このオプションは必須
- オプション引数でパスの与えられた設定ファイル、もしくは環境変数から、生成対象のチェック構成ごとの設定を読み込む

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
