package usecase

import "context"

import "gorm.io/gorm"

// Pattern2UsecaseLedUsecase はユースケース主導トランザクションパターンの実装。
//
// 【パターン2: ユースケース主導】
// トランザクション境界をユースケース自身が管理する。
// GORM 標準の db.Transaction(func(tx) error) を直接使う最もシンプルな実装。
//
// Pattern1 との違い:
//   - ハンドラーは Begin / Commit / Rollback を知らなくてよい
//   - ハンドラーは Execute を呼ぶだけで、エラー判定とレスポンス組み立てに専念できる
//
// メリット:
//   - コードが素直で追いやすい
//   - ハンドラーの責務が「入力受け取り・出力整形」に限定される
//   - 1ユースケース = 1トランザクションの対応が自然な業務処理に向いている
//
// デメリット:
//   - usecase が *gorm.DB を直接保持するため、GORM への依存が残る
//   - テスト時は実 DB を使う（このプロジェクトの古典派方針では問題なし）
//   - 複数ユースケースにまたがる共通トランザクション制御には向かない
//     （その場合は Pattern3 の TransactionManager が適している）
type Pattern2UsecaseLedUsecase struct {
	db   *gorm.DB
	core *CreateArticleCore
}

// NewPattern2UsecaseLedUsecase はユースケース主導パターンを構築して返す。
//
// db は GORM のコネクションを受け取る。
// このパターンでは usecase が直接 db を保持するため、DI コンテナ（app.go）から渡す。
func NewPattern2UsecaseLedUsecase(db *gorm.DB, core *CreateArticleCore) *Pattern2UsecaseLedUsecase {
	return &Pattern2UsecaseLedUsecase{db: db, core: core}
}

// Execute はトランザクション境界を持って業務処理を実行する。
//
// db.Transaction は内部で Begin を呼び出し、fn が nil を返せばコミット、
// エラーを返せば自動でロールバックする。呼び出し元は Rollback を意識しなくてよい。
//
// articleID はクロージャの外で使うために宣言し、fn の中でセットする。
// Go のクロージャでは外側のスコープの変数を直接操作できるため、この方法で戻り値を受け渡す。
func (u *Pattern2UsecaseLedUsecase) Execute(ctx context.Context, input CreateArticleInput) (uint, error) {
	var articleID uint
	// GORM 標準の Transaction ヘルパーを使うことで、
	// Begin / Commit / Rollback の実行順序を GORM に任せられる。
	err := u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, err := u.core.Execute(ctx, tx, input)
		if err != nil {
			// エラーを返すと GORM が自動でロールバックする。
			return err
		}
		// クロージャの外で返すために採番済み ID を保持する。
		articleID = id
		return nil
	})
	if err != nil {
		return 0, err
	}
	return articleID, nil
}
