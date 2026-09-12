# 生成設定とテンプレート

生成設定の項目、リソースの選択、テンプレートデータ、出力ファイルの更新仕様を説明する。

## 生成設定ファイルの構造

生成設定ファイルは単一の YAML ドキュメントとして記述する。未定義の設定項目や複数の YAML ドキュメントはエラーとする。

### トップレベル項目

生成設定のトップレベル項目を、以下に示す。

| 項目        | 型    | 必須 | 説明                                |
| ----------- | ----- | ---- | ----------------------------------- |
| resources | array | ○    | リソース定義のリスト（最低 1 つ必要） |
| outputs   | array | ○    | 出力定義のリスト（最低 1 つ必要）     |

### resources 項目

各リソース定義の項目を、以下に示す。

| 項目           | 型     | 必須 | 説明                                                  |
| -------------- | ------ | ---- | ----------------------------------------------------- |
| name         | string | ○    | リソースの識別子（outputs から参照される）            |
| type         | string | ○    | リソースプロバイダーの種別（例: elasticache_redis） |
| region       | string | ○    | AWS リージョン（例: ap-northeast-1）                |
| filters.tags | map    | -    | タグによる完全一致の AND 条件 |
| filters.tag_conditions | array | - | 候補値、除外、存在・不在によるタグ条件 |

### outputs 項目

各出力定義の項目を、以下に示す。

| 項目                 | 型     | 必須 | 説明                                                 |
| -------------------- | ------ | ---- | ---------------------------------------------------- |
| template           | string | ○    | テンプレートファイルのパス（相対パスまたは絶対パス） |
| output_file        | string | ○    | 出力先ファイルのパス                                 |
| on_empty | string | | 検索結果が 0 件の場合の動作。既定値は render |
| data.resource_name | string | 条件付き | 使用するリソース定義の名前 |
| data.resource_names | array | 条件付き | 集約するリソース定義の名前のリスト |

data.resource_name または空でない data.resource_names のどちらか一方を指定する。同じリソース定義の重複参照はエラーとする。複数定義を参照する場合、ホスト名とポート番号が同じリソースは最初の定義を優先して 1 件にまとめ、ホスト名、ポート番号の順に並べる。タグとメタデータは優先されたリソースの値を使用する。1 定義のみの参照では、検索結果の順序と件数を保持する。

東京と大阪の検索結果を集約する出力定義を、以下に示す。resources には tokyo_redis と osaka_redis をそれぞれのリージョンで定義する。

```yaml
outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.d/conf.yaml
    data:
      resource_names: [tokyo_redis, osaka_redis]
```

### 設定例

タグ条件で Redis ノードを検索する生成設定を、以下に示す。

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

複数リージョンの集約、タグ条件、ロール別の出力、シークレット参照の設定例は、[利用例](examples.md) を参照。

## リソース設定の検証

検索を開始する前に、全リソースのプロバイダー登録と設定を検証する。設定に不備がある場合、クラウド API を呼び出さずにエラーで終了する。

テンプレートの読み込みと構文解析、既存の出力先のファイル種別とシンボリックリンクの検証も検索前に行う。解析したテンプレートを検索後の生成に使用する。書き込み権限や空き容量、検証後のファイルシステム変更によるエラーは保存時に判定する。

出力先はシンボリックリンクを解決してから .. を処理し、絶対パスとして保持する。同じ出力先を複数の出力定義で指定してはいけない。保存には検証済みのパスを使用し、そのパスのシンボリックリンクによる転送先が変化した場合はエラーとする。検証後に元のエイリアスだけを変更しても、保存先は変わらない。保存処理と同時に行われる外部からのファイルシステム変更に対する排他制御は行わない。

ElastiCache と Aurora MySQL のフィルターは filters.tags と filters.tag_conditions を受け付ける。タグ値は文字列で指定すること。数値や真偽値として解釈される YAML の値は引用符で囲む必要がある。未対応のフィルター名や文字列以外のタグ値はエラーとする。

## タグ条件

filters.tag_conditions の条件はすべて AND で結合し、filters.tags と併用した場合は両方を満たすリソースを選択する。対象は ElastiCache のレプリケーショングループタグと Aurora のクラスタータグである。キーと値は大文字小文字を区別する。

各条件は key と operator を指定する。演算子と values の指定方法を、以下に示す。

| operator | values | 一致条件 |
| --- | --- | --- |
| in | 空でない文字列リスト | タグが存在し、値が候補のいずれかと完全一致する |
| not_in | 空でない文字列リスト | タグが存在しない、または値が候補のどれとも一致しない |
| exists | 指定禁止 | タグが存在する。空文字列の値も含む |
| not_exists | 指定禁止 | タグが存在しない |

本番または検証環境から、監視除外タグのある対象を除く条件を、以下に示す。

```yaml
filters:
  tag_conditions:
    - key: env
      operator: in
      values: [prod, staging]
    - key: monitoring-disabled
      operator: not_exists
```

タグ条件は取得したタグに対して判定する。filters.tags を省略した検索はタグのないリソースも評価するため、除外条件のみの検索も使用できる。

## 出力ファイルの更新

すべてのテンプレートの生成に成功した後、出力定義の順にファイルを保存する。生成中にエラーが発生した場合、出力ファイルは更新しない。

各ファイルは出力先と同じディレクトリに作成した一時ファイルから置換する。既存ファイルのアクセス権を引き継ぐ。既存のシンボリックリンクはリンク先を更新する。リンク先が存在しないシンボリックリンクはエラーとする。

新規ファイルのアクセス権は 0644 にプロセスの umask を適用する。既存ファイルのパーミッションビットは保持する。所有者・ACL・拡張属性の保持は保証しない。

保存中にエラーが発生した場合、その時点で処理を終了する。先に保存が完了したファイルは更新済みとなるため、複数ファイル全体の更新は不可分ではない。

## 検索結果が 0 件の場合

outputs[].on_empty は、出力に使用する検索結果の集約後、テンプレートでの絞り込み前の件数に適用する。動作を、以下に示す。

| 値 | 動作 |
| --- | --- |
| render | 空の .Resources を渡して生成し、保存する。既定の動作である |
| error | エラーで終了し、すべての出力ファイルを更新しない |
| keep | その出力の生成と保存を省略する。既存ファイルを保持し、新規ファイルは作成しない |

keep の場合も、テンプレートと出力先の事前検証を行う。AWS API のエラーは 0 件と扱わず、通常どおり処理全体を失敗とする。テンプレート内の条件で全件を除外した場合は on_empty の対象にならない。

必須の監視対象が見つからない場合に更新を中止する設定を、以下に示す。

```yaml
outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/redisdb.d/conf.yaml
    on_empty: error
    data:
      resource_name: production_redis_nodes
```

## テンプレートの基本

Datadog チェック設定テンプレートは Go の text/template 形式で記述する。

.Tags.env や .Metadata.ClusterName のような参照でキーが存在しない場合、生成をエラーで終了する。任意のタグは index と if を組み合わせて参照すること。index による参照は欠損キーのエラー対象に含まれない。

テンプレートに渡すデータを、以下に示す。

| データ | 内容 |
| --- | --- |
| .Resources | 検索したリソースのスライス |
| .Resources の各要素の .Host | ホスト名またはエンドポイント |
| .Resources の各要素の .Port | ポート番号 |
| .Resources の各要素の .Tags | リソースのタグ。型は map[string]string |
| .Resources の各要素の .Metadata | リソース種別固有の追加データ。型は map[string]interface{} |

quote 関数は文字列を二重引用符付きの YAML スカラーへ変換する。テンプレートで生成した ENC 参照は解決せず、そのまま保存する。配布テンプレートは Agent のシークレットバックエンドによる値の取得を前提とする。条件別の認証情報の選択は、[Agent によるシークレット参照](secrets.md) を参照。

## サポートしているリソースプロバイダー

各リソースプロバイダーの種別と利用説明を、以下に示す。

| リソース種別        | 説明                      | ドキュメント                                                       |
| ------------------- | ------------------------- | ------------------------------------------------------------------ |
| elasticache_redis | AWS ElastiCache for Redis | [providers/elasticache.md](providers/elasticache.md) |
| aurora_mysql | AWS Aurora MySQL | [providers/aurora.md](providers/aurora.md) |

Aurora MySQL はクラスターのタグで検索し、各 DB インスタンスのエンドポイントを取得する。実行可能な生成設定例は、[examples/gen-config-aurora-mysql.yaml](../examples/gen-config-aurora-mysql.yaml) を参照。
