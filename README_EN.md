# HostMan 🖥️

[🇨🇳 中文](README.md)

Lightweight Host Management System — Subscription Tracking · Resource Monitoring · Alert Notifications · CLI Tool

> Pure Go, single-binary deployment, SQLite storage, zero external dependencies.

## 📸 Screenshots

### Dashboard

![Dashboard](snapshot/主界面.png)

### Host Details

| System Info | Disk / Network / CPU / Memory |
|:---:|:---:|
| ![System Info](snapshot/详细信息-系统信息.png) | ![Disk Network CPU Memory](snapshot/详细信息-磁盘网络CPU内存.png) |

| Resource Trends | Processes / Alerts |
|:---:|:---:|
| ![Resource Trends](snapshot/详细信息-资源趋势.png) | ![Processes and Alerts](snapshot/详细信息-进程和告警.png) |

## ✨ Features

### 📋 Host Management
- Full CRUD for host info (name, IP, provider, plan, notes)
- Subscription tracking (cost, billing cycle, subscribe/expire dates)
- Monthly cost aggregation (auto-converts monthly/quarterly/yearly)
- Export to CSV / JSON

### 📊 Resource Monitoring
- Agent auto-collects: CPU / Memory / Disk / Network / Load / Processes
- Service status: systemd services + Docker containers
- Static host info: OS, kernel, CPU model, architecture
- Offline detection (heartbeat timeout)
- Automatic metric purging (default: 7 days)

### 📈 Data Visualization
- Historical resource trend charts (Chart.js)
- Selectable time ranges: 6h / 24h / 3d / 7d
- Four dimensions: CPU, Memory, Disk, Load

### 🔔 Alert System
- Auto-detection: CPU / Memory / Disk threshold breach, subscription expiry
- Configurable thresholds (via Web settings)
- Auto-resolve when conditions recover
- Deduplication (no duplicate alerts for same type)

### 📱 Telegram Notifications
- Automatic alert push to Telegram
- Configure Bot Token / Chat ID via Web UI
- One-click test message

### 🖥️ CLI Tool
- `hostman-cli status` — Dashboard overview
- `hostman-cli list` — List hosts (with resource status)
- `hostman-cli show <name>` — Host details
- `hostman-cli alerts` — Active alerts
- `hostman-cli export csv|json` — Export

### 🔐 Security
- Login authentication (bcrypt + Cookie Session)
- HTTPS support (self-signed / Let's Encrypt)
- API Key auth (Agent reporting)
- Admin API Token auth (CLI access)

## 🏗️ Architecture

```
┌──────────────┐     HTTPS :443      ┌──────────────────┐
│   Browser     │◄──────────────────►│   Server          │
└──────────────┘                     │  · Web Dashboard  │
                                     │  · REST API       │
┌──────────────┐     HTTP :8080      │  · Alert Engine   │
│ Agent (local) │───────────────────►│  · SQLite Store   │
└──────────────┘     localhost       └──────────────────┘
                                           ▲
┌──────────────┐     HTTPS :443            │
│ Agent (remote)│──────────────────────────┘
└──────────────┘     insecure mode

┌──────────────┐     Admin API
│  CLI Tool     │──────────────────────────►
└──────────────┘     Bearer Token
```

## 🚀 Quick Start

### Requirements

- Go 1.22+ (build)
- GCC (CGO required for SQLite)
- Linux (Agent reads `/proc`)

### Build

```bash
git clone https://github.com/hjw21century/HostMan.git
cd HostMan

# Build all
make build

# Or build individually
CGO_ENABLED=1 go build -o bin/hostman-server ./cmd/server
CGO_ENABLED=1 go build -o bin/hostman-agent  ./cmd/agent
CGO_ENABLED=0 go build -o bin/hostman-cli    ./cmd/cli
```

### Deploy Server

```bash
# Create directories
mkdir -p /opt/hostman/data /opt/hostman/tls /opt/hostman/web/templates

# Copy files
cp bin/hostman-server /opt/hostman/
cp web/templates/*.html /opt/hostman/web/templates/

# Generate self-signed cert (optional, for HTTPS)
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout /opt/hostman/tls/key.pem -out /opt/hostman/tls/cert.pem \
  -days 3650 -nodes -subj "/CN=HostMan" \
  -addext "subjectAltName=IP:YOUR_SERVER_IP"

# Start
/opt/hostman/hostman-server \
  -db /opt/hostman/data/hostman.db \
  -templates /opt/hostman/web/templates \
  -tls-cert /opt/hostman/tls/cert.pem \
  -tls-key /opt/hostman/tls/key.pem
```

Default admin account: `admin` / `admin` (change password immediately after login)

### Deploy Agent

**Local Agent (same host as Server):**

```bash
cp bin/hostman-agent /opt/hostman/

# Create config
mkdir -p /etc/hostman
cat > /etc/hostman/agent.json << 'EOF'
{
  "server": "http://127.0.0.1:8080",
  "api_key": "get-from-web-ui",
  "interval": 60,
  "insecure": false
}
EOF

/opt/hostman/hostman-agent
```

**Remote Agent (other machines):**

```bash
scp bin/hostman-agent target:/opt/hostman/

# Remote config (insecure: true for self-signed certs)
cat > /etc/hostman/agent.json << 'EOF'
{
  "server": "https://YOUR_SERVER_IP",
  "api_key": "get-from-web-ui",
  "interval": 60,
  "insecure": true
}
EOF
```

### Configure CLI

```bash
cp bin/hostman-cli /usr/local/bin/

# Interactive setup
hostman-cli config
# Server URL: https://YOUR_IP
# API Token: generate from Web Settings page
# Skip TLS verify: y (for self-signed certs)
```

## ⚙️ Configuration

### Server Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8080` | HTTP listen address |
| `-db` | `hostman.db` | SQLite database path |
| `-templates` | auto-detect | Template directory |
| `-tls-cert` | - | TLS certificate path |
| `-tls-key` | - | TLS private key path |
| `-offline-timeout` | `3m` | Heartbeat timeout (mark offline) |
| `-purge-age` | `168h` | Metric retention period |
| `-debug` | `false` | Debug mode |

### Agent Config

File: `/etc/hostman/agent.json`

```json
{
  "server": "http://127.0.0.1:8080",
  "api_key": "your-api-key",
  "interval": 60,
  "insecure": false
}
```

### CLI Commands

Config file: `~/.hostman-cli.json`

```
hostman-cli config              Configure server and token
hostman-cli status              Dashboard overview
hostman-cli list                List all hosts
hostman-cli show <ID|name>      Host details
hostman-cli alerts              Active alerts
hostman-cli export [csv|json]   Export host data
```

## 📡 API Reference

### Agent Report

```bash
curl -X POST https://server/api/v1/report \
  -H "X-API-Key: YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "metric": {
      "cpu_percent": 45.2,
      "mem_total": 8589934592,
      "mem_used": 4294967296,
      "disk_total": 107374182400,
      "disk_used": 53687091200,
      "load_1": 1.5, "load_5": 1.2, "load_15": 0.9,
      "net_in": 1048576, "net_out": 524288,
      "uptime": 864000
    },
    "services": [
      {"name": "nginx", "type": "systemd", "status": "running"},
      {"name": "redis", "type": "docker", "status": "running"}
    ]
  }'
```

### Admin API (CLI)

All Admin API endpoints require `Authorization: Bearer <token>` header.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/admin/status` | Dashboard summary |
| GET | `/api/v1/admin/hosts` | Host list (with latest metrics) |
| GET | `/api/v1/admin/hosts/:id` | Host details |
| GET | `/api/v1/admin/alerts` | Active alerts |
| GET | `/api/v1/admin/export?format=csv` | Export host data |

### Public API

```bash
curl https://server/api/v1/hosts
curl https://server/api/v1/hosts/1
```

## 📁 Project Structure

```
hostman/
├── cmd/
│   ├── server/main.go        # Server entry
│   ├── agent/main.go         # Agent entry
│   └── cli/main.go           # CLI entry
├── internal/
│   ├── model/model.go        # Data models
│   ├── store/sqlite.go       # SQLite data layer
│   ├── handler/handler.go    # HTTP handlers + API
│   ├── agent/collector.go    # Resource collector
│   ├── middleware/auth.go    # Auth middleware
│   └── notify/telegram.go   # Telegram notifications
├── web/templates/            # HTML templates (dark theme)
│   ├── layout.html           # Base layout
│   ├── dashboard.html        # Dashboard
│   ├── hosts.html            # Host list
│   ├── host_detail.html      # Host detail + charts
│   ├── host_form.html        # Host form
│   ├── alerts.html           # Alert records
│   ├── settings.html         # System settings
│   ├── change_password.html  # Change password
│   └── login.html            # Login page
├── snapshot/                 # Screenshots
├── Makefile
├── go.mod
└── README.md
```

## 🛠️ Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go |
| Web Framework | Gin |
| Database | SQLite (go-sqlite3) |
| Charts | Chart.js |
| Password Hash | bcrypt |
| Notifications | Telegram Bot API |
| TLS | crypto/tls |

## 📝 Roadmap

- [x] **Phase 1**: Server + SQLite + Host CRUD + Subscription Management + Web Dashboard
- [x] **Phase 2**: Agent Resource Collection + Heartbeat Reporting + Service Monitoring
- [x] **Phase 3**: Historical Resource Charts + Trend Analysis (Chart.js)
- [x] **Phase 4**: Alert System + Telegram Notifications
- [x] **Phase 5**: CLI Tool + Admin API
- [ ] **Phase 6**: Multi-user Role Management
- [ ] **Phase 7**: Host Groups / Tags
- [ ] **Phase 8**: Custom Monitors + Plugin System

## 📄 License

MIT
