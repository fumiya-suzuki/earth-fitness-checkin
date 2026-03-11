package main

import "time"

func jstNow() time.Time {
	return time.Now().In(jst)
}

func formatJSTDate(t time.Time) string {
	return t.In(jst).Format("2006-01-02")
}

func formatJSTMonth(t time.Time) string {
	return t.In(jst).Format("2006-01")
}

func formatJSTDateTime(t time.Time) string {
	return t.In(jst).Format("2006-01-02 15:04:05")
}
