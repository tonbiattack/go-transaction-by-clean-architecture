package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/domain"
	"gorm.io/gorm"
)

var (
	// ErrInvalidInput はタイトル・本文・StatusID のいずれかが不正な場合に返す。
	// ユースケース層で定義することで、ハンドラーが errors.Is で比較して HTTP ステータスへ変換できる。
	ErrInvalidInput = errors.New("入力が不正です")

	// ErrStatusNotFound は指定した StatusID が article_statuses に存在しない場合に返す。
	// DB 制約エラーではなくアプリケーション層で先に検出することで、エラー原因を明確にする。
	ErrStatusNotFound = errors.New("status が存在しません")
)

// CreateArticleCore は記事作成に必要な共通ロジックをまとめた構造体。
//
// Pattern1〜4 のすべてのユースケースから共通利用される。
// トランザクション制御はこの構造体の外側（各パターンのユースケース）が担当し、
// この構造体はトランザクション内で実行される業務処理のみに責務を限定する。
//
// 依存するリポジトリはすべてインターフェース（contracts.go）で受け取るため、
// この構造体は infrastructure の具体実装を知らない。
type CreateArticleCore struct {
	articles ArticleRepository
	contents ArticleContentRepository
	history  ArticleHistoryRepository
	statuses ArticleStatusRepository
}

// NewCreateArticleCore は CreateArticleCore を構築して返す。
//
// 引数はすべてインターフェース型であるため、テスト時も含めて実装の差し替えが可能。
// このサンプルでは infrastructure/mysql の各リポジトリ実装を渡す。
func NewCreateArticleCore(
	articles ArticleRepository,
	contents ArticleContentRepository,
	history ArticleHistoryRepository,
	statuses ArticleStatusRepository,
) *CreateArticleCore {
	return &CreateArticleCore{
		articles: articles,
		contents: contents,
		history:  history,
		statuses: statuses,
	}
}

// Execute はトランザクション内で実行される記事作成の共通処理。
//
// 呼び出し元（各パターンのユースケース）が開始したトランザクション tx を受け取り、
// そのトランザクション配下で 3 テーブルへの書き込みを行う。
//
// 処理の流れ:
//  1. 入力バリデーション（空文字・ゼロ値チェック）
//  2. StatusID の存在確認（マスタ参照）
//  3. articles へ INSERT
//  4. article_contents へ INSERT（articles の採番済み ID を使用）
//  5. article_histories へ INSERT（同上）
//
// いずれかのステップでエラーが発生した場合は即座にエラーを返す。
// ロールバックの実行は呼び出し元のトランザクション管理に委ねる。
func (c *CreateArticleCore) Execute(ctx context.Context, tx *gorm.DB, input CreateArticleInput) (uint, error) {
	// 空文字・ゼロ値は業務上無意味なため、DB アクセスの前に弾く。
	// サンプルでは分かりやすさ優先で簡易チェックに留めている。
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" || input.StatusID == 0 {
		return 0, ErrInvalidInput
	}

	// 存在しない StatusID で articles を INSERT しても DB 制約エラーになるが、
	// アプリケーション層で先に確認することでエラーメッセージを明確にする。
	exists, err := c.statuses.ExistsByID(ctx, tx, input.StatusID)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrStatusNotFound
	}

	// 監査カラム（created_at / update_at）の時刻を統一するため、
	// ここで一度だけ time.Now() を呼び出し、以降の INSERT で使い回す。
	now := time.Now()
	article := &domain.Article{
		Title:     input.Title,
		StatusID:  input.StatusID,
		CreatedAt: now,
		UpdateAt:  now,
	}
	if err := c.articles.Create(ctx, tx, article); err != nil {
		return 0, err
	}

	// GORM の Create は INSERT 後に自動採番された ID を article.ID へセットする。
	// 子テーブルはこの ID を外部キーとして使う。
	content := &domain.ArticleContent{
		ArticleID: article.ID,
		Body:      input.Body,
		CreatedAt: now,
		UpdateAt:  now,
	}
	if err := c.contents.Create(ctx, tx, content); err != nil {
		return 0, err
	}

	// 履歴テーブルには「何が起きたか」だけを記録する。
	// Event の値は将来的に定数化してもよいが、サンプルではリテラルで留める。
	history := &domain.ArticleHistory{
		ArticleID: article.ID,
		Event:     "created",
		CreatedAt: now,
	}
	if err := c.history.Create(ctx, tx, history); err != nil {
		return 0, err
	}

	// 採番された記事 ID を呼び出し元へ返す。
	// ハンドラーはこの ID をレスポンス JSON に含める。
	return article.ID, nil
}
