package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"database/sql"
	"encoding/json"
)

func main() {

	initDB()

	publicDir := filepath.Join(".", "public")
	fs := http.FileServer(http.Dir(publicDir))
	http.Handle("/", fs)

	// APIハンドラ登録
	http.HandleFunc("/checkin", handleCheckin)
	http.HandleFunc("/checkout", handleCheckout)

	http.HandleFunc("/count-json", handleCountJSON)
	http.HandleFunc("/status", handleStatus)

	// 管理画面
	http.HandleFunc("/admin/visits", handleAdminVisits)
	http.HandleFunc("/admin/visits/today", handleAdminVisitsToday)
	http.HandleFunc("/admin/visits/user", handleAdminVisitDetail)
	http.HandleFunc("/admin/member/type", handleAdminUpdateMemberType)
	http.HandleFunc("/admin/member/poster-id", handleAdminUpdatePosterID) 
	http.HandleFunc("/admin/visits/pay", handleAdminVisitPay)
	http.HandleFunc("/admin/visits/add", handleAdminVisitAdd)
	http.HandleFunc("/admin/visits/delete", handleAdminVisitDelete)
	http.HandleFunc("/admin/members", handleAdminMembers)
	http.HandleFunc("/member/profile", handleMemberProfile)

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
    UserID     string `json:"userId"`
    LastName   string `json:"lastName"`
    FirstName  string `json:"firstName"`
	MemberType string `json:"memberType"` // "general" or "1day"
	DisplayName string `json:"displayName"`
}

// GET /member/profile?userId=xxx
// POST /member/profile
func handleMemberProfile(w http.ResponseWriter, r *http.Request) {
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
    userID := r.URL.Query().Get("userId")
    if userID == "" {
        http.Error(w, "userId is required", http.StatusBadRequest)
        return
    }

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
        json.NewEncoder(w).Encode(map[string]interface{}{
            "exists": false,
        })
        return
    }
    if err != nil {
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(map[string]interface{}{
        "exists":    true,
        "fullName":  fullName,
        "memberType": memberType,
    })
}


// POST: プロファイル登録/更新
// POST: プロファイル登録/更新
func handleMemberProfilePost(w http.ResponseWriter, r *http.Request) {
    var req memberProfileRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    if req.UserID == "" || req.LastName == "" || req.FirstName == "" {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }
    if req.MemberType != "general" && req.MemberType != "1day" {
        http.Error(w, "bad memberType", http.StatusBadRequest)
        return
    }

    fullName := req.LastName + " " + req.FirstName

    // ① まず UPDATE（既存会員なら上書き）
    res, err := db.Exec(
        `UPDATE members
            SET full_name = ?, member_type = ?, display_name = ?
          WHERE line_user_id = ?`,
        fullName, req.MemberType, req.DisplayName, req.UserID,
    )
    if err != nil {
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }

    rows, _ := res.RowsAffected()

    // ② 該当行がなければ INSERT（新規会員）
    if rows == 0 {
        _, err = db.Exec(
            `INSERT INTO members(line_user_id, display_name, full_name, member_type)
             VALUES(?, ?, ?, ?)`,
            req.UserID, req.DisplayName, fullName, req.MemberType,
        )
        if err != nil {
            http.Error(w, "internal server error", http.StatusInternalServerError)
            return
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
