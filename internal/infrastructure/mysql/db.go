package mysql

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// env は環境変数を取得し、未設定の場合は fallback を返す。
//
// 環境ごとに接続先を切り替えるためのヘルパー関数。
// 本番・開発・テストの各環境で同じ変数名を使い、値だけ差し替えることができる。
func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// Open は環境変数から MySQL 接続情報を読み込み、GORM のコネクションを返す。
//
// 参照する環境変数:
//   - MYSQL_HOST     : 接続先ホスト（デフォルト: 127.0.0.1）
//   - MYSQL_PORT     : ポート番号（デフォルト: 3307）
//   - MYSQL_USER     : ユーザー名（デフォルト: appuser）
//   - MYSQL_PASSWORD : パスワード（デフォルト: apppass）
//   - MYSQL_DATABASE : DB 名（デフォルト: appdb）
//
// .env ファイルが存在する場合は godotenv で自動読み込みする。
// 存在しない場合のエラーは無視し（_ =）、OS 環境変数のフォールバックで動作する。
//
// DSN オプション:
//   - parseTime=true       : time.Time 型へのスキャンを有効にする
//   - multiStatements=true : 複数 SQL をセミコロン区切りで送信できるようにする（マイグレーション用）
func Open() (*gorm.DB, error) {
	_ = godotenv.Load()

	host := env("MYSQL_HOST", "127.0.0.1")
	port := env("MYSQL_PORT", "3307")
	user := env("MYSQL_USER", "appuser")
	password := env("MYSQL_PASSWORD", "apppass")
	dbName := env("MYSQL_DATABASE", "appdb")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true", user, password, host, port, dbName)
	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}

// HTTPPort は HTTP サーバーの待受ポートを返す。
//
// 環境変数 HTTP_PORT が設定されていればその値を使い、未設定またはパース失敗時は 8080 を返す。
// main.go はこの関数を使ってサーバー起動ポートを決定する。
func HTTPPort() int {
	port, err := strconv.Atoi(env("HTTP_PORT", "8080"))
	if err != nil {
		return 8080
	}
	return port
}
