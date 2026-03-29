package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"

	"hostman/internal/model"
)

// DB wraps the SQLite connection.
type DB struct {
	conn *sql.DB
}

// New opens (or creates) the SQLite database and runs migrations.
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS hosts (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			name          TEXT NOT NULL,
			ip            TEXT NOT NULL DEFAULT '',
			provider      TEXT NOT NULL DEFAULT '',
			plan          TEXT NOT NULL DEFAULT '',
			cost          REAL NOT NULL DEFAULT 0,
			currency      TEXT NOT NULL DEFAULT 'USD',
			billing_cycle TEXT NOT NULL DEFAULT 'monthly',
			subscribe_at  DATETIME,
			expire_at     DATETIME,
			note          TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'unknown',
			api_key       TEXT NOT NULL DEFAULT '',
			hostname      TEXT NOT NULL DEFAULT '',
			os            TEXT NOT NULL DEFAULT '',
			arch          TEXT NOT NULL DEFAULT '',
			kernel        TEXT NOT NULL DEFAULT '',
			distro        TEXT NOT NULL DEFAULT '',
			cpu_model     TEXT NOT NULL DEFAULT '',
			cpu_cores     INTEGER NOT NULL DEFAULT 0,
			last_seen_at  DATETIME,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS metrics (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			host_id      INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
			cpu_percent  REAL NOT NULL DEFAULT 0,
			mem_total    INTEGER NOT NULL DEFAULT 0,
			mem_used     INTEGER NOT NULL DEFAULT 0,
			disk_total   INTEGER NOT NULL DEFAULT 0,
			disk_used    INTEGER NOT NULL DEFAULT 0,
			net_in       INTEGER NOT NULL DEFAULT 0,
			net_out      INTEGER NOT NULL DEFAULT 0,
			load_1       REAL NOT NULL DEFAULT 0,
			load_5       REAL NOT NULL DEFAULT 0,
			load_15      REAL NOT NULL DEFAULT 0,
			uptime       INTEGER NOT NULL DEFAULT 0,
			collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_host_time ON metrics(host_id, collected_at DESC)`,
		`CREATE TABLE IF NOT EXISTS services (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			host_id    INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			type       TEXT NOT NULL DEFAULT 'systemd',
			status     TEXT NOT NULL DEFAULT 'unknown',
			checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			host_id    INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
			type       TEXT NOT NULL,
			message    TEXT NOT NULL,
			resolved   INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'admin',
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// Migrations for existing databases
		`CREATE INDEX IF NOT EXISTS idx_hosts_api_key ON hosts(api_key)`,
		`CREATE INDEX IF NOT EXISTS idx_hosts_status ON hosts(status)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
	// Safe column additions for existing databases
	alterStmts := []string{
		`ALTER TABLE metrics ADD COLUMN mem_buffered INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE metrics ADD COLUMN mem_cached INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE metrics ADD COLUMN swap_total INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE metrics ADD COLUMN swap_used INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE metrics ADD COLUMN process_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE metrics ADD COLUMN extra_data TEXT NOT NULL DEFAULT '{}'`,
	}
	for _, s := range alterStmts {
		db.conn.Exec(s) // ignore errors (column may already exist)
	}
	for _, s := range stmts {
		if _, err := db.conn.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s[:40], err)
		}
	}
	return nil
}

// ---------- Hosts ----------

func (db *DB) CreateHost(h *model.Host) error {
	res, err := db.conn.Exec(`
		INSERT INTO hosts (name, ip, provider, plan, cost, currency, billing_cycle, subscribe_at, expire_at, note, status, api_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.Name, h.IP, h.Provider, h.Plan, h.Cost, h.Currency, h.BillingCycle,
		h.SubscribeAt, h.ExpireAt, h.Note, h.Status, "",
	)
	if err != nil {
		return err
	}
	h.ID, _ = res.LastInsertId()
	return nil
}

func (db *DB) UpdateHost(h *model.Host) error {
	_, err := db.conn.Exec(`
		UPDATE hosts SET name=?, ip=?, provider=?, plan=?, cost=?, currency=?,
		billing_cycle=?, subscribe_at=?, expire_at=?, note=?, status=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		h.Name, h.IP, h.Provider, h.Plan, h.Cost, h.Currency,
		h.BillingCycle, h.SubscribeAt, h.ExpireAt, h.Note, h.Status, h.ID,
	)
	return err
}

func (db *DB) DeleteHost(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM hosts WHERE id=?`, id)
	return err
}

func (db *DB) GetHost(id int64) (*model.Host, error) {
	h := &model.Host{}
	err := db.conn.QueryRow(`
		SELECT id, name, ip, provider, plan, cost, currency, billing_cycle,
		       subscribe_at, expire_at, note, status, created_at, updated_at
		FROM hosts WHERE id=?`, id).Scan(
		&h.ID, &h.Name, &h.IP, &h.Provider, &h.Plan, &h.Cost, &h.Currency,
		&h.BillingCycle, &h.SubscribeAt, &h.ExpireAt, &h.Note, &h.Status,
		&h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (db *DB) ListHosts() ([]model.Host, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, ip, provider, plan, cost, currency, billing_cycle,
		       subscribe_at, expire_at, note, status, created_at, updated_at
		FROM hosts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []model.Host
	for rows.Next() {
		var h model.Host
		if err := rows.Scan(
			&h.ID, &h.Name, &h.IP, &h.Provider, &h.Plan, &h.Cost, &h.Currency,
			&h.BillingCycle, &h.SubscribeAt, &h.ExpireAt, &h.Note, &h.Status,
			&h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// ListHostsWithMetrics returns all hosts with their latest metric.
func (db *DB) ListHostsWithMetrics() ([]model.HostWithMetric, error) {
	hosts, err := db.ListHosts()
	if err != nil {
		return nil, err
	}
	result := make([]model.HostWithMetric, len(hosts))
	for i, h := range hosts {
		result[i].Host = h
		m, err := db.LatestMetric(h.ID)
		if err == nil {
			result[i].LatestMetric = m
		}
	}
	return result, nil
}

// ExpiringHosts returns hosts expiring within the given duration.
func (db *DB) ExpiringHosts(within time.Duration) ([]model.Host, error) {
	deadline := time.Now().Add(within)
	rows, err := db.conn.Query(`
		SELECT id, name, ip, provider, plan, cost, currency, billing_cycle,
		       subscribe_at, expire_at, note, status, created_at, updated_at
		FROM hosts WHERE expire_at IS NOT NULL AND expire_at <= ? ORDER BY expire_at`,
		deadline,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []model.Host
	for rows.Next() {
		var h model.Host
		if err := rows.Scan(
			&h.ID, &h.Name, &h.IP, &h.Provider, &h.Plan, &h.Cost, &h.Currency,
			&h.BillingCycle, &h.SubscribeAt, &h.ExpireAt, &h.Note, &h.Status,
			&h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// ---------- Metrics ----------

func (db *DB) InsertMetric(m *model.Metric) error {
	return db.InsertMetricWithDetail(m, nil)
}

// InsertMetricWithDetail inserts a metric with optional detailed data.
func (db *DB) InsertMetricWithDetail(m *model.Metric, detail *model.DetailedData) error {
	extraJSON := "{}"
	if detail != nil {
		if data, err := json.Marshal(detail); err == nil {
			extraJSON = string(data)
		}
	}
	_, err := db.conn.Exec(`
		INSERT INTO metrics (host_id, cpu_percent, mem_total, mem_used, mem_buffered, mem_cached,
		    swap_total, swap_used, disk_total, disk_used, net_in, net_out,
		    load_1, load_5, load_15, uptime, process_count, extra_data, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.HostID, m.CPUPercent, m.MemTotal, m.MemUsed, m.MemBuffered, m.MemCached,
		m.SwapTotal, m.SwapUsed, m.DiskTotal, m.DiskUsed, m.NetIn, m.NetOut,
		m.Load1, m.Load5, m.Load15, m.Uptime, m.ProcessCount, extraJSON, m.CollectedAt,
	)
	return err
}

func (db *DB) LatestMetric(hostID int64) (*model.Metric, error) {
	m := &model.Metric{}
	err := db.conn.QueryRow(`
		SELECT id, host_id, cpu_percent, mem_total, mem_used,
		       COALESCE(mem_buffered,0), COALESCE(mem_cached,0),
		       COALESCE(swap_total,0), COALESCE(swap_used,0),
		       disk_total, disk_used, net_in, net_out,
		       load_1, load_5, load_15, uptime,
		       COALESCE(process_count,0), collected_at
		FROM metrics WHERE host_id=? ORDER BY collected_at DESC LIMIT 1`, hostID).Scan(
		&m.ID, &m.HostID, &m.CPUPercent, &m.MemTotal, &m.MemUsed,
		&m.MemBuffered, &m.MemCached, &m.SwapTotal, &m.SwapUsed,
		&m.DiskTotal, &m.DiskUsed, &m.NetIn, &m.NetOut,
		&m.Load1, &m.Load5, &m.Load15, &m.Uptime,
		&m.ProcessCount, &m.CollectedAt,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// GetLatestDetail returns the detailed data from the latest metric.
func (db *DB) GetLatestDetail(hostID int64) (*model.DetailedData, error) {
	var extraJSON string
	err := db.conn.QueryRow(`
		SELECT COALESCE(extra_data, '{}') FROM metrics
		WHERE host_id=? ORDER BY collected_at DESC LIMIT 1`, hostID).Scan(&extraJSON)
	if err != nil {
		return nil, err
	}
	var detail model.DetailedData
	if err := json.Unmarshal([]byte(extraJSON), &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// RecentMetrics returns metrics for a host in the given duration.
func (db *DB) RecentMetrics(hostID int64, dur time.Duration) ([]model.Metric, error) {
	since := time.Now().Add(-dur)
	rows, err := db.conn.Query(`
		SELECT id, host_id, cpu_percent, mem_total, mem_used, disk_total, disk_used,
		       net_in, net_out, load_1, load_5, load_15, uptime, collected_at
		FROM metrics WHERE host_id=? AND collected_at>=? ORDER BY collected_at`,
		hostID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []model.Metric
	for rows.Next() {
		var m model.Metric
		if err := rows.Scan(
			&m.ID, &m.HostID, &m.CPUPercent, &m.MemTotal, &m.MemUsed, &m.DiskTotal, &m.DiskUsed,
			&m.NetIn, &m.NetOut, &m.Load1, &m.Load5, &m.Load15, &m.Uptime, &m.CollectedAt,
		); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// PurgeOldMetrics deletes metrics older than the given duration.
func (db *DB) PurgeOldMetrics(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	res, err := db.conn.Exec(`DELETE FROM metrics WHERE collected_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- Services ----------

func (db *DB) UpsertServices(hostID int64, services []model.Service) error {
	// Delete old services for this host and insert fresh
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM services WHERE host_id=?`, hostID); err != nil {
		tx.Rollback()
		return err
	}
	for _, s := range services {
		if _, err := tx.Exec(`
			INSERT INTO services (host_id, name, type, status, checked_at)
			VALUES (?, ?, ?, ?, ?)`,
			hostID, s.Name, s.Type, s.Status, s.CheckedAt,
		); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) ListServices(hostID int64) ([]model.Service, error) {
	rows, err := db.conn.Query(`
		SELECT id, host_id, name, type, status, checked_at
		FROM services WHERE host_id=? ORDER BY name`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []model.Service
	for rows.Next() {
		var s model.Service
		if err := rows.Scan(&s.ID, &s.HostID, &s.Name, &s.Type, &s.Status, &s.CheckedAt); err != nil {
			return nil, err
		}
		services = append(services, s)
	}
	return services, nil
}

// ---------- Alerts ----------

func (db *DB) CreateAlert(a *model.Alert) error {
	// Skip if there's already an active alert of the same type for this host
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM alerts WHERE host_id=? AND type=? AND resolved=0`, a.HostID, a.Type).Scan(&count)
	if count > 0 {
		return nil // already has active alert
	}
	res, err := db.conn.Exec(`
		INSERT INTO alerts (host_id, type, message) VALUES (?, ?, ?)`,
		a.HostID, a.Type, a.Message,
	)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// AutoResolveAlerts resolves alerts when the condition is no longer met.
func (db *DB) AutoResolveAlerts(hostID int64, alertType string) {
	db.conn.Exec(`UPDATE alerts SET resolved=1 WHERE host_id=? AND type=? AND resolved=0`, hostID, alertType)
}

func (db *DB) ResolveAlert(id int64) error {
	_, err := db.conn.Exec(`UPDATE alerts SET resolved=1 WHERE id=?`, id)
	return err
}

func (db *DB) ListAlerts(hostID int64, onlyActive bool) ([]model.Alert, error) {
	q := `SELECT id, host_id, type, message, resolved, created_at FROM alerts WHERE host_id=?`
	if onlyActive {
		q += ` AND resolved=0`
	}
	q += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := db.conn.Query(q, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []model.Alert
	for rows.Next() {
		var a model.Alert
		if err := rows.Scan(&a.ID, &a.HostID, &a.Type, &a.Message, &a.Resolved, &a.CreatedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

// UpdateHostStatus sets the status of a host.
func (db *DB) UpdateHostStatus(id int64, status string) error {
	_, err := db.conn.Exec(`UPDATE hosts SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, id)
	return err
}

// GetHostByAPIKey finds a host by its API key.
func (db *DB) GetHostByAPIKey(key string) (*model.Host, error) {
	if key == "" {
		return nil, fmt.Errorf("empty api key")
	}
	h := &model.Host{}
	err := db.conn.QueryRow(`
		SELECT id, name, ip, provider, plan, cost, currency, billing_cycle,
		       subscribe_at, expire_at, note, status, created_at, updated_at
		FROM hosts WHERE api_key=?`, key).Scan(
		&h.ID, &h.Name, &h.IP, &h.Provider, &h.Plan, &h.Cost, &h.Currency,
		&h.BillingCycle, &h.SubscribeAt, &h.ExpireAt, &h.Note, &h.Status,
		&h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// SetHostAPIKey sets the API key for agent authentication.
func (db *DB) SetHostAPIKey(id int64, key string) error {
	_, err := db.conn.Exec(`UPDATE hosts SET api_key=? WHERE id=?`, key, id)
	return err
}

// GetHostAPIKey returns the API key for a host.
func (db *DB) GetHostAPIKey(id int64) string {
	var key string
	db.conn.QueryRow(`SELECT api_key FROM hosts WHERE id=?`, id).Scan(&key)
	return key
}

// UpdateHostInfo stores static host information from the agent.
func (db *DB) UpdateHostInfo(id int64, info *model.HostInfo) error {
	if info == nil {
		return nil
	}
	_, err := db.conn.Exec(`
		UPDATE hosts SET hostname=?, os=?, arch=?, kernel=?, distro=?, cpu_model=?, cpu_cores=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		info.Hostname, info.OS, info.Arch, info.Kernel, info.Distro, info.CPUModel, info.CPUCores, id,
	)
	return err
}

// UpdateLastSeen updates the last_seen_at timestamp for a host.
func (db *DB) UpdateLastSeen(id int64) error {
	_, err := db.conn.Exec(`UPDATE hosts SET last_seen_at=? WHERE id=?`, time.Now().UTC(), id)
	return err
}

// MarkOfflineHosts marks hosts as offline if they haven't reported within the timeout.
func (db *DB) MarkOfflineHosts(timeout time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-timeout)
	res, err := db.conn.Exec(`
		UPDATE hosts SET status='offline'
		WHERE status='online' AND last_seen_at IS NOT NULL AND last_seen_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- Users ----------

// CreateUser creates a new user with a bcrypt-hashed password.
func (db *DB) CreateUser(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = db.conn.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, string(hash))
	return err
}

// AuthenticateUser checks username/password and returns the user ID if valid.
func (db *DB) AuthenticateUser(username, password string) (int64, error) {
	var id int64
	var hash string
	err := db.conn.QueryRow(`SELECT id, password_hash FROM users WHERE username=?`, username).Scan(&id, &hash)
	if err != nil {
		return 0, fmt.Errorf("user not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return 0, fmt.Errorf("invalid password")
	}
	return id, nil
}

// UserCount returns the number of users in the database.
func (db *DB) UserCount() int {
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count
}

// ChangePassword updates a user's password.
func (db *DB) ChangePassword(userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(`UPDATE users SET password_hash=? WHERE id=?`, string(hash), userID)
	return err
}

// GetHostInfo returns the static host info fields.
func (db *DB) GetHostInfo(id int64) (*model.HostInfo, error) {
	info := &model.HostInfo{}
	err := db.conn.QueryRow(`
		SELECT COALESCE(hostname,''), COALESCE(os,''), COALESCE(arch,''), COALESCE(kernel,''),
		       COALESCE(distro,''), COALESCE(cpu_model,''), COALESCE(cpu_cores,0)
		FROM hosts WHERE id=?`, id).Scan(
		&info.Hostname, &info.OS, &info.Arch, &info.Kernel, &info.Distro, &info.CPUModel, &info.CPUCores,
	)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// ---------- Settings ----------

func (db *DB) GetSetting(key string) string {
	var val string
	db.conn.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&val)
	return val
}

func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (db *DB) GetSettings(keys []string) map[string]string {
	m := make(map[string]string)
	for _, k := range keys {
		m[k] = db.GetSetting(k)
	}
	return m
}

// CountActiveAlerts returns the number of unresolved alerts.
func (db *DB) CountActiveAlerts() int {
	var n int
	db.conn.QueryRow(`SELECT COUNT(*) FROM alerts WHERE resolved=0`).Scan(&n)
	return n
}
