package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	_ "modernc.org/sqlite"
)

type Note struct {
	Id      int64  `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type IndexPageData struct {
	Notes    []Note
	Page     int
	HasNext  bool
	HasPrev  bool
	NextPage int
	PrevPage int
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
		{"update", "templates/update.html"},
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

func (a *App) mainPage(w http.ResponseWriter, r *http.Request) {
	var notes []Note

	pageStr := r.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit := 1
	offset := (page - 1) * limit

	rows, err := a.DB.Query("SELECT id, title, content FROM notes LIMIT ? OFFSET ?", limit+1, offset)
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

	hasNext := false
	if len(notes) > limit {
		hasNext = true
		notes = notes[:limit] // отрезаем лишний один
	}

	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println(len(notes))
	data := IndexPageData{
		Notes:    notes,
		Page:     page,
		HasPrev:  page > 1,
		HasNext:  hasNext,
		PrevPage: page - 1,
		NextPage: page + 1,
	}

	// Отдаём HTML-страницу
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Templates["index"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) addPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, "templates/add.html")
}

func (a *App) updatePage(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Templates["update"].Execute(w, note); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *App) detailPage(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Templates["details"].Execute(w, note); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handler
func (a *App) createNoteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

func (a *App) updateNoteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	res, err := a.DB.Exec("UPDATE notes SET title = ?, content = ? WHERE id = ?", id)
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

func (a *App) backHandler(w http.ResponseWriter, r *http.Request) {
	returnURL := r.URL.Query().Get("return")
	if returnURL == "" {
		returnURL = "/"
	}
	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatal(err)
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
	http.HandleFunc("/new-note", app.addPage)
	http.HandleFunc("/update-note", app.updatePage)
	http.HandleFunc("/note", app.detailPage)

	http.HandleFunc("/back", app.backHandler)
	http.HandleFunc("/create", app.createNoteHandler)
	http.HandleFunc("/update", app.updateNoteHandler)
	http.HandleFunc("/remove", app.removeNoteHandler)

	fmt.Println("Сервер запущен: http://localhost:8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Ошибка сервера:", err)
	}
}
