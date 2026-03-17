// Package domain はシステムの中核となるエンティティを定義する。
//
// クリーンアーキテクチャにおける最内層であり、フレームワークやDBへの依存を持たない。
// このパッケージのコードは infrastructure や interface に依存してはならない。
//
// このサンプルでは「記事作成」ユースケースを題材に、
// articles / article_contents / article_histories の 3 テーブルへ
// 1 トランザクションで書き込む設計を示している。
package domain

import "time"

// Article は記事の基本情報を表すエンティティ。
//
// 記事のタイトルとステータスを保持する親テーブルに対応する。
// 本文は ArticleContent に分離しており、Article 単体では本文を持たない。
//
// UpdateAt は GORM の autoUpdateTime タグを使わず手動でセットする。
// これは全テーブルで同一時刻を保証するため、usecase 層で now を生成して渡す設計による。
type Article struct {
	ID        uint      `gorm:"primaryKey"`
	Title     string    `gorm:"size:255;not null"`
	StatusID  uint      `gorm:"not null;index"` // article_statuses への外部キー
	CreatedAt time.Time `gorm:"not null"`
	UpdateAt  time.Time `gorm:"not null"`
}

// TableName は GORM がテーブル名を推定する際の自動変換を上書きする。
// GORM のデフォルトは構造体名を複数形にするが、命名規則を明示的に固定したい場合に使う。
func (Article) TableName() string {
	return "articles"
}

// ArticleContent は記事の本文を表すエンティティ。
//
// 本文は Article と 1:1 の関係で別テーブルに分離している。
// 理由は、一覧表示など本文が不要な場面で不必要なデータ転送を避けるため。
// ArticleID で articles テーブルと紐付ける。
type ArticleContent struct {
	ID        uint      `gorm:"primaryKey"`
	ArticleID uint      `gorm:"not null;index"` // articles.id への外部キー
	Body      string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdateAt  time.Time `gorm:"not null"`
}

// TableName は article_contents テーブル名を明示する。
func (ArticleContent) TableName() string {
	return "article_contents"
}

// ArticleHistory は記事に対して発生したイベントの履歴を表すエンティティ。
//
// 「いつ・何が起きたか」を記録するための追記専用テーブルであり、
// 更新が発生しないため UpdateAt は持たない。
// CLAUDE.md の「履歴系テーブルは update_at を付与しない」方針に従っている。
type ArticleHistory struct {
	ID        uint      `gorm:"primaryKey"`
	ArticleID uint      `gorm:"not null;index"` // articles.id への外部キー
	Event     string    `gorm:"size:255;not null"` // 例: "created", "published"
	CreatedAt time.Time `gorm:"not null"`
}

// TableName は article_histories テーブル名を明示する。
func (ArticleHistory) TableName() string {
	return "article_histories"
}

// ArticleStatus は記事のステータスを管理するマスタエンティティ。
//
// CLAUDE.md の「status は CHECK 制約ではなくマスタテーブルで管理する」方針に従い、
// 有効なステータス値をこのテーブルで定義し、Article から外部キー参照する。
// 初期データは infrastructure/mysql/migrate.go の seedStatuses で投入する。
type ArticleStatus struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"size:100;not null;uniqueIndex"` // 例: "draft", "published"
	CreatedAt time.Time `gorm:"not null"`
	UpdateAt  time.Time `gorm:"not null"`
}

// TableName は article_statuses テーブル名を明示する。
func (ArticleStatus) TableName() string {
	return "article_statuses"
}
