package usecase

import (
	"context"

	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/domain"
	"gorm.io/gorm"
)

// このファイルはユースケース層が必要とするインターフェースをすべて宣言する。
//
// クリーンアーキテクチャの「依存性逆転の原則」に従い、
// インターフェースは「使う側（usecase）」が宣言し、「実装する側（infrastructure）」が満たす。
// これにより usecase は infrastructure を知らなくて済み、依存の方向が外向きに保たれる。
//
// インターフェース設計の方針:
//   - リポジトリ引数に tx *gorm.DB を渡すことで、呼び出し側のトランザクションに参加できる
//   - ctx は将来のタイムアウト制御やトレース伝播のために全メソッドに含める
//   - TransactionManager と UnitOfWork はトランザクション制御の抽象化パターン（Pattern3, 4）

// ArticleRepository は articles テーブルへの書き込みを抽象化するインターフェース。
//
// 実装は infrastructure/mysql/repositories.go の ArticleRepository struct が提供する。
// tx を引数で受け取ることで、呼び出し元が開始したトランザクション内で動作する。
type ArticleRepository interface {
	Create(ctx context.Context, tx *gorm.DB, article *domain.Article) error
}

// ArticleContentRepository は article_contents テーブルへの書き込みを抽象化するインターフェース。
//
// Article と同一トランザクション内で保存するために tx を受け取る。
type ArticleContentRepository interface {
	Create(ctx context.Context, tx *gorm.DB, content *domain.ArticleContent) error
}

// ArticleHistoryRepository は article_histories テーブルへの書き込みを抽象化するインターフェース。
//
// 履歴テーブルへの追記も Article / ArticleContent と同一トランザクション内で行う。
type ArticleHistoryRepository interface {
	Create(ctx context.Context, tx *gorm.DB, history *domain.ArticleHistory) error
}

// ArticleStatusRepository は article_statuses テーブルへの参照を抽象化するインターフェース。
//
// 存在判定のみを提供する読み取り専用インターフェース。
// 実装は生 SQL（SELECT EXISTS）で書かれており、GORM チェインに寄せていない（Pattern5 の例示）。
type ArticleStatusRepository interface {
	// ExistsByID は指定した statusID がマスタに存在するかを返す。
	// トランザクション内で他の書き込みと一緒に参照整合性を確認するために tx を受け取る。
	ExistsByID(ctx context.Context, tx *gorm.DB, statusID uint) (bool, error)
}

// TransactionManager はトランザクション開始・終了の責務を抽象化するインターフェース（Pattern3）。
//
// ユースケースは「トランザクションが必要」という意図だけを RunInTransaction で表現し、
// Begin / Commit / Rollback の具体的な手順は実装側（GormTransactionManager）に任せる。
// これにより、ユースケースが GORM 固有の API を直接呼ばなくて済む。
//
// fn の中でエラーを返すと自動でロールバックされ、nil を返すとコミットされる。
type TransactionManager interface {
	RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error
}

// TxRepositorySet はトランザクション内で使うリポジトリ群を束ねた値型（Pattern4）。
//
// UnitOfWork パターンでは、Do コールバックへリポジトリ集合をまとめて渡す。
// 新しいリポジトリが必要になった場合は、このフィールドを追加して Do の型シグネチャを変えずに拡張できる。
type TxRepositorySet struct {
	Articles ArticleRepository
	Contents ArticleContentRepository
	History  ArticleHistoryRepository
	Statuses ArticleStatusRepository
}

// UnitOfWork はトランザクションとリポジトリ集合をまとめて管理する抽象（Pattern4）。
//
// Do コールバックにトランザクション（tx）とリポジトリ集合（repos）を渡すことで、
// ユースケースが「どのリポジトリをトランザクション配下で使うか」を明示できる。
// fn の中でエラーを返すと自動でロールバックされる（TransactionManager に委譲）。
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB, repos TxRepositorySet) error) error
}
