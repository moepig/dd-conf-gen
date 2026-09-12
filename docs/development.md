# 開発

ソース構成、テスト方法、リソースプロバイダーの追加手順を説明する。

## ソース構成

CLI の内部実装とそのテストは internal 配下に配置する。パッケージと資料の配置を、以下に示す。

| パス | 内容 |
| --- | --- |
| main.go | バージョン定義と CLI の起動 |
| internal/app/ | CLI 引数、終了コード、シグナル処理、設定生成の実行制御 |
| internal/config/ | 生成設定の読み込みと検証 |
| internal/providers/ | リソース検索の共通型と各 AWS プロバイダー |
| internal/renderer/ | テンプレートの読み込みと生成 |
| internal/output/ | 出力先の検証とファイル保存 |
| internal/logging/ | コンテキストによるロガーの受け渡し |
| internal/tagfilter/ | タグ条件の検証と判定 |
| tests/ | CI ワークフローの検証 |
| docs/ | 設計、シークレット参照、プロバイダーの利用説明 |
| examples/ | 配布する生成設定とテンプレート |

Go の実装を検証するテストは対象のパッケージと同じディレクトリに配置する。配布例を検証するテストは examples 内の実ファイルを読み込む。内部パッケージは外部モジュール向けの Go API として公開しない。

## テストの実行

テストとモックには testify を使用し、外部アクセスはモックで代替する。競合検出を含む全テストと静的検証のコマンドを、以下に示す。

```bash
go test -race ./...
go vet ./...
```

## 新しいリソースプロバイダーの追加

新しいリソースプロバイダーの追加手順を、以下に示す。

1. internal/providers/{provider_name}/ ディレクトリを作成
2. providers.Provider インターフェースを実装
3. internal/app/cli.go の providers.NewRegistry に渡すマップへリソース種別とプロバイダーの生成関数を追加
4. docs/providers/{provider_name}.md に利用説明を作成

プロバイダーの登録内容は Registry の構築時にコピーし、構築後は変更しない。生成関数はリソース定義ごとに呼び出され、プロバイダーを生成する。並行した検索に使用する場合、生成関数自体も並行呼び出しに対応すること。テストではモックを返す生成関数で専用のインスタンスを構築してアプリケーションへ渡す。ElastiCache の API モックは NewProviderWithClients で注入できる。

Prepare は外部アクセスや入力設定の変更を行わず、プロバイダー内部の型付き設定に変換する。変換した設定の独立したコピーを保持する providers.Discovery 関数を返すこと。不正な設定はエラーとし、成功時に nil の関数を返してはいけない。検索処理はこの関数に context.Context を渡して実行する。

プロバイダーの実装と利用説明の例を、以下に示す。

- [internal/providers/elasticache/provider.go](../internal/providers/elasticache/provider.go) - 実装例
- [providers/elasticache.md](providers/elasticache.md) - ドキュメント例
