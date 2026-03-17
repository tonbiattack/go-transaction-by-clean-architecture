package usecase

import (
	"context"

	"gorm.io/gorm"
)

// Pattern1HandlerLedUsecase はハンドラー主導トランザクションパターンのユースケース。
//
// 【パターン1: ハンドラー主導】
// トランザクションの Begin / Commit / Rollback をすべて呼び出し側（Gin ハンドラー）が担当する。
// このユースケース自体はトランザクション制御を一切持たず、渡された tx を使って業務処理を実行するだけ。
//
// メリット:
//   - コードが単純で、GORM に慣れていれば理解しやすい
//   - 既存コードのリファクタ前の状態として説明しやすい
//
// デメリット:
//   - ハンドラーにトランザクション制御という責務が混入する
//   - Rollback の呼び忘れリスクが呼び出し側に残る
//   - CLI やバッチから同じユースケースを呼ぶ場合、呼び出し側ごとに Begin/Commit/Rollback を書く必要がある
//   - 他パターン（Pattern2〜4）との対比として「改善前の形」として位置づける
type Pattern1HandlerLedUsecase struct {
	core *CreateArticleCore
}

// NewPattern1HandlerLedUsecase はハンドラー主導パターンのユースケースを構築して返す。
func NewPattern1HandlerLedUsecase(core *CreateArticleCore) *Pattern1HandlerLedUsecase {
	return &Pattern1HandlerLedUsecase{core: core}
}

// Execute は呼び出し元が開始したトランザクション tx を受け取り、業務処理を実行する。
//
// このメソッドは Begin / Commit / Rollback を呼ばない。
// それらは呼び出し元（CreatePattern1 ハンドラー）が責任を持つ。
//
// エラーを返した場合、呼び出し元は tx.Rollback() を呼ぶ必要がある。
// 呼び忘れるとトランザクションが宙ぶらりんになるため、Pattern2〜4 では
// この問題を構造的に解決している。
func (u *Pattern1HandlerLedUsecase) Execute(ctx context.Context, tx *gorm.DB, input CreateArticleInput) (uint, error) {
	// トランザクション内の業務処理は CreateArticleCore に委譲する。
	// Begin/Commit/Rollback は呼び出し側（ハンドラー）の責任であり、このメソッドは関与しない。
	return u.core.Execute(ctx, tx, input)
}
