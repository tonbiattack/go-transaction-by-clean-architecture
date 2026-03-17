// Package mysql は MySQL + GORM を使ったインフラ層の実装をまとめる。
//
// クリーンアーキテクチャにおける「インフラ層（外側）」であり、
// usecase パッケージが宣言したインターフェースをここで実装する。
//
// このパッケージの責務:
//   - リポジトリインターフェースの具体的な DB 操作実装
//   - TransactionManager（Pattern3）の GORM 実装
//   - UnitOfWork（Pattern4）の GORM 実装
//
// 依存の方向:
//   - このパッケージは usecase パッケージのインターフェースを満たす
//   - usecase はこのパッケージを知らない（依存性逆転の原則）
//
// 生クエリの方針:
//   - GORM チェインで不自然・読みにくくなる処理は Raw / Exec を使う
//   - 存在判定（ExistsByID）は SELECT EXISTS を生 SQL で書いている（Pattern5 の例示）
package mysql

import (
	"context"

	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/domain"
	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/usecase"
	"gorm.io/gorm"
)

// ArticleRepository は usecase.ArticleRepository インターフェースの MySQL 実装。
//
// フィールドを持たない空の構造体として定義しており、
// DB コネクションは各メソッドの引数 tx で受け取る。
// これにより、呼び出し元のトランザクションに参加できる。
type ArticleRepository struct{}

// Create は articles テーブルへ 1 件 INSERT する。
//
// tx.Create は INSERT 後に自動採番された ID を article.ID へセットするため、
// 呼び出し元はこの関数の後で article.ID を参照できる。
func (r *ArticleRepository) Create(_ context.Context, tx *gorm.DB, article *domain.Article) error {
	// サンプルのため GORM の最短経路で保存する。
	// 本番では重複チェックや楽観的ロックが必要な場面もある。
	return tx.Create(article).Error
}

// ArticleContentRepository は usecase.ArticleContentRepository インターフェースの MySQL 実装。
type ArticleContentRepository struct{}

// Create は article_contents テーブルへ 1 件 INSERT する。
//
// ArticleRepository.Create と同一トランザクション（tx）を使って保存するため、
// articles と article_contents は必ず同じコミット/ロールバック単位で扱われる。
func (r *ArticleContentRepository) Create(_ context.Context, tx *gorm.DB, content *domain.ArticleContent) error {
	// 本文も同じトランザクションを使って保存する。
	return tx.Create(content).Error
}

// ArticleHistoryRepository は usecase.ArticleHistoryRepository インターフェースの MySQL 実装。
type ArticleHistoryRepository struct{}

// Create は article_histories テーブルへ 1 件 INSERT する。
//
// 履歴テーブルへの書き込みも同一トランザクション配下に置くことで、
// 記事・本文・履歴の 3 テーブルが常にアトミックに書き込まれることを保証する。
func (r *ArticleHistoryRepository) Create(_ context.Context, tx *gorm.DB, history *domain.ArticleHistory) error {
	return tx.Create(history).Error
}

// ArticleStatusRepository は usecase.ArticleStatusRepository インターフェースの MySQL 実装。
type ArticleStatusRepository struct{}

// ExistsByID は指定した statusID が article_statuses テーブルに存在するかを返す。
//
// 【Pattern5: 生クエリ併用の例示】
// GORM チェインで存在判定を書くと .First() + errors.Is(gorm.ErrRecordNotFound) という
// 迂回が必要になり意図が伝わりにくい。
// SELECT EXISTS(SELECT 1 FROM ...) を生 SQL で書く方が一目で意図を追えるため、
// このプロジェクトの「生クエリ優先」方針に従い Raw を使う。
//
// tx を受け取ることで、他の書き込みと同一トランザクション内で参照整合性を確認できる。
func (r *ArticleStatusRepository) ExistsByID(_ context.Context, tx *gorm.DB, statusID uint) (bool, error) {
	var exists bool
	// プレースホルダ（?）を使用しているため SQL インジェクションのリスクはない。
	row := tx.Raw("SELECT EXISTS(SELECT 1 FROM article_statuses WHERE id = ?) AS e", statusID).Row()
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// GormTransactionManager は usecase.TransactionManager インターフェースの GORM 実装（Pattern3）。
//
// ユースケースから GORM 固有の db.Transaction(...) を隠蔽し、
// インターフェース経由で「トランザクション内で処理を実行する」という意図だけを受け取る。
type GormTransactionManager struct {
	db *gorm.DB
}

// NewGormTransactionManager は GORM 実装の TransactionManager を構築して返す。
func NewGormTransactionManager(db *gorm.DB) *GormTransactionManager {
	return &GormTransactionManager{db: db}
}

// RunInTransaction は fn をトランザクション内で実行する。
//
// fn が nil を返せば自動でコミット、エラーを返せば自動でロールバックする。
// Begin / Commit / Rollback の手順は GORM の db.Transaction に委譲しているため、
// 呼び出し元（ユースケース）は制御フローを意識しなくてよい。
func (m *GormTransactionManager) RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error {
	// GORM 固有のトランザクション開始処理はこのメソッド内だけに閉じ込める。
	// usecase 層はこのコードを知らなくてよい。
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, tx)
	})
}

// GormUnitOfWork は usecase.UnitOfWork インターフェースの GORM 実装（Pattern4）。
//
// TransactionManager と TxRepositorySet を束ねて管理する。
// Do コールバックへトランザクション（tx）とリポジトリ集合（repoSet）を同時に渡すことで、
// ユースケースが「何をトランザクション配下で使うか」を明示できる。
type GormUnitOfWork struct {
	tm      usecase.TransactionManager
	repoSet usecase.TxRepositorySet
}

// NewGormUnitOfWork は TransactionManager とリポジトリ集合を受け取り UnitOfWork を構築して返す。
//
// tm は GormTransactionManager を渡すことが多いが、インターフェース型で受け取るため
// テスト時に差し替えることも可能。
func NewGormUnitOfWork(tm usecase.TransactionManager, repoSet usecase.TxRepositorySet) *GormUnitOfWork {
	return &GormUnitOfWork{tm: tm, repoSet: repoSet}
}

// Do はトランザクション内でリポジトリ集合を伴って fn を実行する。
//
// トランザクションの開始・終了は内部の TransactionManager に委譲する。
// fn には tx と repoSet を渡すため、ユースケースはトランザクション管理を意識せず
// 業務処理の記述に集中できる。
func (u *GormUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB, repos usecase.TxRepositorySet) error) error {
	// Unit of Work でも実際のトランザクション開始は TransactionManager へ委譲することで、
	// Begin / Commit / Rollback の実装を一箇所（GormTransactionManager）に集約する。
	return u.tm.RunInTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		return fn(ctx, tx, u.repoSet)
	})
}
