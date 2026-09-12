# Aurora MySQL プロバイダー

AWS Aurora MySQL の DB クラスターを検索し、各 DB インスタンスの接続情報を Datadog Agent の MySQL チェック設定へ渡す。

## 検索対象

リソース種別は `aurora_mysql` である。指定リージョンの `aurora-mysql` エンジンのクラスターを検索し、Writer と Reader の DB インスタンスを取得する。Aurora PostgreSQL と通常の RDS MySQL は対象に含まない。

`filters.tags` は DB クラスターのタグに対して適用する。指定したすべてのキーと値が、大文字小文字を区別して完全一致するクラスターを選択する。空文字列の値も完全一致で判定し、キーが存在しないタグとは区別する。フィルターの省略時や空のタグマップの指定時は、タグのないクラスターも対象とする。

DB インスタンスの個別エンドポイントを使用する。エンドポイントのアドレスが空の場合、またはポート番号が 1〜65535 の範囲外の場合は、警告を出力してそのインスタンスを除外する。DB インスタンスを持たないクラスターからはリソースを生成しない。

Datadog が指定する Aurora の接続先については、[Aurora MySQL の Database Monitoring 設定手順](https://docs.datadoghq.com/database_monitoring/setup_mysql/aurora/) を参照。

## 設定

生成設定の項目を、以下に示す。

| 項目 | 必須 | 説明 |
| --- | --- | --- |
| `type` | ○ | `aurora_mysql` |
| `region` | ○ | 検索対象の AWS リージョン |
| `filters.tags` | | クラスタータグのキーと文字列値のマップ |

`filters.tag_conditions` で候補値の OR、除外、タグの存在・不在も指定できる。条件の詳細は、[タグ条件](../configuration.md#タグ条件) を参照。

未対応のフィルター名、マップ以外の `filters.tags`、文字列以外のタグ値は、AWS API へのアクセス前にエラーとする。

生成設定と MySQL チェックのテンプレート例は、[gen-config-aurora-mysql.yaml](../../examples/gen-config-aurora-mysql.yaml) と [mysql.yaml.tmpl](../../examples/templates/mysql.yaml.tmpl) を参照。

リポジトリのルートから設定例を実行するコマンドを、以下に示す。

```bash
dd-conf-gen -config examples/gen-config-aurora-mysql.yaml
```

output_file は保存先に合わせて変更すること。テンプレートは monitoring/mysql の username と password を ENC 参照として出力する。参照名とテンプレート内の AWS リージョンを対象環境に合わせ、Agent 側のシークレットバックエンドを設定する必要がある。条件別の参照方法は、[Agent によるシークレット参照](../../docs/secrets.md) を参照。

配布する MySQL テンプレートは dbm: true を出力する。DB の監視ユーザー、権限、DBM に必要な DB パラメータは、上記の Datadog 設定手順に従って別途設定すること。

## テンプレートデータ

`.Resources` は DB インスタンスごとの接続情報の配列である。各要素のデータを、以下に示す。

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `.Host` | string | DB インスタンスのエンドポイントアドレス |
| `.Port` | int | DB インスタンスのエンドポイントポート |
| `.Tags` | map[string]string | DB クラスターのタグ。DB インスタンスのタグは含まない |
| `.Metadata.ClusterName` | string | DB クラスター識別子 |
| `.Metadata.DBInstanceID` | string | DB インスタンス識別子 |
| `.Metadata.IsWriter` | bool | クラスターのメンバー情報で Writer に指定されている場合に `true` |
| `.Metadata.EngineVersion` | string | DB インスタンスのエンジンバージョン |
| `.Metadata.AvailabilityZone` | string | DB インスタンスの Availability Zone |

Writer のみを出力する場合は `IsWriter` で選択する。対象がない場合は `instances: []` を出力する。

テンプレートと複数出力の設定例は、[mysql-writer.yaml.tmpl](../../examples/templates/mysql-writer.yaml.tmpl) と [gen-config-aurora-roles.yaml](../../examples/gen-config-aurora-roles.yaml) を参照。

## AWS 権限

AWS SDK の標準の認証情報設定を使用する。必要な IAM 権限を、以下に示す。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "rds:DescribeDBClusters",
        "rds:DescribeDBInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

クラスタータグは `DescribeDBClusters` の `TagList` から取得する。API の応答形式と検索条件の詳細は、AWS の [DBCluster](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_DBCluster.html) と [DescribeDBInstances](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_DescribeDBInstances.html) を参照。
