package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apphttp "github.com/tonbiattack/private-go-transaction-by-clean-architecture/internal/interface/http"
)

// createArticleResponse は記事作成 API のレスポンス JSON を表す。
//
// 正常系と異常系の両方で使えるよう、Error フィールドも持つ。
// 正常系では Error が空文字、異常系では ArticleID が 0 になる。
type createArticleResponse struct {
	ArticleID uint   `json:"article_id"`
	Pattern   string `json:"pattern"`
	Error     string `json:"error"`
}

// TestCreateArticleEndpoints は HTTP 入口からDB反映まで通しで確認する E2E テスト（正常系）。
//
// テストの範囲:
//   - HTTP リクエスト → Gin ハンドラー → ユースケース → リポジトリ → MySQL
//
// httptest.NewRequest と httptest.NewRecorder を使うことで、
// 実際のサーバーを起動せずに Gin のルーティングと全配線を通してテストできる。
//
// 各パターンで以下を確認する:
//   - HTTP ステータスが 201 Created であること
//   - レスポンスの article_id が 0 でないこと（採番されたこと）
//   - レスポンスの pattern タグが期待値と一致すること
//   - 3 テーブル（articles / article_contents / article_histories）に各 1 件存在すること
func TestCreateArticleEndpoints(t *testing.T) {
	t.Run("各パターンでHTTP経由の記事作成に成功する", func(t *testing.T) {
		db := setupTestDB(t)

		tests := []struct {
			name           string
			path           string
			expectedTag    string // レスポンス JSON の "pattern" フィールド期待値
			expectedStatus int
		}{
			{name: "パターン1", path: "/v1/pattern1/articles", expectedTag: "handler-led", expectedStatus: http.StatusCreated},
			{name: "パターン2", path: "/v1/pattern2/articles", expectedTag: "usecase-led", expectedStatus: http.StatusCreated},
			{name: "パターン3", path: "/v1/pattern3/articles", expectedTag: "transaction-manager", expectedStatus: http.StatusCreated},
			{name: "パターン4", path: "/v1/pattern4/articles", expectedTag: "unit-of-work", expectedStatus: http.StatusCreated},
		}

		// NewApp は本番の main.go と同じ DI 組み立てを行う。
		// テストと本番で同じ配線を使うことで、設定の乖離が起きにくい。
		router := apphttp.NewApp(db)

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// 各パターンの独立性を保つため、毎回テーブルを初期化する。
				cleanupTables(t, db)

				body, err := json.Marshal(map[string]any{
					"title":     tt.name,
					"body":      "http body",
					"status_id": 1,
				})
				if err != nil {
					t.Fatalf("failed to marshal request: %v", err)
				}

				req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				// ServeHTTP はルーターからハンドラー、ユースケース、リポジトリまで一気通しで実行する。
				router.ServeHTTP(rec, req)

				if rec.Code != tt.expectedStatus {
					t.Fatalf("status = %d, want %d", rec.Code, tt.expectedStatus)
				}

				var res createArticleResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}

				if res.ArticleID == 0 {
					t.Fatalf("article id should be set")
				}
				// pattern フィールドでどのユースケースが実行されたかを確認する。
				if res.Pattern != tt.expectedTag {
					t.Fatalf("pattern = %s, want %s", res.Pattern, tt.expectedTag)
				}
				// 3 テーブル更新のサンプルなので、すべてに反映されたことを確認する。
				if got := countArticles(t, db); got != 1 {
					t.Fatalf("article count = %d, want 1", got)
				}
				if got := countContents(t, db); got != 1 {
					t.Fatalf("content count = %d, want 1", got)
				}
				if got := countHistories(t, db); got != 1 {
					t.Fatalf("history count = %d, want 1", got)
				}
			})
		}
	})
}

// TestCreateArticleEndpointsRollback は HTTP 経由での異常系（ロールバック）を検証する E2E テスト。
//
// status_id=9999 は存在しないマスタ値のため ErrStatusNotFound が返り、
// 各パターンのトランザクション管理によってロールバックされる。
//
// 確認ポイント:
//   - HTTP ステータスが 400 Bad Request であること
//   - レスポンスの error フィールドが空でないこと
//   - articles / article_contents / article_histories がすべて 0 件であること（途中データが残らない）
//
// このテストは「設計の違いに関わらず、どのパターンでもロールバックが正しく機能すること」を示す。
func TestCreateArticleEndpointsRollback(t *testing.T) {
	t.Run("各パターンでHTTP経由の異常系はロールバックされる", func(t *testing.T) {
		db := setupTestDB(t)

		tests := []struct {
			name           string
			path           string
			expectedStatus int
		}{
			{name: "パターン1", path: "/v1/pattern1/articles", expectedStatus: http.StatusBadRequest},
			{name: "パターン2", path: "/v1/pattern2/articles", expectedStatus: http.StatusBadRequest},
			{name: "パターン3", path: "/v1/pattern3/articles", expectedStatus: http.StatusBadRequest},
			{name: "パターン4", path: "/v1/pattern4/articles", expectedStatus: http.StatusBadRequest},
		}

		// 正常系と同じ配線で異常系を流す。
		router := apphttp.NewApp(db)

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cleanupTables(t, db)

				body, err := json.Marshal(map[string]any{
					"title":     tt.name,
					"body":      "http body",
					"status_id": 9999, // 存在しない status → ErrStatusNotFound → 400 Bad Request
				})
				if err != nil {
					t.Fatalf("failed to marshal request: %v", err)
				}

				req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				router.ServeHTTP(rec, req)

				if rec.Code != tt.expectedStatus {
					t.Fatalf("status = %d, want %d", rec.Code, tt.expectedStatus)
				}

				var res createArticleResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if res.Error == "" {
					t.Fatalf("error message should be set")
				}
				// ロールバック確認の本体は「途中成功が 1 件も残っていない」こと。
				// articles の INSERT が成功していても、後続処理でロールバックされていれば 0 件になる。
				if got := countArticles(t, db); got != 0 {
					t.Fatalf("article count = %d, want 0", got)
				}
				if got := countContents(t, db); got != 0 {
					t.Fatalf("content count = %d, want 0", got)
				}
				if got := countHistories(t, db); got != 0 {
					t.Fatalf("history count = %d, want 0", got)
				}
			})
		}
	})
}
