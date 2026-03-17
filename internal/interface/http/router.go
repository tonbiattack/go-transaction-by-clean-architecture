package http

import "github.com/gin-gonic/gin"

// NewRouter はトランザクションパターン比較用のエンドポイントを登録した gin.Engine を返す。
//
// URL 設計の方針:
//   - /v1/pattern{N}/articles の形式でパターン番号を URL に含める
//   - これにより curl やテストコードでどのパターンを呼んでいるかが一目でわかる
//   - 実際のプロダクションでは URL にパターン番号は含めない（比較サンプル用の設計）
//
// エンドポイント一覧:
//   - POST /v1/pattern1/articles : ハンドラー主導パターン
//   - POST /v1/pattern2/articles : ユースケース主導パターン
//   - POST /v1/pattern3/articles : TransactionManager パターン
//   - POST /v1/pattern4/articles : Unit of Work パターン
func NewRouter(handler *ArticleHandler) *gin.Engine {
	r := gin.Default()

	// URL からどのパターンを試しているか分かるように /v1 以下でパターンごとに分ける。
	v1 := r.Group("/v1")
	{
		v1.POST("/pattern1/articles", handler.CreatePattern1)
		v1.POST("/pattern2/articles", handler.CreatePattern2)
		v1.POST("/pattern3/articles", handler.CreatePattern3)
		v1.POST("/pattern4/articles", handler.CreatePattern4)
	}

	return r
}
