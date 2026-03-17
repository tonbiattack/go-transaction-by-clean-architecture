// Package integration はユースケース単位の統合テストをまとめる。
//
// テスト方針（古典派）:
//   - 実 MySQL に接続してテストを行う（DBモック不使用）
//   - 各テスト前に cleanupTables でデータをクリアし、テスト間の独立性を保つ
//   - 状態検証中心（count* 関数でテーブル件数を直接確認する）
//
// このファイルのテスト対象:
//   - Pattern1〜4 のユースケースを直接呼び出して正常系・異常系を検証する
//   - HTTP 層を通さないため、ユースケースとリポジトリの組み合わせだけを確認できる
//   - HTTP 経由の確認は article_http_e2e_test.go が担当する
//
// テスト実行方法:
//
//	go test ./test/integration/...
//	TEST_MYSQL_DSN="user:pass@tcp(host:port)/db?parseTime=true" go test ./test/integration/...
package integration

import (
	"context"
	"os"
	"testing"

	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/domain"
	infra "github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/infrastructure/mysql"
	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/usecase"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// setupTestDB はテスト用 MySQL へ接続し、マイグレーションを実行して *gorm.DB を返す。
//
// 接続先は環境変数 TEST_MYSQL_DSN で指定できる。
// 未設定の場合は docker-compose.yml で定義したデフォルト値を使う。
//
// t.Helper() を使うことで、この関数内でエラーが発生した場合に
// テストの失敗行が setupTestDB ではなく呼び出し元の行として表示される。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		// docker-compose.yml で定義したデフォルト接続先。
		// CI 環境では TEST_MYSQL_DSN を設定して差し替える。
		dsn = "appuser:apppass@tcp(127.0.0.1:3307)/appdb?parseTime=true&multiStatements=true"
	}

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}

	// テーブル作成とマスタデータ投入を行う。
	// Migrate は冪等なため複数テストから呼んでも問題ない。
	if err := infra.Migrate(db); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}

	return db
}

// cleanupTables は各テストの前にすべての記事関連テーブルを空にする。
//
// 削除順序は外部キー制約に従い、子テーブルから先に削除する。
//   1. article_histories（articles を参照）
//   2. article_contents（articles を参照）
//   3. articles
//
// article_statuses はマスタデータなので削除しない。
func cleanupTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("DELETE FROM article_histories").Error; err != nil {
		t.Fatalf("failed to cleanup article_histories: %v", err)
	}
	if err := db.Exec("DELETE FROM article_contents").Error; err != nil {
		t.Fatalf("failed to cleanup article_contents: %v", err)
	}
	if err := db.Exec("DELETE FROM articles").Error; err != nil {
		t.Fatalf("failed to cleanup articles: %v", err)
	}
}

// countArticles は articles テーブルの件数を返すテストヘルパー。
func countArticles(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&domain.Article{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count articles: %v", err)
	}
	return count
}

// countContents は article_contents テーブルの件数を返すテストヘルパー。
func countContents(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&domain.ArticleContent{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count contents: %v", err)
	}
	return count
}

// countHistories は article_histories テーブルの件数を返すテストヘルパー。
func countHistories(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&domain.ArticleHistory{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count histories: %v", err)
	}
	return count
}

// TestPattern1HandlerLedUsecase はハンドラー主導パターンの正常系（コミット）を検証する。
//
// このテストでは、呼び出し側（テストコード）が Begin / Commit / Rollback を管理する。
// これが Pattern1 の実際の使い方であり、ハンドラーでの使い方と同等の構造になっている。
func TestPattern1HandlerLedUsecase(t *testing.T) {
	t.Run("ハンドラー主導でコミットできる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		core := usecase.NewCreateArticleCore(
			&infra.ArticleRepository{},
			&infra.ArticleContentRepository{},
			&infra.ArticleHistoryRepository{},
			&infra.ArticleStatusRepository{},
		)
		uc := usecase.NewPattern1HandlerLedUsecase(core)

		// Pattern1 はテストコード（＝呼び出し側）が Begin を呼ぶ。
		tx := db.Begin()
		id, err := uc.Execute(context.Background(), tx, usecase.CreateArticleInput{
			Title:    "pattern1",
			Body:     "body",
			StatusID: 1,
		})
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("execute failed: %v", err)
		}
		if err := tx.Commit().Error; err != nil {
			t.Fatalf("commit failed: %v", err)
		}

		if id == 0 {
			t.Fatalf("article id should be set")
		}
		// 3 テーブルへの書き込みがコミットされていることを確認する。
		if got := countArticles(t, db); got != 1 {
			t.Fatalf("article count = %d, want 1", got)
		}
		if got := countContents(t, db); got != 1 {
			t.Fatalf("content count = %d, want 1", got)
		}
		if got := countHistories(t, db); got != 1 {
			t.Fatalf("history count = %d, want 1", got)
		}
	})
}

// TestPattern2UsecaseLedUsecase はユースケース主導パターンの異常系（ロールバック）を検証する。
//
// StatusID=9999 は存在しないマスタ値であるため ErrStatusNotFound が返り、
// ユースケース内の db.Transaction が自動でロールバックする。
func TestPattern2UsecaseLedUsecase(t *testing.T) {
	t.Run("ユースケース主導でエラー時にロールバックされる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		core := usecase.NewCreateArticleCore(
			&infra.ArticleRepository{},
			&infra.ArticleContentRepository{},
			&infra.ArticleHistoryRepository{},
			&infra.ArticleStatusRepository{},
		)
		uc := usecase.NewPattern2UsecaseLedUsecase(db, core)

		_, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern2",
			Body:     "body",
			StatusID: 9999, // 存在しない status → ErrStatusNotFound
		})
		if err == nil {
			t.Fatalf("expected error")
		}
		// ロールバック確認: 全テーブルが 0 件であること。
		if got := countArticles(t, db); got != 0 {
			t.Fatalf("article count = %d, want 0", got)
		}
		if got := countContents(t, db); got != 0 {
			t.Fatalf("content count = %d, want 0", got)
		}
		if got := countHistories(t, db); got != 0 {
			t.Fatalf("history count = %d, want 0", got)
		}
	})
}

// TestPattern3TransactionManagerUsecase は TransactionManager パターンの正常系（コミット）を検証する。
//
// TransactionManager 経由でトランザクションを開始し、3 テーブルへの書き込みが
// 正常にコミットされることを確認する。
func TestPattern3TransactionManagerUsecase(t *testing.T) {
	t.Run("TransactionManagerで複数テーブル更新できる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		core := usecase.NewCreateArticleCore(
			&infra.ArticleRepository{},
			&infra.ArticleContentRepository{},
			&infra.ArticleHistoryRepository{},
			&infra.ArticleStatusRepository{},
		)
		tm := infra.NewGormTransactionManager(db)
		uc := usecase.NewPattern3TransactionManagerUsecase(tm, core)

		_, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern3",
			Body:     "body",
			StatusID: 1,
		})
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
		if got := countArticles(t, db); got != 1 {
			t.Fatalf("article count = %d, want 1", got)
		}
		if got := countContents(t, db); got != 1 {
			t.Fatalf("content count = %d, want 1", got)
		}
		if got := countHistories(t, db); got != 1 {
			t.Fatalf("history count = %d, want 1", got)
		}
	})
}

// TestPattern4UnitOfWorkUsecase は UnitOfWork パターンの正常系（articles のみ）を検証する。
//
// 全テーブルの確認は TestPattern4UnitOfWorkUsecase_AllTablesCommit で行う。
func TestPattern4UnitOfWorkUsecase(t *testing.T) {
	t.Run("UnitOfWorkでリポジトリ束ねて更新できる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		tm := infra.NewGormTransactionManager(db)
		repoSet := usecase.TxRepositorySet{
			Articles: &infra.ArticleRepository{},
			Contents: &infra.ArticleContentRepository{},
			History:  &infra.ArticleHistoryRepository{},
			Statuses: &infra.ArticleStatusRepository{},
		}
		uow := infra.NewGormUnitOfWork(tm, repoSet)
		uc := usecase.NewPattern4UnitOfWorkUsecase(uow)

		_, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern4",
			Body:     "body",
			StatusID: 1,
		})
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}

		if got := countArticles(t, db); got != 1 {
			t.Fatalf("article count = %d, want 1", got)
		}
	})
}

// TestPattern1HandlerLedUsecase_Rollback はハンドラー主導パターンの異常系（ロールバック）を検証する。
//
// Pattern1 では呼び出し側が明示的に Rollback を呼ぶ必要がある。
// このテストは「Rollback を呼んだ後に全テーブルが 0 件になること」を確認する。
// つまり「Rollback が正しく機能すること」と「途中で articles に INSERT されていないこと」を証明する。
//
// 比較ポイント:
// Pattern3/4 のロールバックテストと並べると、
// Pattern1 だけが「呼び出し側が Rollback を明示的に呼ぶ必要がある」ことがわかる。
func TestPattern1HandlerLedUsecase_Rollback(t *testing.T) {
	t.Run("ハンドラー主導でエラー時にロールバックすると全テーブルが0件になる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		core := usecase.NewCreateArticleCore(
			&infra.ArticleRepository{},
			&infra.ArticleContentRepository{},
			&infra.ArticleHistoryRepository{},
			&infra.ArticleStatusRepository{},
		)
		uc := usecase.NewPattern1HandlerLedUsecase(core)

		tx := db.Begin()
		_, err := uc.Execute(context.Background(), tx, usecase.CreateArticleInput{
			Title:    "pattern1-rollback",
			Body:     "body",
			StatusID: 9999, // 存在しない status → ErrStatusNotFound
		})
		if err == nil {
			_ = tx.Commit()
			t.Fatalf("エラーが発生するはずです")
		}
		// Pattern1 では呼び出し側（このテストコード）が Rollback を呼ぶ責任を持つ。
		if rbErr := tx.Rollback().Error; rbErr != nil {
			t.Fatalf("rollback failed: %v", rbErr)
		}

		// Rollback 後は全テーブルが 0 件であること。
		if got := countArticles(t, db); got != 0 {
			t.Fatalf("article count = %d, want 0", got)
		}
		if got := countContents(t, db); got != 0 {
			t.Fatalf("content count = %d, want 0", got)
		}
		if got := countHistories(t, db); got != 0 {
			t.Fatalf("history count = %d, want 0", got)
		}
	})
}

// TestPattern2UsecaseLedUsecase_Commit はユースケース主導パターンの正常系（コミット）を検証する。
//
// TestPattern2UsecaseLedUsecase が異常系のみを検証しているため、
// このテストで正常系の動作（3 テーブルへの書き込みと ID 採番）を補完する。
func TestPattern2UsecaseLedUsecase_Commit(t *testing.T) {
	t.Run("ユースケース主導で正常時にコミットされる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		core := usecase.NewCreateArticleCore(
			&infra.ArticleRepository{},
			&infra.ArticleContentRepository{},
			&infra.ArticleHistoryRepository{},
			&infra.ArticleStatusRepository{},
		)
		uc := usecase.NewPattern2UsecaseLedUsecase(db, core)

		id, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern2-commit",
			Body:     "body",
			StatusID: 1,
		})
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
		if id == 0 {
			t.Fatalf("article id should be set")
		}
		// 3 テーブルへの書き込みがコミットされていることを確認する。
		if got := countArticles(t, db); got != 1 {
			t.Fatalf("article count = %d, want 1", got)
		}
		if got := countContents(t, db); got != 1 {
			t.Fatalf("content count = %d, want 1", got)
		}
		if got := countHistories(t, db); got != 1 {
			t.Fatalf("history count = %d, want 1", got)
		}
	})
}

// TestPattern3TransactionManagerUsecase_Rollback は TransactionManager パターンの異常系（ロールバック）を検証する。
//
// RunInTransaction の fn がエラーを返すと GormTransactionManager が自動でロールバックする。
// このテストはその「自動ロールバック」が正しく機能することを確認する。
//
// 比較ポイント:
// Pattern1 のロールバックテストと並べると、
// Pattern3 では呼び出し側が Rollback を書く必要がないことがわかる。
func TestPattern3TransactionManagerUsecase_Rollback(t *testing.T) {
	t.Run("TransactionManagerでエラー時に自動ロールバックされる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		core := usecase.NewCreateArticleCore(
			&infra.ArticleRepository{},
			&infra.ArticleContentRepository{},
			&infra.ArticleHistoryRepository{},
			&infra.ArticleStatusRepository{},
		)
		tm := infra.NewGormTransactionManager(db)
		uc := usecase.NewPattern3TransactionManagerUsecase(tm, core)

		_, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern3-rollback",
			Body:     "body",
			StatusID: 9999,
		})
		if err == nil {
			t.Fatalf("エラーが発生するはずです")
		}
		// ロールバック確認: 全テーブルが 0 件であること。
		if got := countArticles(t, db); got != 0 {
			t.Fatalf("article count = %d, want 0", got)
		}
		if got := countContents(t, db); got != 0 {
			t.Fatalf("content count = %d, want 0", got)
		}
		if got := countHistories(t, db); got != 0 {
			t.Fatalf("history count = %d, want 0", got)
		}
	})
}

// TestPattern4UnitOfWorkUsecase_Rollback は UnitOfWork パターンの異常系（ロールバック）を検証する。
//
// UnitOfWork の Do は内部で TransactionManager に委譲するため、
// エラー時は自動でロールバックされる。
func TestPattern4UnitOfWorkUsecase_Rollback(t *testing.T) {
	t.Run("UnitOfWorkでエラー時にロールバックされる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		tm := infra.NewGormTransactionManager(db)
		repoSet := usecase.TxRepositorySet{
			Articles: &infra.ArticleRepository{},
			Contents: &infra.ArticleContentRepository{},
			History:  &infra.ArticleHistoryRepository{},
			Statuses: &infra.ArticleStatusRepository{},
		}
		uow := infra.NewGormUnitOfWork(tm, repoSet)
		uc := usecase.NewPattern4UnitOfWorkUsecase(uow)

		_, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern4-rollback",
			Body:     "body",
			StatusID: 9999,
		})
		if err == nil {
			t.Fatalf("エラーが発生するはずです")
		}
		// ロールバック確認: 全テーブルが 0 件であること。
		if got := countArticles(t, db); got != 0 {
			t.Fatalf("article count = %d, want 0", got)
		}
		if got := countContents(t, db); got != 0 {
			t.Fatalf("content count = %d, want 0", got)
		}
		if got := countHistories(t, db); got != 0 {
			t.Fatalf("history count = %d, want 0", got)
		}
	})
}

// TestPattern4UnitOfWorkUsecase_AllTablesCommit は UnitOfWork パターンの正常系で全テーブルを検証する。
//
// TestPattern4UnitOfWorkUsecase は articles のみ確認しているため、
// このテストで article_contents と article_histories への書き込みも補完する。
func TestPattern4UnitOfWorkUsecase_AllTablesCommit(t *testing.T) {
	t.Run("UnitOfWorkで正常時に全テーブルへコミットされる", func(t *testing.T) {
		db := setupTestDB(t)
		cleanupTables(t, db)

		tm := infra.NewGormTransactionManager(db)
		repoSet := usecase.TxRepositorySet{
			Articles: &infra.ArticleRepository{},
			Contents: &infra.ArticleContentRepository{},
			History:  &infra.ArticleHistoryRepository{},
			Statuses: &infra.ArticleStatusRepository{},
		}
		uow := infra.NewGormUnitOfWork(tm, repoSet)
		uc := usecase.NewPattern4UnitOfWorkUsecase(uow)

		id, err := uc.Execute(context.Background(), usecase.CreateArticleInput{
			Title:    "pattern4-all-tables",
			Body:     "body",
			StatusID: 1,
		})
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
		if id == 0 {
			t.Fatalf("article id should be set")
		}
		// 3 テーブルすべてへの書き込みを確認する。
		if got := countArticles(t, db); got != 1 {
			t.Fatalf("article count = %d, want 1", got)
		}
		if got := countContents(t, db); got != 1 {
			t.Fatalf("content count = %d, want 1", got)
		}
		if got := countHistories(t, db); got != 1 {
			t.Fatalf("history count = %d, want 1", got)
		}
	})
}

// TestPattern5RawQueryStatusRepository は生クエリによる status 存在判定を検証する（Pattern5）。
//
// ExistsByID は GORM チェインではなく SELECT EXISTS の生 SQL で実装されている。
// このテストはその動作確認であり、「存在する ID（1）が true を返すこと」を確認する。
//
// トランザクションを Begin して ExistsByID を呼び、その後 Rollback している。
// 読み取り専用の確認なので Rollback しても影響はないが、
// 「トランザクション内でも生クエリが動作する」ことの実証としてあえてこの形にしている。
func TestPattern5RawQueryStatusRepository(t *testing.T) {
	t.Run("生クエリでstatus存在判定できる", func(t *testing.T) {
		db := setupTestDB(t)
		repo := &infra.ArticleStatusRepository{}

		// ExistsByID は tx を引数で受け取るため、Begin してから渡す。
		tx := db.Begin()
		exists, err := repo.ExistsByID(context.Background(), tx, 1)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("exists failed: %v", err)
		}
		// 読み取り専用のため Rollback しても問題ない。
		_ = tx.Rollback()

		if !exists {
			t.Fatalf("status should exist")
		}
	})
}
