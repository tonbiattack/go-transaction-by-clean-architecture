// Package usecase はアプリケーション固有のユースケースを実装する。
//
// クリーンアーキテクチャにおける「ユースケース層」に相当し、
// domain エンティティを操作して業務処理を実現する。
//
// このパッケージの責務:
//   - ビジネスルールの実行（入力検証、マスタ参照、複数テーブル更新）
//   - トランザクション境界の宣言（パターンによって方法が異なる）
//   - リポジトリインターフェースの定義（contracts.go）
//
// このパッケージが依存してよいのは domain パッケージのみ。
// infrastructure や interface/http に依存してはならない。
//
// サンプルとして 4 つのトランザクションパターンを実装している。
//   - Pattern1: ハンドラー主導（呼び出し側が Begin/Commit/Rollback を管理）
//   - Pattern2: ユースケース主導（GORM の db.Transaction を直接使う）
//   - Pattern3: TransactionManager を介する（トランザクション開始を抽象に隠す）
//   - Pattern4: Unit of Work（リポジトリ群をまとめてトランザクションに渡す）
package usecase

// CreateArticleInput は記事作成ユースケースへの入力値を表す。
//
// HTTP リクエストの JSON 構造や GORM のモデルとは切り離した独立した型として定義する。
// これにより、ユースケースは入力の出所（HTTP / CLI / バッチ）を問わず再利用できる。
//
// フィールド:
//   - Title: 記事タイトル。空文字は不正。
//   - Body: 記事本文。空文字は不正。
//   - StatusID: article_statuses.id への参照。存在しない値はユースケース内で弾く。
type CreateArticleInput struct {
	Title    string
	Body     string
	StatusID uint
}
