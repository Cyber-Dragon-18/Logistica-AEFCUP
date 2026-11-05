# Logistics App for AEFCUP

A web application to manage the logistics of a student association: service requests, shift scheduling, attendance and a photo gallery, with different permissions for each role.

I built it for AEFCUP at the Faculty of Sciences of the University of Porto, as a small, self-contained tool for real events.

## Features

- **Accounts and roles:** registration and login with hashed passwords (bcrypt), with roles such as admin and logistics
- **Service requests:** departments create requests for an event, and the logistics team tracks and updates their status
- **Email notifications:** the logistics team gets an email whenever a new request is created
- **Shift scheduling:** build a shift grid for an event, let members sign up for slots, and mark unavailability
- **Attendance:** record meeting attendance per department, with absence justifications and admin validation
- **Photo gallery:** albums with cover images, stored on Cloudinary
- **User profiles:** name, password and profile photo
- **Shutdown mode:** an admin-triggered switch that wipes the data and turns the app into a closing page, for apps that are meant to live for a single season

## Built with

- **Backend:** Go (`net/http`, no framework)
- **Database:** PostgreSQL (hosted on Neon)
- **Frontend:** vanilla HTML, CSS and JavaScript served from `public/`
- **Services:** Cloudinary for images, Gmail SMTP for email
- **Hosting:** Render

## Project structure

```
.
├── main.go        HTTP server, API handlers and database setup
├── schema.sql     database schema
├── go.mod         Go module and dependencies
└── public/        static frontend (HTML, CSS, JS)
```

## Getting started

You need Go and a PostgreSQL database (a free Neon database works well).

1. **Clone the repository**
   ```bash
   git clone git@github.com:Cyber-Dragon-18/Logistica-AEFCUP.git
   cd Logistica-AEFCUP
   ```

2. **Create the tables** by running `schema.sql` on your database.

3. **Set the environment variables** (see the table below). For example:
   ```bash
   export DATABASE_URL="postgresql://USER:PASSWORD@HOST/DBNAME?sslmode=require"
   ```

4. **Run the server**
   ```bash
   go run .
   ```

5. Open http://localhost:8080.

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | yes | PostgreSQL connection string |
| `PORT` | no | Port to listen on (default `8080`) |
| `EMAIL_USER` | no | Gmail address that sends the notifications |
| `EMAIL_PASS` | no | App password for that Gmail account |
| `CLOUDINARY_URL` | no | Cloudinary credentials, needed for the photo gallery |

Without `EMAIL_USER` and `EMAIL_PASS` the app still works and just skips the email notifications.

## Deployment

The app is deployed on Render as a Go web service. Set the environment variables above in the service settings. Never commit credentials to the repository.

## Author

Rodrigo Coelho · [GitHub](https://github.com/Cyber-Dragon-18) · [Website](https://cyber-dragon-18.github.io)
