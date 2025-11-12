// store.go
package main

import (
	"sync"
	"time"
)

// フロントから来るJSONの形
type checkinRequest struct {
	UserID string `json:"userId"`
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
	expireAfter    = 3 * time.Hour // 3時間で自動的に無効とみなす
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