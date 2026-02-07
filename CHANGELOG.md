# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2024-XX-XX

### Changed - 破壊的変更

- **完全な書き換え**: アーキテクチャを刷新し、テンプレートベースの設計に移行
- **CLI インターフェース**: `-check` と `-config` オプションを廃止し、`-meta` オプションに統一
- **設定ファイル形式**: YAML 設定をメタ設定ファイルとテンプレートファイルに分離
- **出力方法**: 標準出力から直接ファイル書き込みに変更

### Added - 新機能

- テンプレートベースの設定生成（Go text/template を使用）
- 複数リソース種別のサポート（拡張可能なプロバイダーアーキテクチャ）
- 1回の実行で複数ファイルの生成が可能
- リソースプロバイダーの登録機構
- メタ設定ファイルのバリデーション
- 詳細なエラーメッセージと進捗表示

### Improved - 改善

- テストカバレッジの向上（config: 100%, renderer: 100%, elasticache: 93.7%）
- モジュール構造の改善（パッケージの明確な分離）
- ドキュメントの充実（README、MIGRATION.md）

### Fixed - 修正

なし

### Removed - 削除

- `checks/redisdb/` パッケージを完全削除
- 環境変数からの設定読み込み機能（今後のバージョンで再実装予定）
- 標準出力への設定出力機能

### Migration

v0.1.x からの移行については [MIGRATION.md](MIGRATION.md) を参照してください。

## [0.1.x] - Previous Releases

### 機能

- ElastiCache for Redis の設定生成
- AWS タグによるリソースフィルタリング
- タグマッピング機能
- 設定ファイルまたは環境変数からの設定読み込み
