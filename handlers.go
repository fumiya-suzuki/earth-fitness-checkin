package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type clientLogRequest struct {
	Event       string `json:"event"`
	Level       string `json:"level"`
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Path        string `json:"path"`
	Href        string `json:"href"`
	UserAgent   string `json:"userAgent"`
	Detail      string `json:"detail"`
	Status      int    `json:"status"`
	Phase       string `json:"phase"`
}

// GET /count-json
func handleCountJSON(w http.ResponseWriter, r *http.Request) {
	count := getCurrentCount() // ← ここで期限切れも掃除される
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"count": count,
		"max":   getMaxPeople(),
	})
	fields := eventFieldsFromRequest(r)
	fields["count"] = count
	fields["max"] = getMaxPeople()
	appLog.info("count_checked", fields)
}

// POST /checkin
func handleCheckin(w http.ResponseWriter, r *http.Request) {
	fields := eventFieldsFromRequest(r)
	if r.Method != http.MethodPost {
		fields["status"] = http.StatusMethodNotAllowed
		appLog.error("request_error", fields)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req checkinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "bad request"
		appLog.error("request_error", fields)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	fields["line_user_id"] = req.UserID
	fields["display_name"] = req.DisplayName
	appLog.info("checkin_attempt", fields)

	// 来店履歴を保存
	if err := recordVisit(req.UserID, req.DisplayName); err != nil {
		log.Println("recordVisit error:", err)
		errorFields := eventFields{
			"request_id":   requestIDFromContext(r.Context()),
			"path":         r.URL.Path,
			"method":       r.Method,
			"line_user_id": req.UserID,
			"operation":    "record_visit",
			"error":        err.Error(),
		}
		appLog.error("db_error", errorFields)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	count := addCheckin(req.UserID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{
		"count": count,
		"max":   getMaxPeople(),
	}); err != nil {
		log.Println("encode error:", err)
		appLog.error("response_encode_failed", eventFields{
			"request_id":   requestIDFromContext(r.Context()),
			"path":         r.URL.Path,
			"method":       r.Method,
			"line_user_id": req.UserID,
			"error":        err.Error(),
		})
	}
	successFields := eventFieldsFromRequest(r)
	successFields["line_user_id"] = req.UserID
	successFields["display_name"] = req.DisplayName
	successFields["count_after"] = count
	appLog.info("checkin_success", successFields)

	log.Printf("チェックイン：%+v\n", req)
}

// POST /checkout
func handleCheckout(w http.ResponseWriter, r *http.Request) {
	fields := eventFieldsFromRequest(r)
	if r.Method != http.MethodPost {
		fields["status"] = http.StatusMethodNotAllowed
		appLog.error("request_error", fields)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req checkinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "bad request"
		appLog.error("request_error", fields)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	fields["line_user_id"] = req.UserID
	fields["display_name"] = req.DisplayName
	appLog.info("checkout_attempt", fields)

	count := removeCheckin(req.UserID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{
		"count": count,
		"max":   getMaxPeople(),
	}); err != nil {
		log.Println("encode error:", err)
		appLog.error("response_encode_failed", eventFields{
			"request_id":   requestIDFromContext(r.Context()),
			"path":         r.URL.Path,
			"method":       r.Method,
			"line_user_id": req.UserID,
			"error":        err.Error(),
		})
	}
	successFields := eventFieldsFromRequest(r)
	successFields["line_user_id"] = req.UserID
	successFields["display_name"] = req.DisplayName
	successFields["count_after"] = count
	appLog.info("checkout_success", successFields)

	log.Printf("チェックアウト: %+v\n", req)
}

// GET /status?userId=xxxx
func handleStatus(w http.ResponseWriter, r *http.Request) {
	fields := eventFieldsFromRequest(r)
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "userId is required"
		appLog.error("request_error", fields)
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	status := getAutoToggleStatus(userID)
	fields["line_user_id"] = userID
	fields["checked_in"] = status.CheckedIn
	fields["count"] = status.Count
	fields["max"] = status.Max
	fields["can_auto_checkout"] = status.CanAutoCheckout
	fields["can_auto_checkin"] = status.CanAutoCheckin
	fields["auto_checkout_blocked_seconds"] = status.AutoCheckoutBlockedSeconds
	fields["auto_checkin_blocked_seconds"] = status.AutoCheckinBlockedSeconds
	if status.CheckedIn && !status.CanAutoCheckout {
		fields["reason"] = "auto_checkout_block"
	} else if !status.CheckedIn && !status.CanAutoCheckin {
		fields["reason"] = "auto_checkin_block"
	} else if status.CheckedIn {
		fields["reason"] = "checked_in"
	} else {
		fields["reason"] = "ready_for_checkin"
	}
	appLog.info("status_checked", fields)

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

// POST /client-log
func handleClientLog(w http.ResponseWriter, r *http.Request) {
	fields := eventFieldsFromRequest(r)
	if r.Method != http.MethodPost {
		fields["status"] = http.StatusMethodNotAllowed
		appLog.error("request_error", fields)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req clientLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "bad request"
		appLog.error("request_error", fields)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.UserAgent != "" {
		fields["user_agent"] = req.UserAgent
	}
	fields["client_event"] = req.Event
	fields["client_path"] = req.Path
	fields["href"] = req.Href
	fields["detail"] = req.Detail
	fields["phase"] = req.Phase
	fields["status_code"] = req.Status
	if req.UserID != "" {
		fields["line_user_id"] = req.UserID
	}
	if req.DisplayName != "" {
		fields["display_name"] = req.DisplayName
	}

	if req.Level == "INFO" {
		appLog.info("client_diag", fields)
	} else {
		appLog.error("client_diag", fields)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
