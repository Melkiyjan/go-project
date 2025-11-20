package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"

	_ "modernc.org/sqlite"
)

type Note struct {
	Id      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type App struct {
	DB        *sql.DB
	Templates map[string]*template.Template
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "data.sqlite")
	if err != nil {
		log.Fatal("Ошибка подключения к БД:", err)
	}

	createTable := `
    CREATE TABLE IF NOT EXISTS notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    content TEXT NOT NULL
                                     );`

	_, err = db.Exec(createTable)
	if err != nil {
		log.Fatal("Ошибка создания таблицы:", err)
	}

	return db, nil
}

func (a *App) initTemplates() error {
	a.Templates = make(map[string]*template.Template)

	templates := []struct {
		name string
		file string
	}{
		{"index", "templates/index.html"},
		{"add", "templates/add.html"},
		{"details", "templates/details.html"},
	}

	for _, tmpl := range templates {
		parsed, err := template.ParseFiles(tmpl.file)
		if err != nil {
			return fmt.Errorf("ошибка загрузки %s: %w", tmpl.file, err)
		}
		a.Templates[tmpl.name] = parsed
	}
	return nil
}

func (a *App) mainPage(w http.ResponseWriter, _ *http.Request) {
	var notes []Note

	rows, err := a.DB.Query("SELECT * FROM notes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Логируем, но не прерываем выполнение
			log.Printf("Ошибка при закрытии rows: %v", closeErr)
		}
	}()

	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.Id, &n.Title, &n.Content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		notes = append(notes, n)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Отдаём HTML-страницу
	if err := a.Templates["index"].Execute(w, notes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) addPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "templates/add.html")
}

func (a *App) noteDetailPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	var note Note

	err := a.DB.QueryRow("SELECT id, title, content FROM notes WHERE id = ?", id).
		Scan(&note.Id, &note.Title, &note.Content)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Note not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := a.Templates["details"].Execute(w, note); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) createNoteHandler(w http.ResponseWriter, r *http.Request) {
	title := r.FormValue("title")
	content := r.FormValue("content")

	if title == "" || content == "" {
		http.Error(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	result, err := a.DB.Exec("INSERT INTO notes (title, content) VALUES (?, ?)", title, content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/note?id=%d", id), http.StatusSeeOther)
}

func (a *App) removeNoteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	res, err := a.DB.Exec("DELETE FROM notes WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatal("Ошибка инициализации БД:", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			// Логируем, но не прерываем выполнение
			log.Printf("Ошибка при закрытии db: %v", closeErr)
		}
	}()

	app := &App{DB: db}

	if err := app.initTemplates(); err != nil {
		log.Fatal("Ошибка инициализации шаблонов:", err)
	}

	// можно проверить, что что-то реально загрузилось
	for name := range app.Templates {
		log.Println("Загружен шаблон:", name)
	}

	http.HandleFunc("/", app.mainPage)
	http.HandleFunc("/add", app.addPage)
	http.HandleFunc("/create", app.createNoteHandler)
	http.HandleFunc("/note", app.noteDetailPage)
	http.HandleFunc("/remove", app.removeNoteHandler)

	fmt.Println("Сервер запущен: http://localhost:8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Ошибка сервера:", err)
	}
}
