package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// GET /count-json
func handleCountJSON(w http.ResponseWriter, r *http.Request) {
	count := getCurrentCount() // ← ここで期限切れも掃除される
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"count": count,
		"max":   getMaxPeople(),
	})
}

// POST /checkin
func handleCheckin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req checkinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// 来店履歴を保存
	if err := recordVisit(req.UserID, req.DisplayName); err != nil {
		log.Println("recordVisit error:", err)
	}

	count := addCheckin(req.UserID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{
		"count": count,
		"max":   getMaxPeople(),
	}); err != nil {
		log.Println("encode error:", err)
	}

	log.Printf("チェックイン：%+v\n", req)
}

// POST /checkout
func handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req checkinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	count := removeCheckin(req.UserID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{
		"count": count,
		"max":   getMaxPeople(),
	}); err != nil {
		log.Println("encode error:", err)
	}

	log.Printf("チェックアウト: %+v\n", req)
}

// GET /status?userId=xxxx
func handleStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	status := getAutoToggleStatus(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"checkedIn":                  status.CheckedIn,
		"count":                      status.Count,
		"max":                        status.Max,
		"canAutoCheckout":            status.CanAutoCheckout,
		"canAutoCheckin":             status.CanAutoCheckin,
		"autoCheckoutBlockedSeconds": status.AutoCheckoutBlockedSeconds,
		"autoCheckinBlockedSeconds":  status.AutoCheckinBlockedSeconds,
	})
}
