// Command mock-ai serves a fixed Chat Completions response for local Postman demos.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		message := map[string]string{"role": "assistant", "content": `{"category":"general","summary":"本地模拟摘要","reply_suggestion":"请提供更多问题细节。"}`}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message}}})
	})
	server := &http.Server{Addr: "127.0.0.1:19090", Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	log.Print("local mock provider listening on 127.0.0.1:19090")
	log.Fatal(server.ListenAndServe())
}
