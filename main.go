package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	publicDir := filepath.Join(".", "public")
	fs := http.FileServer(http.Dir(publicDir))
	http.Handle("/", fs)

	// APIハンドラ登録
	http.HandleFunc("/checkin", handleCheckin)
	http.HandleFunc("/checkout", handleCheckout)

	http.HandleFunc("/count-json", handleCountJSON)
	http.HandleFunc("/status", handleStatus)
	
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
