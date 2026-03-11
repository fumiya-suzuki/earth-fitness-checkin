// store.go
package main

import (
	"sync"
	"time"
)

// フロントから来るJSONの形
type checkinRequest struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

// チェックインしたときの情報
type checkinInfo struct {
	At time.Time // いつチェックインしたか
}

// メモリ上でチェックインを管理する
var (
	mu                   sync.Mutex
	checkedInUsers       = make(map[string]checkinInfo)
	lastCheckoutAtByUser = make(map[string]time.Time)
	maxPeople            = 10 // Max定員10人（Jsと合わせる）
	expireAfter          = 90 * time.Minute
	autoCheckoutBlockFor = 10 * time.Minute
	autoCheckinBlockFor  = 30 * time.Minute
)

// 期限切れの人を消す共通処理
func cleanupExpiredLocked(now time.Time) {
	for id, info := range checkedInUsers {
		if now.Sub(info.At) > expireAfter {
			delete(checkedInUsers, id)
			appLog.info("checkin_expired_cleanup", eventFields{
				"line_user_id":       id,
				"checked_in_at":      info.At.Format(time.RFC3339),
				"expired_after_min":  int(expireAfter / time.Minute),
				"cleaned_up_at":      now.Format(time.RFC3339),
				"cleanup_reason":     "expired_session",
				"remaining_checkedin": len(checkedInUsers),
			})
		}
	}
}

// ユーザーを追加して現在の人数を返す
func addCheckin(userID string) int {
	mu.Lock()
	defer mu.Unlock()

	cleanupExpiredLocked(time.Now())

	checkedInUsers[userID] = checkinInfo{At: time.Now()}
	delete(lastCheckoutAtByUser, userID)
	return len(checkedInUsers)
}

// チェックアウトして現在の人数を返す
func removeCheckin(userID string) int {
	mu.Lock()
	defer mu.Unlock()

	if _, ok := checkedInUsers[userID]; ok {
		delete(checkedInUsers, userID)
		lastCheckoutAtByUser[userID] = time.Now()
	} else {
		appLog.info("checkout_without_active_checkin", eventFields{
			"line_user_id": userID,
		})
	}
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

type autoToggleStatus struct {
	CheckedIn                  bool
	Count                      int
	Max                        int
	CanAutoCheckout            bool
	CanAutoCheckin             bool
	AutoCheckoutBlockedSeconds int
	AutoCheckinBlockedSeconds  int
}

func getAutoToggleStatus(userID string) autoToggleStatus {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	cleanupExpiredLocked(now)

	status := autoToggleStatus{
		Count: len(checkedInUsers),
		Max:   maxPeople,
	}

	info, checkedIn := checkedInUsers[userID]
	status.CheckedIn = checkedIn

	if checkedIn {
		elapsed := now.Sub(info.At)
		if elapsed >= autoCheckoutBlockFor {
			status.CanAutoCheckout = true
		} else {
			status.CanAutoCheckout = false
			status.AutoCheckoutBlockedSeconds = int((autoCheckoutBlockFor - elapsed).Seconds())
			if status.AutoCheckoutBlockedSeconds < 0 {
				status.AutoCheckoutBlockedSeconds = 0
			}
		}
		status.CanAutoCheckin = false
		return status
	}

	lastCheckoutAt, hasLastCheckout := lastCheckoutAtByUser[userID]
	if !hasLastCheckout {
		status.CanAutoCheckin = true
		return status
	}

	elapsed := now.Sub(lastCheckoutAt)
	if elapsed >= autoCheckinBlockFor {
		status.CanAutoCheckin = true
	} else {
		status.CanAutoCheckin = false
		status.AutoCheckinBlockedSeconds = int((autoCheckinBlockFor - elapsed).Seconds())
		if status.AutoCheckinBlockedSeconds < 0 {
			status.AutoCheckinBlockedSeconds = 0
		}
	}

	return status
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
		appLog.error("db_error", eventFields{
			"line_user_id": lineUserID,
			"operation":    "upsert_member_on_visit",
			"error":        err.Error(),
		})
		return err
	}

	// visits に1件挿入（paid は 0）
	if _, err = tx.Exec(
		`INSERT INTO visits(line_user_id, visited_at, paid)
         VALUES(?, ?, 0)`,
		lineUserID, visitedAt,
	); err != nil {
		appLog.error("db_error", eventFields{
			"line_user_id": lineUserID,
			"operation":    "insert_visit",
			"error":        err.Error(),
		})
		return err
	}

	return tx.Commit()
}
