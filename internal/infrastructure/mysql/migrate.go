package mysql

import (
	"errors"
	"time"

	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/domain"
	"gorm.io/gorm"
)

// Migrate は必要なテーブルの作成と初期データの投入をまとめて実行する。
//
// main.go の起動時と、テストの setupTestDB から呼び出される。
// AutoMigrate はテーブルが存在しない場合に CREATE TABLE し、
// 既存テーブルのカラム追加・型変更には対応するが、カラム削除は行わない。
//
// テーブルの作成順序:
//  1. article_statuses（外部キーの参照先マスタを先に作る）
//  2. articles（article_statuses を外部キー参照）
//  3. article_contents（articles を外部キー参照）
//  4. article_histories（articles を外部キー参照）
func Migrate(db *gorm.DB) error {
	// 依存関係の順に AutoMigrate へ渡す。
	// GORM は渡した順に CREATE TABLE を実行するため、外部キー制約を満たす順序が重要。
	if err := db.AutoMigrate(
		&domain.ArticleStatus{},
		&domain.Article{},
		&domain.ArticleContent{},
		&domain.ArticleHistory{},
	); err != nil {
		return err
	}

	// テーブル作成後に初期マスタデータを投入する。
	if err := seedStatuses(db); err != nil {
		return err
	}

	return nil
}

// seedStatuses は article_statuses テーブルに初期マスタデータを投入する。
//
// 投入するデータ:
//   - ID=1: "draft"（下書き）
//   - ID=2: "published"（公開）
//
// FirstOrCreate を使って冪等に投入するため、
// 何度 Migrate を呼んでも重複挿入によるエラーが起きない。
// テスト環境でも毎回 Migrate を呼ぶため、冪等性は重要。
func seedStatuses(db *gorm.DB) error {
	now := time.Now()
	statuses := []domain.ArticleStatus{
		{ID: 1, Name: "draft", CreatedAt: now, UpdateAt: now},
		{ID: 2, Name: "published", CreatedAt: now, UpdateAt: now},
	}

	for _, status := range statuses {
		// 既存レコードがあれば SELECT のみ、なければ INSERT する。
		// これにより Migrate の多重呼び出しが安全になる。
		if err := db.Where("id = ?", status.ID).FirstOrCreate(&status).Error; err != nil {
			return err
		}
	}

	// 投入確認: "draft" が 1 件も存在しない場合はシード失敗と判定する。
	// AutoMigrate と seed の間に問題があった場合にここで気づける。
	var count int64
	if err := db.Model(&domain.ArticleStatus{}).Where("name = ?", "draft").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("status seed failed")
	}
	return nil
}
