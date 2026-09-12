# Secrets Manager のシークレット参照

生成設定の出力定義から AWS Secrets Manager の文字列を取得し、テンプレートに渡す方法を説明する。

## 設定

`outputs[].data.secrets` に、テンプレートから参照する名前と取得元を記述する。各取得元の設定項目を、以下に示す。

| 項目 | 必須 | 内容 |
| --- | --- | --- |
| `region` | ○ | シークレットを取得する AWS リージョン |
| `secret_id` | ○ | シークレットの名前または ARN |
| `json_key` | | JSON オブジェクトのトップレベルのキー。指定時はそのキーの文字列値を取得する |
| `version_id` | | 取得するバージョン ID |
| `version_stage` | | 取得するバージョンのステージ |

`version_id` と `version_stage` の両方を省略した場合、AWS の既定動作に従って `AWSCURRENT` を取得する。両方を指定する場合は同じバージョンを指す必要がある。

`json_key` を省略した場合、`SecretString` 全体を使用する。指定したキーが存在しない場合や値が文字列でない場合はエラーとする。空文字列の値は使用できる。`SecretBinary` には対応しない。

MySQL のユーザー名とパスワードを同じ JSON シークレットから取得する出力定義を、以下に示す。`production_mysql_instances` は `resources` で定義する。

```yaml
outputs:
  - template: templates/mysql-secrets.yaml.tmpl
    output_file: /etc/datadog-agent/conf.d/mysql.d/conf.yaml
    on_empty: error
    data:
      resource_name: production_mysql_instances
      secrets:
        username:
          region: ap-northeast-1
          secret_id: production/mysql/monitoring
          json_key: username
        password:
          region: ap-northeast-1
          secret_id: production/mysql/monitoring
          json_key: password
```

## テンプレート

取得値は `.Secrets` の文字列マップで参照する。`range .Resources` の内側から参照する場合は `$.Secrets` を使用する。

`quote` 関数は文字列を二重引用符付きの YAML スカラーに変換する。パスワードに改行、引用符、コロンなどが含まれる場合も、元の文字列を保持する。

テンプレートの例を、以下に示す。

```yaml
init_config:
instances:
{{- if not .Resources }} []
{{- end }}
{{- range .Resources }}
  - host: {{ .Host | quote }}
    port: {{ .Port }}
    username: {{ $.Secrets.username | quote }}
    password: {{ $.Secrets.password | quote }}
{{- end }}
```

実行可能な生成設定とテンプレートは、[gen-config-secrets.yaml](../examples/gen-config-secrets.yaml) と [mysql-secrets.yaml.tmpl](../examples/templates/mysql-secrets.yaml.tmpl) を参照。

## 取得と保存

設定、プロバイダー、テンプレート、出力先の事前検証を終えてから外部アクセスを開始する。シークレットはリソース検索の後、各出力のテンプレート生成前に取得する。`on_empty: keep` によって省略する出力のシークレットは取得しない。

同じリージョン、シークレット ID、バージョン指定の取得結果は、1 回の実行内で共有する。JSON 内の複数のキーを参照する場合も API 呼び出しは 1 回である。次の実行では再取得するため、ローテーション後の値を反映するには生成処理を再実行する必要がある。

取得失敗や JSON フィールドの不備がある場合、すべての出力ファイルを更新せずに終了する。取得値はログに出力しない。取得値を渡したテンプレートが失敗した場合は、値がエラーメッセージに含まれる可能性があるため、実行エラーの詳細を表示しない。

シークレット参照のある出力ファイルには取得値が平文で含まれる。新規・既存ファイルともにアクセス権を `0600` に設定する。生成コマンドは Datadog Agent が出力ファイルを読める所有者で実行する必要がある。シンボリックリンクの出力先ではリンク先のファイルを更新する。

## AWS 認証と権限

AWS SDK の標準の認証情報設定を使用する。別アカウントのシークレットを参照する場合は完全な ARN を指定し、呼び出し元からのアクセスを許可する必要がある。

必要な権限を、以下に示す。

| 権限 | 適用対象 |
| --- | --- |
| `secretsmanager:GetSecretValue` | 取得するシークレット |
| `kms:Decrypt` | カスタマーマネージド KMS キーで暗号化されている場合のキー |

バージョン指定、応答形式、権限の詳細は、AWS の [GetSecretValue](https://docs.aws.amazon.com/secretsmanager/latest/apireference/API_GetSecretValue.html) を参照。
