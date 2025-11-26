// db.go
package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

// アプリ起動時に呼び出す
func initDB() {
	var err error

	// カレントディレクトリに checkin.db というファイルを作る
	db, err = sql.Open("sqlite3", "./checkin.db?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		log.Fatal("DBオープン失敗:", err)
	}

	// テーブル作成（なければ作る）
	schema := `
CREATE TABLE IF NOT EXISTS members (
  line_user_id  TEXT PRIMARY KEY,          -- LINEのユーザーID
  display_name  TEXT,                      -- LINEの表示名（ニックネーム）
  poster_id     TEXT,                      -- 将来使う用
  member_type   TEXT NOT NULL DEFAULT 'general', -- 'general' or '1day'
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS visits (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  line_user_id TEXT NOT NULL,
  visited_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := db.Exec(schema); err != nil {
		log.Fatal("DB初期化失敗:", err)
	}
	log.Println("✅ DB初期化完了")
}
