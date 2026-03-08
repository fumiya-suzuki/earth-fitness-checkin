package main

import (
	"fmt"
	"log"
	"time"
)

const (
	visitsRetentionDays = 90
	visitsCleanupEvery  = 24 * time.Hour
)

func startVisitsCleanupJob() {
	// 起動直後に1回実行してから、24時間ごとに削除する。
	runVisitsCleanup()

	go func() {
		ticker := time.NewTicker(visitsCleanupEvery)
		defer ticker.Stop()

		for range ticker.C {
			runVisitsCleanup()
		}
	}()
}

func runVisitsCleanup() {
	res, err := db.Exec(
		`DELETE FROM visits
          WHERE visited_at < datetime('now', 'localtime', ?)`,
		fmt.Sprintf("-%d days", visitsRetentionDays),
	)
	if err != nil {
		log.Println("cleanup visits error:", err)
		return
	}

	deleted, err := res.RowsAffected()
	if err != nil {
		log.Println("cleanup visits rows affected error:", err)
		return
	}

	log.Printf("✅ visits cleanup: deleted %d rows older than %d days\n", deleted, visitsRetentionDays)
}
