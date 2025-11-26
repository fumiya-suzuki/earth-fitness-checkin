// admin.go
package main

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"
	"net/url"
	"fmt"
	"strings"
)

// 1人分の集計結果
type VisitSummary struct {
	LineUserID     string
	DisplayName    string
	FullName       string
	MemberType     string
	Count          int
	HighlightRed   bool // 未払いあり
	HighlightGreen bool // 全て支払い済み
	PosterID       string
}

var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
}

var adminVisitsTmpl = template.Must(
	template.New("admin_visits.html").
		Funcs(funcMap).
		ParseFiles(filepath.Join("public", "admin_visits.html")),
)

var adminVisitsTodayTmpl = template.Must(
	template.New("admin_visits_today.html").
		Funcs(funcMap).
		ParseFiles(filepath.Join("public", "admin_visits_today.html")),
)

var adminVisitDetailTmpl = template.Must(
	template.New("admin_visit_detail.html").
		Funcs(funcMap).
		ParseFiles(filepath.Join("public", "admin_visit_detail.html")),
)

// 今月分の集計を取得（フィルタ付き）
func getMonthlySummaries(year int, month int, filterText, filterType string) ([]VisitSummary, error) {
	ym := fmt.Sprintf("%04d-%02d", year, month)

	baseSQL := `
SELECT 
  v.line_user_id,
  IFNULL(m.display_name, ''),
  IFNULL(m.full_name, ''), 
  IFNULL(m.member_type, 'general'),
  IFNULL(m.poster_id, ''),
  COUNT(v.id) as cnt
FROM visits v
LEFT JOIN members m ON m.line_user_id = v.line_user_id
WHERE strftime('%Y-%m', v.visited_at, 'localtime') = ?
`
	args := []interface{}{ym}
	var where []string

	// 会員種別フィルタ（general / 1day）
	if filterType == "general" || filterType == "1day" {
		where = append(where, "m.member_type = ?")
		args = append(args, filterType)
	}

	// 名前 / フルネーム / PosterID / LINE ID でのキーワード検索
	if filterText != "" {
		like := "%" + filterText + "%"
		where = append(where,
			"(IFNULL(m.display_name,'') LIKE ? OR " +
				"IFNULL(m.full_name,'') LIKE ? OR " +
				"IFNULL(m.poster_id,'') LIKE ? OR " +
				"v.line_user_id LIKE ?)",
		)
		args = append(args, like, like, like, like)
	}

	if len(where) > 0 {
		baseSQL += " AND " + strings.Join(where, " AND ")
	}

	baseSQL += `
GROUP BY v.line_user_id, m.display_name, m.member_type, m.poster_id
ORDER BY cnt DESC, m.display_name;
`

	rows, err := db.Query(baseSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VisitSummary
	for rows.Next() {
		var s VisitSummary
		if err := rows.Scan(
			&s.LineUserID,
			&s.DisplayName,
			&s.FullName,
			&s.MemberType,
			&s.PosterID,
			&s.Count,
		); err != nil {
			return nil, err
		}

		// デフォルトは色なし
		s.HighlightRed = false
		s.HighlightGreen = false

		// 1Day かつ 今月6回以上なら支払い状況をチェック
		if s.MemberType == "1day" && s.Count >= 6 {
			allPaid, err := isAllDueVisitsPaid(s.LineUserID)
			if err != nil {
				log.Println("isAllDueVisitsPaid error:", err)
			} else if allPaid {
				s.HighlightGreen = true
			} else {
				s.HighlightRed = true
			}
		}

		list = append(list, s)
	}

	return list, rows.Err()
}

// 今日分の集計を取得（ここは今まで通り）
func getTodaySummaries() ([]VisitSummary, error) {
	rows, err := db.Query(`
WITH monthly AS (
  SELECT
    v.line_user_id,
    COUNT(*) AS monthly_cnt
  FROM visits v
  WHERE strftime('%Y-%m', v.visited_at, 'localtime') = strftime('%Y-%m', 'now', 'localtime')
  GROUP BY v.line_user_id
)
SELECT 
  v.line_user_id,
  IFNULL(m.display_name, ''),
  IFNULL(m.full_name, ''),
  IFNULL(m.member_type, 'general'),
  IFNULL(m.poster_id, ''),
  COALESCE(monthly.monthly_cnt, 0) AS cnt
FROM visits v
LEFT JOIN members m
  ON m.line_user_id = v.line_user_id
LEFT JOIN monthly
  ON monthly.line_user_id = v.line_user_id
WHERE date(v.visited_at, 'localtime') = date('now', 'localtime')
GROUP BY
  v.line_user_id,
  m.display_name,
  m.full_name,
  m.member_type,
  m.poster_id,
  monthly.monthly_cnt
ORDER BY
  cnt DESC,
  m.full_name,
  m.display_name;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VisitSummary
	for rows.Next() {
		var s VisitSummary
		if err := rows.Scan(
			&s.LineUserID,
			&s.DisplayName,
			&s.FullName,
			&s.MemberType,
			&s.PosterID,
			&s.Count,
		); err != nil {
			return nil, err
		}

		s.HighlightRed = false
		s.HighlightGreen = false

		if s.MemberType == "1day" && s.Count >= 6 {
			allPaid, err := isAllDueVisitsPaid(s.LineUserID)
			if err != nil {
				log.Println("isAllDueVisitsPaid error:", err)
			} else if allPaid {
				s.HighlightGreen = true
			} else {
				s.HighlightRed = true
			}
		}

		list = append(list, s)
	}
	return list, rows.Err()
}

// 今月・前月一覧
func handleAdminVisits(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	q := r.URL.Query().Get("q")                     // フィルタ文字列
	memberType := r.URL.Query().Get("member_type")  // "general" / "1day" / ""

	base := time.Now()
	if mode == "prev" {
		base = base.AddDate(0, -1, 0)
	}

	// この月のラベル & キー
	monthLabel := base.Format("2006年1月") // 例: 2025年10月
	monthKey := base.Format("2006-01")    // 例: 2025-10（SQL用）

	year, m, _ := base.Date()
	summaries, err := getMonthlySummaries(year, int(m), q, memberType)
	if err != nil {
		log.Println("getMonthlySummaries error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	successMsg := r.URL.Query().Get("success_msg")

	data := struct {
		Summaries        []VisitSummary
		MonthLabel       string
		MonthKey         string
		Mode             string
		IsPrev           bool
		IsCurrent        bool
		SuccessMsg       string
		Q                string
		MemberTypeFilter string
	}{
		Summaries:        summaries,
		MonthLabel:       monthLabel,
		MonthKey:         monthKey,
		Mode:             mode,
		IsPrev:           mode == "prev",
		IsCurrent:        mode != "prev",
		SuccessMsg:       successMsg,
		Q:                q,
		MemberTypeFilter: memberType,
	}

	if err := adminVisitsTmpl.Execute(w, data); err != nil {
		log.Println("template execute error:", err)
	}
}

// 今日一覧（そのまま）
func handleAdminVisitsToday(w http.ResponseWriter, r *http.Request) {
	summaries, err := getTodaySummaries()
	if err != nil {
		log.Println("getTodaySummaries error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	dateLabel := now.Format("2006年1月2日")
	successMsg := r.URL.Query().Get("success_msg")

	data := struct {
		Summaries  []VisitSummary
		DateLabel  string
		SuccessMsg string
	}{
		Summaries:  summaries,
		DateLabel:  dateLabel,
		SuccessMsg: successMsg,
	}

	if err := adminVisitsTodayTmpl.Execute(w, data); err != nil {
		log.Println("template execute error:", err)
	}
}

// POST /admin/member/type
func handleAdminUpdateMemberType(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	lineUserID := r.FormValue("line_user_id")
	newType := r.FormValue("member_type")

	if lineUserID == "" || (newType != "general" && newType != "1day") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// DB更新
	_, err := db.Exec(
		`UPDATE members SET member_type = ? WHERE line_user_id = ?`,
		newType, lineUserID,
	)
	if err != nil {
		log.Println("update member_type error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[ADMIN] change member_type: %s -> %s\n", lineUserID, newType)

	// 終わったら一覧に戻す
	http.Redirect(w, r, "/admin/members", http.StatusSeeOther)
}

// POST /admin/member/poster-id
func handleAdminUpdatePosterID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	lineUserID := r.FormValue("line_user_id")
	posterID := r.FormValue("poster_id")

	if lineUserID == "" {
		http.Error(w, "line_user_id is required", http.StatusBadRequest)
		return
	}

	// 表示名を取っておく（メッセージ用）
	var displayName string
	err := db.QueryRow(
		`SELECT IFNULL(display_name, '') FROM members WHERE line_user_id = ?`,
		lineUserID,
	).Scan(&displayName)
	if err != nil {
		log.Println("select display_name error:", err)
	}

	// 空文字も許容（クリアしたい場合もあるので）
	_, err = db.Exec(
		`UPDATE members SET poster_id = ? WHERE line_user_id = ?`,
		posterID, lineUserID,
	)
	if err != nil {
		log.Println("update poster_id error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// メッセージ作成
	if displayName == "" {
		displayName = "不明なユーザー"
	}
	msg := fmt.Sprintf("%s さんのPosterIDを正常に更新しました。", displayName)

	// 直前に見ていたページ（今月 or 今日）のURLをベースにリダイレクト
	redirectTo := "/admin/visits"
	if ref := r.Referer(); ref != "" {
		if u, err := url.Parse(ref); err == nil {
			q := u.Query()
			q.Set("success_msg", msg)
			u.RawQuery = q.Encode()
			redirectTo = u.RequestURI()
		}
	}

	log.Printf("[ADMIN] update poster_id: %s -> %s\n", lineUserID, posterID)
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// あるユーザーの「今月の来店履歴」を取得
type VisitRecord struct {
	ID          int
	TimeStr     string
	NeedPayment bool
	Paid        bool
}

type VisitDetail struct {
	LineUserID string
	PosterID   string
	DisplayName string
	FullName    string
	MemberType  string
	MonthLabel  string
	MonthKey    string
	IsPrev      bool
	Count       int
	Visits      []VisitRecord
}

// ym: "2025-11" のような "YYYY-MM" 形式。空文字なら「今月」
func getUserMonthlyVisitDetail(lineUserID, ym string) (*VisitDetail, error) {
	// 対象月の決定
	var base time.Time
	if ym != "" {
		// "2025-10" → time.Time
		if t, err := time.Parse("2006-01", ym); err == nil {
			base = t
		} else {
			base = time.Now()
		}
	} else {
		base = time.Now()
	}

	monthLabel := base.Format("2006年1月") // 画面用
	monthKey := base.Format("2006-01")    // SQL用 "YYYY-MM"

	// ここで「前月かどうか」を判定
	now := time.Now()
	prev := now.AddDate(0, -1, 0)
	isPrev := base.Year() == prev.Year() && base.Month() == prev.Month()

	detail := &VisitDetail{
		LineUserID: lineUserID,
		MonthLabel: monthLabel,
		MonthKey:   monthKey,
		IsPrev:     isPrev,
	}

	rows, err := db.Query(`
        SELECT 
          v.id,
          IFNULL(m.display_name, ''),
          IFNULL(m.full_name, ''),
          IFNULL(m.member_type, 'general'),
          IFNULL(m.poster_id, ''), 
          strftime('%Y/%m/%d %H:%M', v.visited_at, 'localtime') AS visited_local,
          IFNULL(v.paid, 0)
        FROM visits v
        LEFT JOIN members m ON m.line_user_id = v.line_user_id
        WHERE v.line_user_id = ?
          AND strftime('%Y-%m', v.visited_at, 'localtime') = ?
        ORDER BY v.visited_at ASC
    `, lineUserID, monthKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		var (
			id           int
			name         string
			fullName     string
			memberType   string
			posterID     string
			visitedAtStr string
			paidInt      int
		)
		if err := rows.Scan(&id, &name, &fullName, &memberType, &posterID, &visitedAtStr, &paidInt); err != nil {
			return nil, err
		}

		i++
		if detail.DisplayName == "" {
			detail.DisplayName = name
			detail.FullName = fullName
			detail.MemberType = memberType
			detail.PosterID = posterID
		}

		rec := VisitRecord{
			ID:          id,
			TimeStr:     visitedAtStr,
			Paid:        paidInt != 0,
			NeedPayment: (memberType == "1day" && i >= 6),
		}
		detail.Visits = append(detail.Visits, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	detail.Count = len(detail.Visits)
	return detail, nil
}

// GET /admin/visits/user?line_user_id=xxxxx
func handleAdminVisitDetail(w http.ResponseWriter, r *http.Request) {
	lineUserID := r.URL.Query().Get("line_user_id")
	if lineUserID == "" {
		http.Error(w, "line_user_id is required", http.StatusBadRequest)
		return
	}

	ym := r.URL.Query().Get("month") // 例: "2025-10"。空なら「今月」を扱う

	detail, err := getUserMonthlyVisitDetail(lineUserID, ym)
	if err != nil {
		log.Println("getUserMonthlyVisitDetail error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := adminVisitDetailTmpl.Execute(w, detail); err != nil {
		log.Println("template execute error:", err)
	}
}

// 今月の 6 回目以降の来店がすべて paid=1 かどうか
func isAllDueVisitsPaid(lineUserID string) (bool, error) {
	rows, err := db.Query(`
SELECT 
  datetime(v.visited_at, 'localtime'),
  IFNULL(v.paid, 0)
FROM visits v
WHERE v.line_user_id = ?
  AND strftime('%Y-%m', v.visited_at, 'localtime')
      = strftime('%Y-%m', 'now', 'localtime')
ORDER BY v.visited_at ASC;
`, lineUserID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		i++
		var visitedAtStr string
		var paidInt int
		if err := rows.Scan(&visitedAtStr, &paidInt); err != nil {
			return false, err
		}
		// 6回目以降で未払いが1件でもあれば NG
		if i >= 6 && paidInt == 0 {
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return true, nil
}

// POST /admin/visits/pay
func handleAdminVisitPay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	visitID := r.FormValue("visit_id")
	lineUserID := r.FormValue("line_user_id")
	month := r.FormValue("month")

	if visitID == "" || lineUserID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// チェックされていれば "1"、外されていれば ""（そもそもキーが来ない）
	paid := 0
	if r.FormValue("paid") == "1" {
		paid = 1
	}

	_, err := db.Exec(`UPDATE visits SET paid = ? WHERE id = ?`, paid, visitID)
	if err != nil {
		log.Println("update paid error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// 詳細画面に戻す
	redirectTo := "/admin/visits/user?line_user_id=" + url.QueryEscape(lineUserID)
	if month != "" {
		redirectTo += "&month=" + url.QueryEscape(month)
	}

	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// 会員一覧 1行分
type MemberSummary struct {
	LineUserID   string
	DisplayName  string
	FullName     string
	MemberType   string
	PosterID     string
	MonthlyCount int
}

var adminMembersTmpl = template.Must(
	template.New("admin_members.html").
		Funcs(funcMap).
		ParseFiles(filepath.Join("public", "admin_members.html")),
)

// 会員一覧 + 今月の来店回数（フィルタ付き）
func getMemberSummaries(filterText, filterType string) ([]MemberSummary, error) {
	baseSQL := `
SELECT
  m.line_user_id,
  IFNULL(m.display_name, ''),
  IFNULL(m.full_name, ''),
  IFNULL(m.member_type, 'general'),
  IFNULL(m.poster_id, ''),
  SUM(
    CASE
      WHEN v.id IS NOT NULL
       AND strftime('%Y-%m', v.visited_at, 'localtime') = strftime('%Y-%m', 'now', 'localtime')
      THEN 1
      ELSE 0
    END
  ) AS monthly_count
FROM members m
LEFT JOIN visits v ON v.line_user_id = m.line_user_id
`
	var where []string
	var args []interface{}

	// 会員種別フィルタ
	if filterType == "general" || filterType == "1day" {
		where = append(where, "m.member_type = ?")
		args = append(args, filterType)
	}

	// キーワード（display_name / full_name / poster_id / line_user_id）
	if filterText != "" {
		like := "%" + filterText + "%"
		where = append(where,
			"(IFNULL(m.display_name,'') LIKE ? OR " +
				"IFNULL(m.full_name,'') LIKE ? OR " +
				"IFNULL(m.poster_id,'') LIKE ? OR " +
				"m.line_user_id LIKE ?)",
		)
		args = append(args, like, like, like, like)
	}

	if len(where) > 0 {
		baseSQL += " WHERE " + strings.Join(where, " AND ")
	}

	baseSQL += `
GROUP BY
  m.line_user_id,
  m.display_name,
  m.full_name,
  m.member_type,
  m.poster_id
ORDER BY
  m.full_name, m.display_name;
`

	rows, err := db.Query(baseSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MemberSummary
	for rows.Next() {
		var s MemberSummary
		if err := rows.Scan(
			&s.LineUserID,
			&s.DisplayName,
			&s.FullName,
			&s.MemberType,
			&s.PosterID,
			&s.MonthlyCount,
		); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GET /admin/members
func handleAdminMembers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	memberType := r.URL.Query().Get("member_type")

	members, err := getMemberSummaries(q, memberType)
	if err != nil {
		log.Println("getMemberSummaries error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	successMsg := r.URL.Query().Get("success_msg")

	data := struct {
		Members          []MemberSummary
		SuccessMsg       string
		Q                string
		MemberTypeFilter string
	}{
		Members:          members,
		SuccessMsg:       successMsg,
		Q:                q,
		MemberTypeFilter: memberType,
	}

	if err := adminMembersTmpl.Execute(w, data); err != nil {
		log.Println("template execute error:", err)
	}
}

// POST /admin/visits/add
func handleAdminVisitAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	lineUserID := r.FormValue("line_user_id")
	if lineUserID == "" {
		http.Error(w, "line_user_id is required", http.StatusBadRequest)
		return
	}

	// 本日分の来店を1件追加（支払い済みフラグは 0）
	_, err := db.Exec(
		`INSERT INTO visits (line_user_id, visited_at, paid)
         VALUES (?, datetime('now','localtime'), 0)`,
		lineUserID,
	)
	if err != nil {
		log.Println("insert visit error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// この会員の詳細ページに戻る
	redirectTo := "/admin/visits/user?line_user_id=" + url.QueryEscape(lineUserID)
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// POST /admin/visits/delete
func handleAdminVisitDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	visitID := r.FormValue("visit_id")
	lineUserID := r.FormValue("line_user_id")

	if visitID == "" || lineUserID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// その visit だけ削除
	_, err := db.Exec(`DELETE FROM visits WHERE id = ?`, visitID)
	if err != nil {
		log.Println("delete visit error:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	redirectTo := "/admin/visits/user?line_user_id=" + url.QueryEscape(lineUserID)
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}
