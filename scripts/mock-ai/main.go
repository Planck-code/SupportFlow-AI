// Command mock-ai serves a fixed Chat Completions response for local Postman demos.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	demoOutage := flag.Bool("demo-outage", false, "return 503 for the synthetic A3 outage ticket")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if *demoOutage {
			var request struct {
				Messages []struct{ Role, Content string }
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
				http.Error(w, "invalid fixture request", http.StatusBadRequest)
				return
			}
			for _, m := range request.Messages {
				var input struct{ Title string }
				if m.Role == "user" && json.Unmarshal([]byte(m.Content), &input) == nil && input.Title == "A3 synthetic provider outage" {
					http.Error(w, "simulated outage", http.StatusServiceUnavailable)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		message := map[string]string{"role": "assistant", "content": `{"category":"general","summary":"本地模拟摘要","reply_suggestion":"请提供更多问题细节。"}`}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message}}})
	})
	server := &http.Server{Addr: "127.0.0.1:19090", Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	log.Print("local mock provider listening on 127.0.0.1:19090")
	log.Fatal(server.ListenAndServe())
}
