package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	_ "github.com/lib/pq" // Driver para PostgreSQL
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

type Request struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	AuthorID     int    `json:"authorId"`
	Department   string `json:"department"`
	ServiceType  string `json:"serviceType"`
	EventName    string `json:"eventName"`
	EventDate    string `json:"eventDate"`
	Observations string `json:"observations"`
}

type User struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Role       string `json:"role"`
	Department string `json:"department"`
	Photo      string `json:"photo"`
}

func main() {
	var err error

	// Ligar à base de dados usando a variável de ambiente
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		// Substituir pela string do Neon/Supabase para testes locais
		connStr = ""
	}

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTables()
	seedDatabase()

	// Servir ficheiros estáticos
	http.Handle("/", http.FileServer(http.Dir("./public")))

	// Endpoints da API
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/requests", handleRequests)
	http.HandleFunc("/api/requests/list", handleListRequests)
	http.HandleFunc("/api/requests/update", handleUpdateStatus)
	http.HandleFunc("/api/user/update", handleUpdateProfile)
	http.HandleFunc("/api/user/upload-photo", handleUploadPhoto)
	http.HandleFunc("/api/admin/users", handleGetUsers)
	http.HandleFunc("/api/admin/update-role", handleUpdateRole)
	http.HandleFunc("/api/admin/delete-user", handleDeleteUser)

	http.HandleFunc("/api/gallery/upload", handleGalleryUpload)
	http.HandleFunc("/api/gallery/public", handlePublicGallery)
	http.HandleFunc("/api/gallery/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			return
		}
		title := r.URL.Query().Get("title")
		_, err := db.Exec("DELETE FROM gallery WHERE title = $1", title)
		if err != nil {
			http.Error(w, "Erro ao eliminar", 500)
			return
		}
		w.WriteHeader(200)
	})
	http.HandleFunc("/api/gallery/admin-list", handleAdminGalleryList)
	http.HandleFunc("/api/gallery/add-photos", handleAddPhotosToAlbum)
	http.HandleFunc("/api/gallery/delete-photo", handleDeletePhoto)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Servidor online na porta %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func createTables() {
	// Tabelas adaptadas para SERIAL (Postgres)
	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name TEXT,
			email TEXT UNIQUE,
			password TEXT,
			role TEXT,
			department TEXT,
			photo TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS requests (
			id SERIAL PRIMARY KEY,
			title TEXT,
			description TEXT,
			status TEXT DEFAULT 'PENDING',
			author_id INTEGER,
			department TEXT,
			service_type TEXT,
			event_name TEXT,
			event_date TEXT,
			observations TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	}
	for _, t := range tables {
		db.Exec(t)
	}
}

// NOTIFICAÇÃO POR EMAIL
func sendEmailNotification(req Request) {
	from := os.Getenv("EMAIL_USER")
	pass := os.Getenv("EMAIL_PASS")
	if from == "" || pass == "" {
		return
	}

	// Procurar todos os emails da equipa de Logística
	rows, err := db.Query("SELECT email FROM users WHERE role = 'LOGISTICA'")
	if err != nil {
		return
	}
	defer rows.Close()

	var recipients []string
	for rows.Next() {
		var email string
		rows.Scan(&email)
		recipients = append(recipients, email)
	}

	if len(recipients) == 0 {
		return
	}

	subject := "Subject: [AEF CUP] Novo Pedido - " + req.Title + "\n"
	body := fmt.Sprintf("Olá Logística,\n\nUm novo pedido foi criado por %s:\n\n"+
		"Evento: %s\nServiço: %s\nDescrição: %s\n\nAcedam ao Dashboard para gerir: https://logistica-aefcup.onrender.com/.",
		req.Department, req.EventName, req.ServiceType, req.Description)

	auth := smtp.PlainAuth("", from, pass, "smtp.gmail.com")
	smtp.SendMail("smtp.gmail.com:587", auth, from, recipients, []byte(subject+"\n"+body))
}

func handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req Request
		json.NewDecoder(r.Body).Decode(&req)

		query := `INSERT INTO requests (title, description, author_id, department, service_type, event_name, event_date, observations, status) 
                  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'PENDING')`

		_, err := db.Exec(query, req.Title, req.Description, req.AuthorID, req.Department, req.ServiceType, req.EventName, req.EventDate, req.Observations)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		go sendEmailNotification(req) // Enviar emails em background
		w.WriteHeader(201)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&creds)

	var u User
	var hash string

	// Usamos COALESCE para evitar o erro de NULL no campo photo
	query := `SELECT id, name, email, password, role, department, COALESCE(photo, '') 
              FROM public.users 
              WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))`

	log.Printf("Tentativa de login para: [%s]", creds.Email)

	// O Scan agora receberá sempre uma string, mesmo que photo seja NULL no Neon
	err := db.QueryRow(query, creds.Email).Scan(&u.ID, &u.Name, &u.Email, &hash, &u.Role, &u.Department, &u.Photo)

	if err != nil {
		log.Printf("Erro na BD: %v", err)
		http.Error(w, "Incorreto", 401)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(creds.Password)); err != nil {
		log.Println("Senha errada")
		http.Error(w, "Incorreto", 401)
		return
	}

	log.Printf("✅ Login bem sucedido para: %s", u.Email)
	u.Password = ""
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u)
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var u User
	json.NewDecoder(r.Body).Decode(&u)
	hash, _ := bcrypt.GenerateFromPassword([]byte(u.Password), 10)
	_, err := db.Exec("INSERT INTO users (name, email, password, role, department) VALUES ($1, $2, $3, 'COORDENADOR', $4)",
		u.Name, u.Email, string(hash), u.Department)
	if err != nil {
		http.Error(w, "Erro", 400)
		return
	}
	w.WriteHeader(201)
}

func handleListRequests(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	role := r.URL.Query().Get("role")

	var rows *sql.Rows
	var err error
	if role == "LOGISTICA" {
		rows, err = db.Query("SELECT id, title, description, status, author_id, department, service_type, event_name, event_date, observations FROM requests ORDER BY id DESC")
	} else {
		rows, err = db.Query("SELECT id, title, description, status, author_id, department, service_type, event_name, event_date, observations FROM requests WHERE author_id = $1 ORDER BY id DESC", userID)
	}

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var reqs []Request
	for rows.Next() {
		var i Request
		rows.Scan(&i.ID, &i.Title, &i.Description, &i.Status, &i.AuthorID, &i.Department, &i.ServiceType, &i.EventName, &i.EventDate, &i.Observations)
		reqs = append(reqs, i)
	}
	json.NewEncoder(w).Encode(reqs)
}

func handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	var data struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Role   string `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&data)
	if data.Role == "LOGISTICA" {
		db.Exec("UPDATE requests SET status = $1 WHERE id = $2", data.Status, data.ID)
		w.WriteHeader(200)
	}
}

func handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var u User
	json.NewDecoder(r.Body).Decode(&u)
	if u.Password != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(u.Password), 10)
		db.Exec("UPDATE users SET name = $1, password = $2, photo = $3 WHERE id = $4", u.Name, string(hash), u.Photo, u.ID)
	} else {
		db.Exec("UPDATE users SET name = $1, photo = $2 WHERE id = $3", u.Name, u.Photo, u.ID)
	}
	w.WriteHeader(200)
}

func handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	file, handler, _ := r.FormFile("photo")
	defer file.Close()
	userID := r.FormValue("id")
	os.MkdirAll("./public/uploads", os.ModePerm)
	path := fmt.Sprintf("/uploads/%s_%s", userID, handler.Filename)
	dst, _ := os.Create("./public" + path)
	defer dst.Close()
	io.Copy(dst, file)
	db.Exec("UPDATE users SET photo = $1 WHERE id = $2", path, userID)
	fmt.Fprint(w, path)
}

func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("role") != "LOGISTICA" {
		return
	}
	rows, _ := db.Query("SELECT id, name, email, role, department FROM users")
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Department)
		users = append(users, u)
	}
	json.NewEncoder(w).Encode(users)
}

func handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	var data struct {
		TargetID  int    `json:"id"`
		NewRole   string `json:"role"`
		AdminRole string `json:"adminRole"`
	}
	json.NewDecoder(r.Body).Decode(&data)
	if data.AdminRole == "LOGISTICA" {
		db.Exec("UPDATE users SET role = $1 WHERE id = $2", data.NewRole, data.TargetID)
	}
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	var data struct {
		TargetID  int    `json:"id"`
		AdminRole string `json:"adminRole"`
	}
	json.NewDecoder(r.Body).Decode(&data)
	if data.AdminRole == "LOGISTICA" {
		db.Exec("DELETE FROM users WHERE id = $1", data.TargetID)
	}
}

func seedDatabase() {
	hash, _ := bcrypt.GenerateFromPassword([]byte("Romdsc__18"), 10)
	db.Exec("INSERT INTO users (name, email, password, role, department) VALUES ($1, $2, $3, 'LOGISTICA', 'Direção') ON CONFLICT (email) DO NOTHING",
		"Rodrigo Coelho", "rodrimcoelho@gmail.com", string(hash))
}

func handleGalleryUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}

	r.ParseMultipartForm(32 << 20)
	title := r.FormValue("title")
	dept := r.FormValue("department")
	cld, _ := cloudinary.NewFromURL(os.Getenv("CLOUDINARY_URL"))

	// 1. PROCESSAR O CARTAZ (is_cover = TRUE)
	coverFile, _, err := r.FormFile("cover")
	if err == nil {
		resp, _ := cld.Upload.Upload(r.Context(), coverFile, uploader.UploadParams{Folder: "aefcup"})
		db.Exec("INSERT INTO gallery (title, image_url, department, is_cover) VALUES ($1, $2, $3, TRUE)",
			title, resp.SecureURL, dept)
		coverFile.Close()
	}

	// 2. PROCESSAR AS RESTANTES FOTOS (is_cover = FALSE)
	files := r.MultipartForm.File["images"]
	for _, fileHeader := range files {
		file, _ := fileHeader.Open()
		resp, _ := cld.Upload.Upload(r.Context(), file, uploader.UploadParams{Folder: "aefcup"})
		db.Exec("INSERT INTO gallery (title, image_url, department, is_cover) VALUES ($1, $2, $3, FALSE)",
			title, resp.SecureURL, dept)
		file.Close()
	}
	w.WriteHeader(201)
}

func handlePublicGallery(w http.ResponseWriter, r *http.Request) {
	const secretToken = "aefcup-galeria-2026-xyz"
	if r.URL.Query().Get("token") != secretToken {
		http.Error(w, "Acesso Negado", 403)
		return
	}
	rows, _ := db.Query("SELECT title, image_url, is_cover FROM gallery ORDER BY created_at DESC")
	var photos []map[string]interface{} // Mudado para interface{} para suportar booleanos
	for rows.Next() {
		var t, u string
		var ic bool // Variável para ler o booleano
		rows.Scan(&t, &u, &ic)
		photos = append(photos, map[string]interface{}{"title": t, "url": u, "is_cover": ic})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photos)
}

func handleAdminGalleryList(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT title, image_url, is_cover FROM gallery ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var photos []map[string]interface{}
	for rows.Next() {
		var t, u string
		var ic bool
		rows.Scan(&t, &u, &ic)
		photos = append(photos, map[string]interface{}{"title": t, "url": u, "is_cover": ic})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photos)
}

func handleAddPhotosToAlbum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}

	r.ParseMultipartForm(32 << 20)
	title := r.FormValue("title")
	dept := r.FormValue("department")
	cld, _ := cloudinary.NewFromURL(os.Getenv("CLOUDINARY_URL"))

	files := r.MultipartForm.File["images"]
	for _, fileHeader := range files {
		file, _ := fileHeader.Open()
		resp, _ := cld.Upload.Upload(r.Context(), file, uploader.UploadParams{Folder: "aefcup"})
		db.Exec("INSERT INTO gallery (title, image_url, department, is_cover) VALUES ($1, $2, $3, FALSE)",
			title, resp.SecureURL, dept)
		file.Close()
	}
	w.WriteHeader(200)
}

func handleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		return
	}

	// Recebe o URL da imagem para a remover da BD
	photoURL := r.URL.Query().Get("url")
	_, err := db.Exec("DELETE FROM gallery WHERE image_url = $1", photoURL)
	if err != nil {
		http.Error(w, "Erro ao eliminar foto", 500)
		return
	}
	w.WriteHeader(200)
}
