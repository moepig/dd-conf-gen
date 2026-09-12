# 利用例

生成設定とテンプレートの組み合わせによる、検索結果の集約、タグ条件、ロール選択、シークレット参照の利用例を説明する。

## 設定例の一覧

生成設定と使用目的を、以下に示す。

| 生成設定 | 使用目的 |
| --- | --- |
| [gen-config.yaml](../examples/gen-config.yaml) | Redis の基本設定と awsenv → env、service → team のタグ名変換 |
| [gen-config-multi-region.yaml](../examples/gen-config-multi-region.yaml) | 東京・大阪の Redis を集約し、本番または検証環境から監視除外タグのある対象を除く |
| [gen-config-redis-roles.yaml](../examples/gen-config-redis-roles.yaml) | 1 回の Redis 検索から全ノード用とプライマリ用の設定を生成する |
| [gen-config-aurora-mysql.yaml](../examples/gen-config-aurora-mysql.yaml) | Aurora MySQL の全 DB インスタンス用の設定を生成する |
| [gen-config-aurora-roles.yaml](../examples/gen-config-aurora-roles.yaml) | 1 回の Aurora 検索から全インスタンス用と Writer 用の設定を生成する |
| [gen-config-secrets.yaml](../examples/gen-config-secrets.yaml) | team タグに応じた Secrets Manager の参照名を DBM のチェック設定へ出力する |
| [gen-config-redis-secrets.yaml](../examples/gen-config-redis-secrets.yaml) | team タグに応じた Secrets Manager の参照名を Redis のチェック設定へ出力する |

## 実行手順

リポジトリのルートから実行する。生成設定のリージョン、タグ条件、出力先を対象環境に合わせて変更する必要がある。AWS 認証情報と各プロバイダーの API 権限を設定すること。

複数リージョンの検索結果を集約するコマンドを、以下に示す。

```bash
dd-conf-gen -config examples/gen-config-multi-region.yaml
```

テンプレートの相対パスは生成設定のあるディレクトリから解決する。すべての配布テンプレートは ENC 参照を出力する。基本例は monitoring/redis または monitoring/mysql の username と password を参照する。参照名を対象環境に合わせて変更すること。

Agent のシークレットバックエンド設定の抜粋は [datadog.yaml](../examples/datadog.yaml) に配置している。Agent の設定へ組み込み、タスクロールと Secrets Manager への接続を設定する必要がある。API キーなどの Agent 全体の設定は別途用意する。MySQL の配布テンプレートは DBM を有効にするため、DB 側の監視ユーザー、権限、パラメータも設定すること。

## 複数の出力とロール選択

同じ resource_name を複数の出力から参照すると、検索結果を再利用する。ロール別の例では、全ノード用とプライマリ／Writer 用の設定をそれぞれ /tmp/dd-conf-gen に出力する。

同じノードの重複監視を避けるため、全ノード用とその部分集合の設定は用途に応じて選択して配置すること。別々の Agent へ配置する場合は、各 Agent の出力先に合わせて変更する。

Redis のプライマリ用テンプレートは、RoleKnown と IsPrimary の両方が true のノードを選択する。クラスターモード有効時はロールを取得できないため、このテンプレートでは除外する。クラスターモード有効の全ノードを監視する場合は、基本の redis.yaml.tmpl を使用する。

Aurora の Writer 用テンプレートは IsWriter が true のインスタンスを選択する。選択対象がない場合は instances: [] を出力する。

## タグ名の変換

タグ名の変換はテンプレートに記述する。基本の Redis テンプレートでは、クラウドタグの awsenv を Datadog の env、service を team に変換する。固定タグ instancetag:bar も出力する。

引用符や改行を含むタグ値を保持するため、quote 関数でタグ文字列全体を YAML スカラーに変換する。awsenv の変換例を、以下に示す。

```gotemplate
{{- if index .Tags "awsenv" }}
      - {{ printf "env:%s" (index .Tags "awsenv") | quote }}
{{- end }}
```

## 検索結果が 0 件の場合

集約の例では on_empty: error により、検索結果が全体で 0 件の場合に更新を中止する。一部のリージョンだけが 0 件の場合は、残りの結果で生成する。

既存ファイルを保持して処理を続ける場合は、対象の出力を on_empty: keep に変更する。対象の廃止を反映して空の設定へ更新する場合は on_empty: render を使用する。配布するテンプレートは空の結果を instances: [] として出力する。

on_empty はテンプレートによるロール選択の前に判定する。検索結果が存在していても、プライマリ／Writer が選択されなければ、その出力は instances: [] となる。

## シークレット参照

Secrets Manager の例では、team-a と team-b の条件から参照名を選択し、ENC[...] を出力する。シークレットの値の取得は Agent が行う。生成コマンドに Secrets Manager の取得権限は不要である。

条件と参照名の対応、Agent の設定、権限、ローテーションの反映方法は、[Agent によるシークレット参照](secrets.md) を参照。
