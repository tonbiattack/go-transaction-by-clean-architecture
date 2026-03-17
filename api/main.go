// Package main はアプリケーションのエントリーポイント。
//
// main.go の責務はアプリケーション起動の 3 ステップに限定する。
//  1. DB 接続の確立
//  2. マイグレーションの実行
//  3. HTTP サーバーの起動
//
// ビジネスロジックや DI 組み立ては usecase・interface/http パッケージに委譲する。
// これにより main.go は起動シーケンスのみを表現する薄いファイルに保たれる。
package main

import (
	"log"
	"strconv"

	"github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/infrastructure/mysql"
	handler "github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/interface/http"
)

func main() {
	// ステップ1: DB 接続の確立。
	// 接続情報は環境変数から読み込む（mysql.Open 参照）。
	// 接続に失敗した場合は起動を中断する。
	db, err := mysql.Open()
	if err != nil {
		log.Fatalf("db connection failed: %v", err)
	}

	// ステップ2: マイグレーションの実行。
	// AutoMigrate でテーブルを作成し、seedStatuses でマスタデータを投入する。
	// サンプルプロジェクトのためクローン直後にそのまま起動できるよう起動時実行にしている。
	// 本番では別コマンド（migrate コマンドなど）で管理するのが一般的。
	if err := mysql.Migrate(db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	// ステップ3: HTTP サーバーの起動。
	// DI の組み立てと Gin ルーターの構築は handler.NewApp に委譲する。
	// ポート番号は環境変数 HTTP_PORT で指定（デフォルト: 8080）。
	r := handler.NewApp(db)
	if err := r.Run(":" + strconv.Itoa(mysql.HTTPPort())); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
