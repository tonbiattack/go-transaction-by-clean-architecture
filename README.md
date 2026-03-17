# private-go-transaction-by-clean-architecture

Go + GORM + Gin + Clean Architecture で、複数テーブル更新のトランザクション責務を比較するサンプル実装です。

## 実装済みパターン

1. ハンドラー主導 (`/v1/pattern1/articles`)
2. ユースケース主導 (`/v1/pattern2/articles`)
3. TransactionManager (`/v1/pattern3/articles`)
4. Unit of Work (`/v1/pattern4/articles`)
5. 生クエリ存在判定 (`ArticleStatusRepository.ExistsByID`)
6. 実 DB 統合テスト (`test/integration/article_patterns_test.go`)

## 前提

- Docker / Docker Compose
- Go 1.23+

## 起動手順

1. 環境変数を用意

```bash
cp .env.example .env
```

2. MySQL 起動（競合回避でデフォルト 3307）

```bash
docker compose up -d
```

3. API 起動

```bash
go run ./cmd/api
```

## API リクエスト例

```bash
curl -X POST http://localhost:8080/v1/pattern3/articles \
  -H "Content-Type: application/json" \
  -d '{"title":"hello","body":"world","status_id":1}'
```

## テスト

```bash
go test ./... -v
```

`TEST_MYSQL_DSN` が未指定の場合は、次の DSN を使います。

```text
appuser:apppass@tcp(127.0.0.1:3307)/appdb?parseTime=true&multiStatements=true
```