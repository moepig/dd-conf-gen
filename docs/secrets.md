# Agent によるシークレット参照

dd-conf-gen が接続先ごとにシークレット参照名を出力し、Datadog Agent が AWS Secrets Manager から認証情報を取得する設定を説明する。

## Agent の設定

AWS Secrets Manager のネイティブバックエンドには Datadog Agent 7.70 以降を使用する。Agent の datadog.yaml に追加する設定を、以下に示す。

```yaml
secret_backend_type: aws.secrets
secret_backend_config:
  aws_session:
    aws_region: ap-northeast-1
```

チェック設定の ENC[シークレット名;JSON キー] は、指定した JSON シークレットの値への参照である。名前の代わりに ARN も使用できる。接続先ごとに参照名を変えることで、同じ Agent から異なる認証情報を使用できる。

構文と Agent バージョンごとの対応範囲は、Datadog の [Secrets Management](https://docs.datadoghq.com/agent/configuration/secrets-management/?tab=agentyamlfile) を参照。

設定の抜粋は [datadog.yaml](../examples/datadog.yaml) に配置している。Agent の既存設定へ追加すること。API キーなどの共通設定は含まない。

## 条件別の認証情報

条件別の配布例では、env: prod かつ team が team-a または team-b の Aurora クラスターまたは ElastiCache レプリケーショングループを検索する。team タグの値から参照名を構成する。条件と参照先を、以下に示す。

| チェック | team タグ | シークレット名 | JSON キー |
| --- | --- | --- | --- |
| MySQL | team-a | production/team-a/mysql/monitoring | username、password |
| MySQL | team-b | production/team-b/mysql/monitoring | username、password |
| Redis | team-a | production/team-a/redis/monitoring | username、password |
| Redis | team-b | production/team-b/redis/monitoring | username、password |

検索条件は [gen-config-secrets.yaml](../examples/gen-config-secrets.yaml)、参照名の生成は [mysql-secrets.yaml.tmpl](../examples/templates/mysql-secrets.yaml.tmpl) に記述する。これらのファイルのリージョン、タグ条件、参照名、出力先を対象環境に合わせて変更すること。DBM に必要な DB 側の監視ユーザーと権限は別途設定する。

Redis の検索条件は [gen-config-redis-secrets.yaml](../examples/gen-config-redis-secrets.yaml)、参照名の生成は [redis-secrets.yaml.tmpl](../examples/templates/redis-secrets.yaml.tmpl) に記述する。Redis 側の監視ユーザーと権限も別途設定する。

Team A のインスタンスに対する生成結果の例を、以下に示す。

```yaml
init_config:
instances:
  - host: instance-a.example.rds.amazonaws.com
    port: 3306
    dbm: true
    username: "ENC[production/team-a/mysql/monitoring;username]"
    password: "ENC[production/team-a/mysql/monitoring;password]"
    aws:
      instance_endpoint: instance-a.example.rds.amazonaws.com
      region: ap-northeast-1
```

Redis integration も username と password に同じ形式の参照を使用できる。条件ごとに参照先の命名規則が異なる場合は、Go テンプレートの条件分岐か、リソース定義と出力の分割で対応する。

## 取得と権限

dd-conf-gen はシークレットの値を取得せず、参照先の存在確認も行わない。生成時に AWS Secrets Manager へのアクセスは発生しない。設定ファイルに保存するのは ENC 参照であり、値は Agent が解決する。

Agent を実行する ECS タスクロールには、参照先に対する secretsmanager:GetSecretValue を付与する。カスタマーマネージド KMS キーを使用する場合は、そのキーの kms:Decrypt も必要である。権限の詳細は、AWS の [GetSecretValue](https://docs.aws.amazon.com/secretsmanager/latest/apireference/API_GetSecretValue.html) を参照。

シークレット取得の成功と DB 接続の成功は Agent 側で確認する。生成設定やテンプレートに認証情報の値を直接記述しないこと。dd-conf-gen はテンプレート中の平文を検出・除去する機能を持たない。

## 値の更新

参照先の値だけをローテーションした場合は、Agent 側で値を更新する。Agent 7.76 以降では Autodiscovery に登録されたチェックの ENC 参照を実行中に更新する機能が提供されている。secret_refresh_interval または手動の secret refresh を使用し、使用する Agent イメージと生成ファイルの読み込み方式でチェックへの反映を確認すること。対応しない構成では Agent を再起動する。

参照先のシークレット名や接続先を変更する場合は、dd-conf-gen の再実行と Agent への設定の再適用が必要である。

更新機能の詳細は、Datadog の [Refreshing secrets at runtime](https://docs.datadoghq.com/agent/configuration/secrets-management/?lang_pref=en#refreshing-secrets-at-runtime) を参照。
