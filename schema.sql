CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    role TEXT CHECK(role IN ('COORDENADOR', 'LOGISTICA')) DEFAULT 'COORDENADOR',
    department TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT DEFAULT 'PENDING',
    author_id INTEGER,
    department TEXT NOT NULL,    
    service_type TEXT NOT NULL,  
    event_name TEXT,             
    event_date TEXT,             
    observations TEXT, 

    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(author_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS shift_tables (
    id SERIAL PRIMARY KEY, 
    event_name TEXT NOT NULL,
    shift_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS shift_slots (
    id SERIAL PRIMARY KEY,
    time_range TEXT,
    category TEXT,
    label TEXT,
    user_name TEXT,
    event_name TEXT,
    event_date TEXT
);