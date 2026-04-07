package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {

	initDB()
	startVisitsCleanupJob()
	appLog.cleanupOldFiles(jstNow())

	adminConfig, err := loadAdminAuthConfig()
	if err != nil {
		log.Fatal(err)
	}
	adminAuth := newAdminAuth(adminConfig)

	publicDir := filepath.Join(".", "public")
	fs := http.FileServer(http.Dir(publicDir))
	http.Handle("/", withRequestID(fs))

	// APIハンドラ登録
	handle := func(pattern string, fn http.HandlerFunc) {
		http.Handle(pattern, withRequestID(http.HandlerFunc(fn)))
	}
	handleAdmin := func(pattern string, fn http.HandlerFunc) {
		http.Handle(pattern, adminAuth.middleware(withRequestID(http.HandlerFunc(fn))))
	}

	handle("/checkin", handleCheckin)
	handle("/checkout", handleCheckout)

	handle("/count-json", handleCountJSON)
	handle("/status", handleStatus)
	handle("/client-log", handleClientLog)
	handle("/admin/login", adminAuth.handleLogin)
	handleAdmin("/admin/logout", adminAuth.handleLogout)

	// 管理画面
	handleAdmin("/admin/visits", handleAdminVisits)
	handleAdmin("/admin/visits/today", handleAdminVisitsToday)
	handleAdmin("/admin/visits/calendar", handleAdminVisitsCalendar)
	handleAdmin("/admin/visits/day", handleAdminVisitsDay)
	handleAdmin("/admin/visits/user", handleAdminVisitDetail)
	handleAdmin("/admin/member/type", handleAdminUpdateMemberType)
	handleAdmin("/admin/member/poster-id", handleAdminUpdatePosterID)
	handleAdmin("/admin/visits/pay", handleAdminVisitPay)
	handleAdmin("/admin/visits/add", handleAdminVisitAdd)
	handleAdmin("/admin/visits/delete", handleAdminVisitDelete)
	handleAdmin("/admin/members", handleAdminMembers)
	handle("/member/profile", handleMemberProfile)

	// ポート設定
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("✅ サーバー起動: http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

// POST用のリクエスト
type memberProfileRequest struct {
	UserID      string `json:"userId"`
	LastName    string `json:"lastName"`
	FirstName   string `json:"firstName"`
	MemberType  string `json:"memberType"` // "general" or "1day"
	DisplayName string `json:"displayName"`
}

// GET /member/profile?userId=xxx
// POST /member/profile
func handleMemberProfile(w http.ResponseWriter, r *http.Request) {
	appLog.info("profile_request_received", eventFieldsFromRequest(r))
	switch r.Method {
	case http.MethodGet:
		handleMemberProfileGet(w, r)
	case http.MethodPost:
		handleMemberProfilePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET: プロファイル取得
func handleMemberProfileGet(w http.ResponseWriter, r *http.Request) {
	fields := eventFieldsFromRequest(r)
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "userId is required"
		appLog.error("request_error", fields)
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}
	fields["line_user_id"] = userID

	var (
		fullName   string
		memberType string
	)

	err := db.QueryRow(
		`SELECT IFNULL(full_name, ''), IFNULL(member_type, 'general')
           FROM members
          WHERE line_user_id = ?`,
		userID,
	).Scan(&fullName, &memberType)

	w.Header().Set("Content-Type", "application/json")

	if err == sql.ErrNoRows || fullName == "" {
		// レコードがない or full_name が空 → 未登録扱い
		fields["exists"] = false
		fields["full_name_empty"] = fullName == ""
		appLog.info("profile_lookup", fields)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exists": false,
		})
		return
	}
	if err != nil {
		fields["operation"] = "select_member_profile"
		fields["error"] = err.Error()
		appLog.error("db_error", fields)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	fields["exists"] = true
	fields["member_type"] = memberType
	appLog.info("profile_lookup", fields)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"exists":     true,
		"fullName":   fullName,
		"memberType": memberType,
	})
}

// POST: プロファイル登録/更新
func handleMemberProfilePost(w http.ResponseWriter, r *http.Request) {
	fields := eventFieldsFromRequest(r)
	var req memberProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "bad request"
		appLog.error("request_error", fields)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	fields["line_user_id"] = req.UserID
	fields["display_name"] = req.DisplayName
	fields["member_type"] = req.MemberType
	appLog.info("profile_register_attempt", fields)

	if req.UserID == "" || req.LastName == "" || req.FirstName == "" {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "missing required profile fields"
		appLog.error("request_error", fields)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.MemberType != "general" && req.MemberType != "1day" {
		fields["status"] = http.StatusBadRequest
		fields["error"] = "bad memberType"
		appLog.error("request_error", fields)
		http.Error(w, "bad memberType", http.StatusBadRequest)
		return
	}

	fullName := req.LastName + " " + req.FirstName
	createdAt := formatJSTDateTime(jstNow())

	// ① まず UPDATE（既存会員なら上書き）
	res, err := db.Exec(
		`UPDATE members
            SET full_name = ?, member_type = ?, display_name = ?
          WHERE line_user_id = ?`,
		fullName, req.MemberType, req.DisplayName, req.UserID,
	)
	if err != nil {
		fields["operation"] = "update_member_profile"
		fields["error"] = err.Error()
		appLog.error("db_error", fields)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	rows, _ := res.RowsAffected()

	// ② 該当行がなければ INSERT（新規会員）
	if rows == 0 {
		_, err = db.Exec(
			`INSERT INTO members(line_user_id, display_name, full_name, member_type, created_at)
             VALUES(?, ?, ?, ?, ?)`,
			req.UserID, req.DisplayName, fullName, req.MemberType, createdAt,
		)
		if err != nil {
			fields["operation"] = "insert_member_profile"
			fields["error"] = err.Error()
			appLog.error("db_error", fields)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	successFields := eventFieldsFromRequest(r)
	successFields["line_user_id"] = req.UserID
	successFields["display_name"] = req.DisplayName
	successFields["member_type"] = req.MemberType
	appLog.info("profile_register_success", successFields)
}
