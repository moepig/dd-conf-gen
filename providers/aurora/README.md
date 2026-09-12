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

`filters.tag_conditions` で候補値の OR、除外、タグの存在・不在も指定できる。条件の詳細は、[タグ条件](../../README.md#タグ条件) を参照。

未対応のフィルター名、マップ以外の `filters.tags`、文字列以外のタグ値は、AWS API へのアクセス前にエラーとする。

生成設定と MySQL チェックのテンプレート例は、[gen-config-aurora-mysql.yaml](../../examples/gen-config-aurora-mysql.yaml) と [mysql.yaml.tmpl](../../examples/templates/mysql.yaml.tmpl) を参照。

リポジトリのルートから設定例を実行するコマンドを、以下に示す。

```bash
dd-conf-gen -config examples/gen-config-aurora-mysql.yaml
```

`output_file` は保存先に合わせて変更すること。テンプレートには Datadog Agent の環境変数参照 `%%env_MYSQL_USERNAME%%` と `%%env_MYSQL_PASSWORD%%` を記述している。接続情報は Datadog Agent の実行環境で設定する必要がある。DB の監視ユーザーと権限は別途設定する。

このテンプレートは MySQL チェック用である。Database Monitoring を有効にする場合は、上記の Datadog 設定手順に従って DB と Agent を設定し、テンプレートへ `dbm: true` などの必要な項目を追加すること。

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

Writer のみを出力するテンプレートの例を、以下に示す。

```yaml
init_config:

instances:
{{- range .Resources }}
  {{- if .Metadata.IsWriter }}
  - host: {{ printf "%q" .Host }}
    port: {{ .Port }}
    username: "%%env_MYSQL_USERNAME%%"
    password: "%%env_MYSQL_PASSWORD%%"
  {{- end }}
{{- end }}
```

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
