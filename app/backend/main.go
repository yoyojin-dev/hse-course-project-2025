package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"backend/storage"
)

func main() {
	st := storage.NewMemoryStorage() // Инициализация памяти ОЗУ, потом можно заменить на другую реализацию интерфейса

	http.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello from backend")
	})

	http.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		var code string
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "application/json") {
			var p struct {
				GameCode string `json:"game_code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			code = p.GameCode
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, `{"error":"invalid form"}`, http.StatusBadRequest)
				return
			}
			code = r.FormValue("game_code")
		}

		if code == "" {
			http.Error(w, `{"error":"missing game_code"}`, http.StatusBadRequest)
			return
		}

		if st.ValidateGameID(code) {
			http.Redirect(w, r, "/joining/"+code, http.StatusSeeOther)
			return
		}

		// Если код не найден — ставим flash-cookie и редиректим назад на корень
		cookie := &http.Cookie{
			Name:     "flash",
			Value:    "notfound",
			Path:     "/",
			MaxAge:   5, // короткоживущая
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/api/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintln(w, "method not allowed")
			return
		}

		// В простом варианте принимаем пустое тело и создаём игру с пустым payload
		id, err := st.CreateGame(map[string]interface{}{"created": true})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "cannot create game")
			return
		}

		http.Redirect(w, r, "/created/"+id, http.StatusSeeOther)
	})

	fmt.Println("Backend started on :8080")
	http.ListenAndServe(":8080", nil)
}
