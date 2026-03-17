package usecase

import "context"
import "gorm.io/gorm"

// Pattern3TransactionManagerUsecase は TransactionManager を介するパターンの実装。
//
// 【パターン3: TransactionManager を介する】
// トランザクション開始の手段を TransactionManager インターフェースに抽象化する。
// ユースケースは「トランザクションが必要」という意図だけを RunInTransaction で表現し、
// Begin / Commit / Rollback の具体的な手順はインフラ層の実装に任せる。
//
// Pattern2 との違い:
//   - usecase が *gorm.DB を直接保持しない（TransactionManager インターフェースを持つ）
//   - GORM 固有の db.Transaction(...) を usecase が知らなくてよい
//   - 将来的に DB ライブラリを変えても usecase のコードは変わらない
//
// メリット:
//   - ユースケースとインフラの境界がより明確になる
//   - 複数ユースケースに同じトランザクション制御を横展開しやすい
//   - Commit / Rollback の呼び忘れが構造的に起きない
//
// デメリット:
//   - TransactionManager の実装（GormTransactionManager）が別ファイルに分かれるため、
//     コード量がやや増える
//   - fn の引数に *gorm.DB が残るため、完全な GORM 非依存にはなっていない
//     （より徹底するなら ctx にトランザクションを埋め込む方法もあるが、複雑さが上がる）
type Pattern3TransactionManagerUsecase struct {
	tm   TransactionManager
	core *CreateArticleCore
}

// NewPattern3TransactionManagerUsecase は TransactionManager 利用版のユースケースを構築して返す。
//
// tm は contracts.go で定義した TransactionManager インターフェースを受け取る。
// 実装は infrastructure/mysql/repositories.go の GormTransactionManager が提供する。
func NewPattern3TransactionManagerUsecase(tm TransactionManager, core *CreateArticleCore) *Pattern3TransactionManagerUsecase {
	return &Pattern3TransactionManagerUsecase{tm: tm, core: core}
}

// Execute は TransactionManager 経由でトランザクションを開始して業務処理を実行する。
//
// RunInTransaction の fn 内でエラーを返すと自動でロールバックされる。
// ユースケースは「何をトランザクション内でやるか」だけを記述すればよい。
func (u *Pattern3TransactionManagerUsecase) Execute(ctx context.Context, input CreateArticleInput) (uint, error) {
	var articleID uint
	// ユースケースは「このクロージャ全体をトランザクション内で実行してほしい」という意図を伝えるだけ。
	// Begin / Commit / Rollback の手順は TransactionManager の実装が担う。
	err := u.tm.RunInTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		id, err := u.core.Execute(ctx, tx, input)
		if err != nil {
			return err
		}
		articleID = id
		return nil
	})
	if err != nil {
		return 0, err
	}
	return articleID, nil
}
