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
	_ "github.com/lib/pq"
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

type ShiftTable struct {
	ID        int    `json:"id"`
	EventName string `json:"eventName"`
	Date      string `json:"date"`
}

type ShiftSlot struct {
	ID        int    `json:"id"`
	TimeRange string `json:"timeRange"`
	Category  string `json:"category"`
	Label     string `json:"label"`
	UserEmail string `json:"userEmail"`
	UserName  string `json:"userName"`
}

type ShiftData struct {
	EventName string `json:"eventName"`
	Date      string `json:"date"`
	Slots     []struct {
		T string `json:"t"`
		C string `json:"c"`
		L string `json:"l"`
	} `json:"slots"`
}

type GridCell struct {
	Row     int    `json:"r"`
	Col     int    `json:"c"`
	Content string `json:"content"`
	IsSlot  bool   `json:"isSlot"`
	Span    int    `json:"span"` // Novo campo
}

type GridPayload struct {
	EventName string     `json:"eventName"`
	Date      string     `json:"date"`
	Cells     []GridCell `json:"cells"`
}

type AttendanceRecord struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Date   string `json:"date"`
	Status string `json:"status"`
	Obs    string `json:"obs"`
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
	http.HandleFunc("/api/shifts/list", handleListShifts)
	http.HandleFunc("/api/shifts/book", handleBookShift)
	http.HandleFunc("/api/shifts/save-grid", handleSaveGrid)
	http.HandleFunc("/api/shifts/clear", handleClearShift)
	http.HandleFunc("/api/shifts/select-user", handleSelectUser)
	http.HandleFunc("/api/shifts/delete-grid", handleDeleteGrid)
	http.HandleFunc("/api/attendance/members", handleAttendanceMembers)
	http.HandleFunc("/api/attendance/user", handleUserAttendance)
	http.HandleFunc("/api/attendance/save", handleSaveAttendance)
	http.HandleFunc("/api/attendance/justify", handleJustifyAbsence)
	http.HandleFunc("/api/attendance/admin-list", handleAdminAttendanceList)
	http.HandleFunc("/api/attendance/validate", handleValidateAttendance)
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
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "Dados inválidos", 400)
		return
	}

	// Criptografia da password
	hash, _ := bcrypt.GenerateFromPassword([]byte(u.Password), 10)

	// AGORA: Usamos u.Role (que vem do formulário) em vez de escrever "COORDENADOR"
	query := "INSERT INTO users (name, email, password, role, department) VALUES ($1, $2, $3, $4, $5)"
	_, err := db.Exec(query, u.Name, u.Email, string(hash), u.Role, u.Department)

	if err != nil {
		log.Printf("Erro no registo: %v", err)
		http.Error(w, "Erro ao criar conta. O email já pode estar em uso.", 400)
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
	if r.Method != http.MethodPost {
		return
	}
	var data struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&data)

	_, err := db.Exec("UPDATE requests SET status = $1 WHERE id = $2", data.Status, data.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
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
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Dados inválidos", 400)
		return
	}

	// Agora permite ADMIN ou LOGISTICA
	role := data.AdminRole
	if role == "LOGISTICA" || role == "ADMIN" {
		_, err := db.Exec("DELETE FROM users WHERE id = $1", data.TargetID)
		if err != nil {
			log.Printf("Erro ao eliminar utilizador: %v", err)
			http.Error(w, "Erro ao eliminar da base de dados", 500)
			return
		}

		log.Printf("Utilizador %d eliminado por %s", data.TargetID, role)
		w.WriteHeader(http.StatusOK) // Envia 200 OK para o frontend
		return
	}

	http.Error(w, "Não tens permissão para eliminar utilizadores", 403)
}

func seedDatabase() {
	hash, _ := bcrypt.GenerateFromPassword([]byte("Romdsc__18"), 10)
	db.Exec("INSERT INTO users (name, email, password, role, department) VALUES ($1, $2, $3, 'ADMIN', 'Marketing') ON CONFLICT (email) DO NOTHING",
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

func handleListShifts(w http.ResponseWriter, r *http.Request) {
	// Usamos um LEFT JOIN para trazer os slots e as inscrições juntas numa só viagem à BD
	query := `
        SELECT s.id, s.row_idx, s.col_idx, s.content, s.is_slot, s.event_name, s.event_date,
               e.user_name, e.selected
        FROM shift_slots s
        LEFT JOIN shift_enrollments e ON s.id = e.slot_id
        ORDER BY s.row_idx, s.col_idx`

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type slotData struct {
		ID      int                      `json:"id"`
		R       int                      `json:"r"`
		C       int                      `json:"c"`
		Content string                   `json:"content"`
		IsSlot  bool                     `json:"isSlot"`
		Users   []map[string]interface{} `json:"users"`
	}

	slotsMap := make(map[int]*slotData)
	var eventName, eventDate string
	var order []int

	for rows.Next() {
		var id, r, c int
		var content, en, ed string
		var isSlot bool
		var uName sql.NullString
		var sel sql.NullBool

		rows.Scan(&id, &r, &c, &content, &isSlot, &en, &ed, &uName, &sel)
		eventName, eventDate = en, ed

		if _, ok := slotsMap[id]; !ok {
			slotsMap[id] = &slotData{ID: id, R: r, C: c, Content: content, IsSlot: isSlot, Users: []map[string]interface{}{}}
			order = append(order, id)
		}
		if uName.Valid {
			slotsMap[id].Users = append(slotsMap[id].Users, map[string]interface{}{
				"name": uName.String, "selected": sel.Bool,
			})
		}
	}

	var finalSlots []interface{}
	for _, id := range order {
		finalSlots = append(finalSlots, slotsMap[id])
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"eventName": eventName,
		"date":      eventDate,
		"slots":     finalSlots,
	})
}

func handleBookShift(w http.ResponseWriter, r *http.Request) {
	var data struct {
		ID   int    `json:"id"`
		User string `json:"userName"`
	}
	json.NewDecoder(r.Body).Decode(&data)

	// Usamos ON CONFLICT para evitar erros se ele clicar duas vezes rápido
	query := `INSERT INTO shift_enrollments (slot_id, user_name) 
              VALUES ($1, $2) 
              ON CONFLICT DO NOTHING`

	_, err := db.Exec(query, data.ID, data.User)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(200)
}

func handleSaveGrid(w http.ResponseWriter, r *http.Request) {
	var data GridPayload
	json.NewDecoder(r.Body).Decode(&data)

	tx, _ := db.Begin() // Inicia transação para ser super rápido

	tx.Exec("DELETE FROM shift_enrollments")
	tx.Exec("DELETE FROM shift_slots")

	stmt, _ := tx.Prepare("INSERT INTO shift_slots (row_idx, col_idx, content, is_slot, event_name, event_date, col_span) VALUES ($1, $2, $3, $4, $5, $6, $7)")

	for _, cell := range data.Cells {
		stmt.Exec(cell.Row, cell.Col, cell.Content, cell.IsSlot, data.EventName, data.Date, cell.Span)
	}

	tx.Commit() // Grava tudo de uma vez
	w.WriteHeader(http.StatusOK)
}

func handleClearShift(w http.ResponseWriter, r *http.Request) {
	var data struct {
		ID   int    `json:"id"`
		User string `json:"userName"`
	}
	json.NewDecoder(r.Body).Decode(&data)

	// Apaga apenas a inscrição deste utilizador neste slot
	_, err := db.Exec("DELETE FROM shift_enrollments WHERE slot_id = $1 AND user_name = $2", data.ID, data.User)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleSelectUser(w http.ResponseWriter, r *http.Request) {
	var data struct {
		SlotID   int    `json:"slotId"`
		UserName string `json:"userName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Erro no JSON", 400)
		return
	}

	// O NOT selected é a forma mais rápida de alternar o estado
	query := "UPDATE shift_enrollments SET selected = NOT selected WHERE slot_id = $1 AND user_name = $2"

	_, err := db.Exec(query, data.SlotID, data.UserName)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleDeleteGrid(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	// 1. Apaga primeiro as inscrições porque elas dependem dos slots
	_, err := db.Exec("DELETE FROM shift_enrollments")
	if err != nil {
		log.Printf("Erro ao apagar inscrições: %v", err)
	}

	// 2. Apaga os slots
	_, err = db.Exec("DELETE FROM shift_slots")
	if err != nil {
		log.Printf("Erro ao apagar slots: %v", err)
	}

	// 3. Reinicia o contador para novos postos começarem no ID 1
	db.Exec("ALTER SEQUENCE shift_slots_id_seq RESTART WITH 1")

	w.WriteHeader(http.StatusOK)
}

func handleAttendanceMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	dept := r.URL.Query().Get("dept")
	if dept == "" {
		http.Error(w, "Departamento não especificado", http.StatusBadRequest)
		return
	}

	// Procura membros filtrando por departamento
	rows, err := db.Query("SELECT id, name, email FROM users WHERE department = $1 ORDER BY name ASC", dept)
	if err != nil {
		http.Error(w, "Erro ao aceder à base de dados", 500)
		return
	}
	defer rows.Close()

	var members []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			continue
		}
		members = append(members, u)
	}

	// Garante que devolve [] em vez de null se não houver ninguém
	if members == nil {
		members = []User{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

func handleUserAttendance(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		http.Error(w, "ID em falta", 400)
		return
	}

	// Consulta atualizada com "id"
	rows, err := db.Query("SELECT id, meeting_title, meeting_date, status, observations FROM attendance_records WHERE user_id = $1", userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var history []AttendanceRecord
	for rows.Next() {
		var a AttendanceRecord
		// Scan agora inclui o ID
		if err := rows.Scan(&a.ID, &a.Title, &a.Date, &a.Status, &a.Obs); err != nil {
			continue
		}
		history = append(history, a)
	}

	if history == nil {
		history = []AttendanceRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func handleSaveAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", 405)
		return
	}

	// Estrutura para receber os dados do frontend
	var data struct {
		Title   string `json:"title"`
		Date    string `json:"date"`
		Records []struct {
			UserID int    `json:"userId"`
			Status string `json:"status"`
			Obs    string `json:"obs"`
		} `json:"records"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Erro no JSON", 400)
		return
	}

	// Iniciar uma transação para guardar tudo de uma vez
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Erro no servidor", 500)
		return
	}

	for _, rec := range data.Records {
		_, err := tx.Exec(`
			INSERT INTO attendance_records (user_id, meeting_title, meeting_date, status, observations)
			VALUES ($1, $2, $3, $4, $5)`,
			rec.UserID, data.Title, data.Date, rec.Status, rec.Obs)

		if err != nil {
			tx.Rollback()
			http.Error(w, "Erro ao guardar registo", 500)
			return
		}
	}

	tx.Commit()
	w.WriteHeader(http.StatusOK)
}
func handleJustifyAbsence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	var data struct {
		AbsenceID int    `json:"absenceId"`
		Reason    string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&data)

	query := "UPDATE attendance_records SET status = 'PENDENTE_JUSTIFICACAO', observations = $1 WHERE id = $2"
	_, err := db.Exec(query, data.Reason, data.AbsenceID)
	if err != nil {
		http.Error(w, "Erro ao justificar", 500)
		return
	}
	w.WriteHeader(200)
}

// Função para listar a assiduidade (agora com suporte para a visão global da Logística)
func handleAdminAttendanceList(w http.ResponseWriter, r *http.Request) {
	dept := r.URL.Query().Get("dept")

	var rows *sql.Rows
	var err error

	// Se for "ALL", a Logística/Admin vê tudo. Caso contrário, filtra pelo departamento.
	if dept == "ALL" {
		rows, err = db.Query(`
            SELECT a.id, u.name, a.meeting_title, a.meeting_date, a.status, a.observations, u.department 
            FROM attendance_records a
            JOIN users u ON a.user_id = u.id
            ORDER BY a.meeting_date DESC`)
	} else {
		rows, err = db.Query(`
            SELECT a.id, u.name, a.meeting_title, a.meeting_date, a.status, a.observations, u.department 
            FROM attendance_records a
            JOIN users u ON a.user_id = u.id
            WHERE u.department = $1
            ORDER BY a.meeting_date DESC`, dept)
	}

	if err != nil {
		http.Error(w, "Erro ao listar assiduidade", 500)
		return
	}
	defer rows.Close()

	var list []map[string]interface{}
	for rows.Next() {
		var id int
		var name, title, date, status, obs, uDept string
		// O Scan deve coincidir com o número de colunas da Query (7 colunas)
		if err := rows.Scan(&id, &name, &title, &date, &status, &obs, &uDept); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "name": name, "title": title, "date": date,
			"status": status, "obs": obs, "department": uDept,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// Função que faltava e causava o erro de compilação
func handleValidateAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", 405)
		return
	}
	var data struct {
		ID       int  `json:"id"`
		Accepted bool `json:"accepted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Dados inválidos", 400)
		return
	}

	novoStatus := "FALTA_JUSTIFICADA"
	if !data.Accepted {
		novoStatus = "FALTA_INJUSTIFICADA"
	}

	_, err := db.Exec("UPDATE attendance_records SET status = $1 WHERE id = $2", novoStatus, data.ID)
	if err != nil {
		http.Error(w, "Erro ao atualizar base de dados", 500)
		return
	}
	w.WriteHeader(200)
}
