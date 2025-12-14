// store.go
package main

import (
	"sync"
	"time"
)

// フロントから来るJSONの形
type checkinRequest struct {
	UserID string `json:"userId"`
	DisplayName  string `json:"displayName"`
}

// チェックインしたときの情報
type checkinInfo struct {
	At time.Time // いつチェックインしたか
}

// メモリ上でチェックインを管理する
var (
	mu             sync.Mutex
	checkedInUsers = make(map[string]checkinInfo)
	maxPeople      = 10			   // Max定員10人（Jsと合わせる）
	expireAfter    = 1.5 * time.Hour // 1.5時間で自動的に無効とみなす
)

// 期限切れの人を消す共通処理
func cleanupExpiredLocked(now time.Time) {
	for id, info := range checkedInUsers {
		if now.Sub(info.At) > expireAfter {
			delete(checkedInUsers, id)
		}
	}
}

// ユーザーを追加して現在の人数を返す
func addCheckin(userID string) int {
	mu.Lock()
	defer mu.Unlock()

	cleanupExpiredLocked(time.Now())

	checkedInUsers[userID] = checkinInfo{At: time.Now()}
	return len(checkedInUsers)
}

// チェックアウトして現在の人数を返す
func removeCheckin(userID string) int {
	mu.Lock()
	defer mu.Unlock()

	delete(checkedInUsers, userID)
	return len(checkedInUsers)
}

// 現在の人数を取得（ついでに期限切れも掃除する）
func getCurrentCount() int {
	mu.Lock()
	defer mu.Unlock()

	cleanupExpiredLocked(time.Now())
	return len(checkedInUsers)
}

// 定員を取得
func getMaxPeople() int {
	return maxPeople
}

// 指定のユーザーがまだ中にいるかどうか
func isCheckedIn(userID string) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	// まず期限切れ掃除
	cleanupExpiredLocked(now)

	_, ok := checkedInUsers[userID]
	return ok
}

// visitを記録する（チェックイン時に呼ぶ）
func recordVisit(lineUserID, displayName string) (err error) {
	if lineUserID == "" {
		return nil
	}

	// 日本時間で「今」
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)
	now := time.Now().In(jst)
	visitedAt := now.Format("2006-01-02 15:04:05") // 例: 2025-11-25T08:22:13+09:00

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// members にユーザーを登録（なければ INSERT、あれば display_name を更新）
	if _, err = tx.Exec(
		`INSERT INTO members(line_user_id, display_name, member_type, created_at)
         VALUES(?, ?, 'general', ?)
         ON CONFLICT(line_user_id)
         DO UPDATE SET display_name = excluded.display_name`,
		lineUserID, displayName, visitedAt,
	); err != nil {
		return err
	}

	// visits に1件挿入（paid は 0）
	if _, err = tx.Exec(
		`INSERT INTO visits(line_user_id, visited_at, paid)
         VALUES(?, ?, 0)`,
		lineUserID, visitedAt,
	); err != nil {
		return err
	}

	return tx.Commit()
}

