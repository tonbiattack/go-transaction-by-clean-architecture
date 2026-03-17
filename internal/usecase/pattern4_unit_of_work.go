package usecase

import "context"
import "gorm.io/gorm"

// Pattern4UnitOfWorkUsecase は Unit of Work パターンに近い構成の実装。
//
// 【パターン4: Unit of Work】
// トランザクション対象のリポジトリ集合（TxRepositorySet）を UnitOfWork が管理し、
// Do コールバックへトランザクション（tx）とリポジトリ集合（repos）をまとめて渡す。
//
// Pattern3 との違い:
//   - TransactionManager は「トランザクション開始」のみを抽象化する
//   - UnitOfWork は「トランザクション開始」＋「使うリポジトリ群の管理」を抽象化する
//   - ユースケースは「どのリポジトリをトランザクション配下で使うか」を宣言しやすい
//
// メリット:
//   - 複数のリポジトリをトランザクション内でまとめて扱う場面で見通しがよくなる
//   - ユースケース単位でリポジトリ群の組み合わせを明示できる
//
// デメリット:
//   - 今回のような単純な題材では Pattern3 と比べて恩恵が小さい
//   - 抽象化の層が増えるためコード量が増える
//   - Do コールバック内で NewCreateArticleCore を再構築しており、
//     Pattern1〜3 と共通の core インスタンスを使えない構造になっている
//
// どんな場面で向いているか:
//   - 1 トランザクション内で使うリポジトリが多く、毎回引数で渡すのが冗長な場合
//   - 複数のユースケースが同じリポジトリ組み合わせを使い回す場合
type Pattern4UnitOfWorkUsecase struct {
	uow UnitOfWork
}

// NewPattern4UnitOfWorkUsecase は Unit of Work 版のユースケースを構築して返す。
//
// uow は contracts.go で定義した UnitOfWork インターフェースを受け取る。
// 実装は infrastructure/mysql/repositories.go の GormUnitOfWork が提供する。
func NewPattern4UnitOfWorkUsecase(uow UnitOfWork) *Pattern4UnitOfWorkUsecase {
	return &Pattern4UnitOfWorkUsecase{uow: uow}
}

// Execute は UnitOfWork 経由でトランザクションとリポジトリ集合を受け取り業務処理を実行する。
//
// Do コールバックの引数 repos には TxRepositorySet が渡される。
// ユースケースはここから必要なリポジトリを取り出して使う。
//
// repos から NewCreateArticleCore を構築しているのは、
// UnitOfWork がトランザクション配下のリポジトリ実態を管理しているためで、
// 外から渡した core インスタンスではなく、UnitOfWork が提供する repos を使う必要がある。
func (u *Pattern4UnitOfWorkUsecase) Execute(ctx context.Context, input CreateArticleInput) (uint, error) {
	var articleID uint
	err := u.uow.Do(ctx, func(ctx context.Context, tx *gorm.DB, repos TxRepositorySet) error {
		// Unit of Work から渡されたリポジトリ集合を使って共通処理を組み立てる。
		// トランザクション内で使うリポジトリは UnitOfWork が保証するため、
		// ここでは受け取った repos をそのまま利用する。
		core := NewCreateArticleCore(repos.Articles, repos.Contents, repos.History, repos.Statuses)
		id, err := core.Execute(ctx, tx, input)
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
