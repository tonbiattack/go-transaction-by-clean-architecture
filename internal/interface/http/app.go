// Package http はクリーンアーキテクチャの「インターフェース層（Interface Adapters）」を担う。
//
// このパッケージの責務:
//   - HTTP リクエスト/レスポンスの変換（ハンドラー）
//   - ルーティングの定義
//   - DI（依存性注入）の組み立て（app.go）
//
// ハンドラーはビジネスロジックを直接持たない。
// 入力を受け取り usecase を呼び出し、結果を HTTP レスポンスへ変換する責務に限定する。
//
// DI の組み立て方針:
//   - app.go の NewApp が唯一の組み立て場所
//   - main.go と統合テスト（setupTestDB を使うテスト）のどちらからも呼べるよう、
//     引数は *gorm.DB のみにしている
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/infrastructure/mysql"
	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/usecase"
	"gorm.io/gorm"
)

// NewApp はリポジトリ・ユースケース・ハンドラーの依存関係を組み立てて gin.Engine を返す。
//
// クリーンアーキテクチャにおける「コンポジションルート」の役割を担う。
// 依存の組み立て順序:
//  1. インフラ実装（リポジトリ）を生成する
//  2. 共通ロジック（CreateArticleCore）を生成する
//  3. 各パターンのユースケースを生成する（core を共有する）
//  4. ハンドラーへすべてのユースケースを注入する
//  5. ルーターへハンドラーを登録して返す
//
// Pattern3 と Pattern4 は TransactionManager を共有している。
// これにより GormTransactionManager のインスタンスが 1 つに保たれる。
func NewApp(db *gorm.DB) *gin.Engine {
	// --- リポジトリ（インフラ実装）の生成 ---
	// 各リポジトリは空の構造体で、DB コネクションはメソッド引数の tx で受け取る。
	articleRepo := &mysql.ArticleRepository{}
	contentRepo := &mysql.ArticleContentRepository{}
	historyRepo := &mysql.ArticleHistoryRepository{}
	statusRepo := &mysql.ArticleStatusRepository{}

	// --- 共通ロジックの生成 ---
	// CreateArticleCore は Pattern1〜4 すべてで共有する。
	// ロジックの重複を避け、トランザクションパターンの差異だけを各ユースケースで表現する。
	core := usecase.NewCreateArticleCore(articleRepo, contentRepo, historyRepo, statusRepo)

	// --- Pattern1: ハンドラー主導 ---
	// core のみを渡す。Begin/Commit/Rollback はハンドラーが担当するため db は不要。
	pattern1 := usecase.NewPattern1HandlerLedUsecase(core)

	// --- Pattern2: ユースケース主導 ---
	// db.Transaction を内部で呼ぶため、db を渡す必要がある。
	pattern2 := usecase.NewPattern2UsecaseLedUsecase(db, core)

	// --- Pattern3: TransactionManager ---
	// GormTransactionManager は Pattern3 と Pattern4 で再利用する。
	tm := mysql.NewGormTransactionManager(db)
	pattern3 := usecase.NewPattern3TransactionManagerUsecase(tm, core)

	// --- Pattern4: Unit of Work ---
	// TxRepositorySet にトランザクション配下で使うリポジトリをまとめて渡す。
	repoSet := usecase.TxRepositorySet{
		Articles: articleRepo,
		Contents: contentRepo,
		History:  historyRepo,
		Statuses: statusRepo,
	}
	uow := mysql.NewGormUnitOfWork(tm, repoSet)
	pattern4 := usecase.NewPattern4UnitOfWorkUsecase(uow)

	// --- ハンドラーとルーターの組み立て ---
	// すべてのパターンを 1 つのハンドラーに集約し、ルーターへ渡す。
	return NewRouter(NewArticleHandler(db, pattern1, pattern2, pattern3, pattern4))
}
