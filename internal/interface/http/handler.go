package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/usecase"
	"gorm.io/gorm"
)

// CreateArticleRequest は記事作成 API のリクエスト JSON を表す。
//
// JSON タグにより ShouldBindJSON で自動的にフィールドへマッピングされる。
// usecase.CreateArticleInput とは意図的に分離している。
// ハンドラーは HTTP の関心事（JSON 構造・バリデーション）に責務を限定し、
// usecase は HTTP を知らない独立した入力型（CreateArticleInput）を使う。
type CreateArticleRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	StatusID uint   `json:"status_id"`
}

// ArticleHandler はトランザクションパターン比較用のハンドラー。
//
// Pattern1〜4 の 4 つのユースケースをフィールドとして保持し、
// それぞれ対応するエンドポイントハンドラーメソッドで呼び出す。
//
// db フィールドは Pattern1 専用。
// Pattern1 はハンドラーが Begin/Commit/Rollback を担うため、ハンドラー層で db を持つ必要がある。
// Pattern2〜4 はユースケース内でトランザクションを管理するため、ハンドラーは db を使わない。
type ArticleHandler struct {
	db       *gorm.DB // Pattern1 のトランザクション開始用（Pattern2〜4 では使用しない）
	pattern1 *usecase.Pattern1HandlerLedUsecase
	pattern2 *usecase.Pattern2UsecaseLedUsecase
	pattern3 *usecase.Pattern3TransactionManagerUsecase
	pattern4 *usecase.Pattern4UnitOfWorkUsecase
}

// NewArticleHandler は 4 パターンのユースケースを受け取りハンドラーを構築する。
//
// db は Pattern1 のトランザクション開始に使う。
// Pattern2〜4 は usecase 内でトランザクションを管理するため、引数に db を持たない Execute を呼ぶ。
func NewArticleHandler(
	db *gorm.DB,
	pattern1 *usecase.Pattern1HandlerLedUsecase,
	pattern2 *usecase.Pattern2UsecaseLedUsecase,
	pattern3 *usecase.Pattern3TransactionManagerUsecase,
	pattern4 *usecase.Pattern4UnitOfWorkUsecase,
) *ArticleHandler {
	return &ArticleHandler{
		db:       db,
		pattern1: pattern1,
		pattern2: pattern2,
		pattern3: pattern3,
		pattern4: pattern4,
	}
}

// CreatePattern1 はハンドラーがトランザクション境界を握るパターンの入口。
//
// 【Pattern1: ハンドラー主導】
// このハンドラーは次の責務をすべて担う:
//  1. JSON デコード（ShouldBindJSON）
//  2. トランザクション開始（db.Begin）
//  3. ユースケース呼び出し（tx を渡す）
//  4. エラー時のロールバック（tx.Rollback）
//  5. 成功時のコミット（tx.Commit）
//  6. レスポンス組み立て
//
// 比較ポイント:
// CreatePattern2〜4 と比べると、このハンドラーだけがトランザクション制御コードを含んでいる。
// 「ハンドラーが膨らみやすい」という問題を説明するための比較対象として機能する。
func (h *ArticleHandler) CreatePattern1(c *gin.Context) {
	var req CreateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不正なリクエストです"})
		return
	}

	// ハンドラー主導では、HTTP 層で入力変換とトランザクション開始を行う。
	// Pattern2〜4 ではこの Begin 呼び出しがハンドラーに存在しない。
	input := usecase.CreateArticleInput{Title: req.Title, Body: req.Body, StatusID: req.StatusID}
	tx := h.db.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}

	id, err := h.pattern1.Execute(c.Request.Context(), tx, input)
	if err != nil {
		// ユースケースがエラーを返した場合、ハンドラーが明示的に Rollback を呼ぶ必要がある。
		// この呼び忘れリスクが Pattern1 の構造的な問題点であり、Pattern2〜4 はこれを解消する。
		_ = tx.Rollback()
		h.handleError(c, err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		// コミット自体が失敗した場合（ネットワーク断など）のロールバック。
		// コミット失敗時は GORM が内部でロールバックを試みるが、明示的に呼ぶことで意図を明確にする。
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"article_id": id, "pattern": "handler-led"})
}

// CreatePattern2 はユースケース側にトランザクションを任せるパターンの入口。
//
// 【Pattern2: ユースケース主導】
// このハンドラーの責務は JSON デコード・ユースケース呼び出し・レスポンス組み立てのみ。
// Begin / Commit / Rollback はユースケース内の db.Transaction が管理するため、
// ハンドラーは一切関知しない。
//
// 比較ポイント:
// CreatePattern1 と比べてトランザクション制御コードがない。
// ハンドラーが薄くなり、責務が「HTTP 入出力」に限定されている。
func (h *ArticleHandler) CreatePattern2(c *gin.Context) {
	var req CreateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不正なリクエストです"})
		return
	}

	input := usecase.CreateArticleInput{Title: req.Title, Body: req.Body, StatusID: req.StatusID}
	id, err := h.pattern2.Execute(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"article_id": id, "pattern": "usecase-led"})
}

// CreatePattern3 は TransactionManager を介するパターンの入口。
//
// 【Pattern3: TransactionManager】
// ハンドラーの構造は Pattern2 と同じ。
// トランザクション制御が TransactionManager の抽象に移っているだけで、
// ハンドラーから見た呼び出し方は変わらない。
//
// 比較ポイント:
// Pattern2 との違いはユースケース内部にある（handler.go では差が見えない）。
// ユースケース層の pattern2 vs pattern3 を比較するとトランザクション抽象化の差がわかる。
func (h *ArticleHandler) CreatePattern3(c *gin.Context) {
	var req CreateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不正なリクエストです"})
		return
	}

	input := usecase.CreateArticleInput{Title: req.Title, Body: req.Body, StatusID: req.StatusID}
	id, err := h.pattern3.Execute(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"article_id": id, "pattern": "transaction-manager"})
}

// CreatePattern4 は Unit of Work パターンの入口。
//
// 【Pattern4: Unit of Work】
// ハンドラーの構造は Pattern2/3 と同じ。
// リポジトリ管理が UnitOfWork に移っているが、ハンドラーから見た呼び出し方は変わらない。
//
// 比較ポイント:
// Pattern3 との違いはユースケース内部にある（handler.go では差が見えない）。
// ユースケース層の pattern3 vs pattern4 を比較するとリポジトリ管理の差がわかる。
func (h *ArticleHandler) CreatePattern4(c *gin.Context) {
	var req CreateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不正なリクエストです"})
		return
	}

	input := usecase.CreateArticleInput{Title: req.Title, Body: req.Body, StatusID: req.StatusID}
	id, err := h.pattern4.Execute(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"article_id": id, "pattern": "unit-of-work"})
}

// handleError はユースケースのエラーを HTTP ステータスコードへ変換する。
//
// errors.Is を使うことで、エラーをラップした場合でも正しく判定できる。
// 新しいユースケースエラーを追加した場合はここに case を追加する。
//
// エラーマッピング:
//   - ErrInvalidInput    → 400 Bad Request（入力値の問題はクライアント起因）
//   - ErrStatusNotFound  → 400 Bad Request（存在しないマスタ値を指定した）
//   - その他             → 500 Internal Server Error（予期しないエラー）
func (h *ArticleHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, usecase.ErrStatusNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
