# CorvusMiner Control Panel

A modern web-based control panel for managing mining operations, built with Go and PostgreSQL.

## Features

- **Dashboard**: Real-time overview of mining operations and statistics
- **Miner Management**: Add, remove, and monitor mining devices
- **Configuration**: Centralized settings for pool connection and mining parameters
- **Responsive Design**: Works seamlessly on desktop and mobile devices
- **RESTful API**: JSON endpoints for programmatic access
- **Real-time Updates**: Dynamic data refresh without page reloads

## Project Structure

```
Panel/
├── main.go                 # Application entry point
├── go.mod                  # Go module dependencies
├── handlers/
│   └── handlers.go        # HTTP request handlers
├── models/
│   └── models.go          # Data models (Miner, Config)
├── database/
│   └── db.go              # PostgreSQL database operations
├── templates/
│   ├── layout.html        # Base template
│   ├── dashboard.html     # Dashboard page
│   ├── miners.html        # Miner management page
│   └── config.html        # Configuration page
├── static/
│   ├── css/
│   │   └── style.css      # Stylesheet
│   └── js/
│       └── main.js        # Frontend JavaScript
```

## Prerequisites

- Go 1.21 or later
- PostgreSQL 14 or later
- Git

## Installation

1. Clone the repository or navigate to the project directory:
```bash
cd Panel
```

2. Download Go dependencies:
```bash
go mod download
```

3. Build the project:
```bash
go build -o corvusminer-panel
```

4. Start PostgreSQL. When the panel starts without `DATABASE_URL`, it prompts for:

- User (defaults to `postgres`)
- Password (hidden in an interactive terminal)
- Host or IP address (defaults to `localhost`)
- Port (defaults to `5432`)
- Database name (defaults to `corvus`)
- SSL mode (defaults to `disable`)

If the selected database does not exist, the panel connects to PostgreSQL's `postgres` maintenance database and creates it. It then creates the tables and inserts the default configuration automatically. The selected user must have `CREATEDB` permission for this first launch.

For unattended deployments, set the complete connection string instead:

PowerShell:
```powershell
$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/corvus?sslmode=disable"
```

Bash:
```bash
export DATABASE_URL="postgres://postgres:password@localhost:5432/corvus?sslmode=disable"
```

## Running the Application

```bash
./corvusminer-panel
```

Enter the PostgreSQL connection settings when prompted. The application will then start on `http://localhost:8080`.

### For Development (with auto-reload)

Install air for hot-reloading:
```bash
go install github.com/cosmtrek/air@latest
air
```

## Usage

### Dashboard
Access the main dashboard at `http://localhost:8080/` to view:
- Total number of miners
- Pool configuration
- Recent miner statistics

### Miner Management
Navigate to `/miners` to:
- View all configured miners
- Add new miners (name, IP, port)
- Delete miners
- Monitor real-time status

### Configuration
Navigate to `/config` to:
- Set pool URL (e.g., `stratum+tcp://pool.example.com:3333`)
- Configure worker name
- Set difficulty target
- Enable/disable SSL/TLS
- Adjust update intervals

## API Endpoints

### Miners
- `GET /api/miners` - Get all miners
- `POST /api/miners/add` - Add new miner
- `POST /api/miners/delete` - Delete miner

### Configuration
- `GET /api/config/get` - Get current configuration
- `POST /api/config/update` - Update configuration

### Pages
- `GET /` - Dashboard
- `GET /miners` - Miner list page
- `GET /config` - Configuration page

## Database Schema

### Miners Table
```sql
CREATE TABLE IF NOT EXISTS miners (
    id BIGSERIAL PRIMARY KEY,
    device_hash TEXT NOT NULL UNIQUE,
    pc_username TEXT NOT NULL,
    status TEXT DEFAULT 'online',
    last_seen INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Config Table
```sql
CREATE TABLE IF NOT EXISTS config (
    id BIGSERIAL PRIMARY KEY,
    cpu_config TEXT,
    gpu_config TEXT,
    gpu_algo TEXT DEFAULT '',
    enable_cpu INTEGER DEFAULT 1,
    enable_gpu INTEGER DEFAULT 1,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Extending the Project

### Adding New Handlers
Add new handler functions in `handlers/handlers.go`:
```go
func (h *Handler) NewPage(w http.ResponseWriter, r *http.Request) {
    // Implementation
}
```

### Adding New Database Operations
Extend `database/db.go` with additional query methods:
```go
func (db *DB) NewOperation() error {
    // Implementation
}
```

### Adding New Templates
Create new HTML templates in `templates/` directory and register them in handlers.

### Styling
Modify `static/css/style.css` or add new CSS files and link them in `templates/layout.html`.

## Development Notes

- The application uses Go's `html/template` package for templating
- PostgreSQL tables are automatically created on first run
- The PostgreSQL driver is pgx and does not require CGO
- Static files are served with proper MIME types
- All API responses use JSON format
- CORS is not configured (for internal use)

## Troubleshooting

### Database Issues
Verify that PostgreSQL is reachable and that `DATABASE_URL` includes the correct user, password, host, port, and database. Use `sslmode=require` for hosted databases unless your provider specifies another TLS mode.

### Port Already in Use
If port 8080 is in use, modify `main.go` to use a different port:
```go
log.Fatal(http.ListenAndServe(":YOUR_PORT", nil))
```

### Static Files Not Loading
Ensure you run the application from the project directory where the `static/` folder exists.

## License

Proprietary - CorvusMiner 2025

## Support

For issues or feature requests, please contact the development team.
