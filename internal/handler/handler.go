package handler

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"strconv"
	"time"

	"hostman/internal/middleware"
	"hostman/internal/model"
	"hostman/internal/store"

	"github.com/gin-gonic/gin"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	DB       *store.DB
	Sessions *middleware.SessionStore
	tmpls    map[string]*template.Template
}

// New creates a Handler and parses templates from the given directory.
func New(db *store.DB, sessions *middleware.SessionStore, tmplDir string) (*Handler, error) {
	funcMap := template.FuncMap{
		"fmtBytes": fmtBytes,
		"fmtPct":   func(v float64) string { return fmt.Sprintf("%.1f%%", v) },
		"fmtTime": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("2006-01-02 15:04")
		},
		"fmtTimePtr": func(t *time.Time) string {
			if t == nil {
				return "-"
			}
			return t.Format("2006-01-02")
		},
		"fmtDate": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"fmtUptime": fmtUptime,
		"fmtCost": func(cost float64, currency string) string {
			return fmt.Sprintf("%s %.2f", currency, cost)
		},
		"statusColor": func(s string) string {
			switch s {
			case "online":
				return "green"
			case "offline":
				return "red"
			default:
				return "gray"
			}
		},
		"daysUntil": func(t *time.Time) string {
			if t == nil {
				return "-"
			}
			d := time.Until(*t).Hours() / 24
			if d < 0 {
				return fmt.Sprintf("已过期 %d 天", int(-d))
			}
			return fmt.Sprintf("%d 天", int(d))
		},
		"daysUntilN": func(t *time.Time) int {
			if t == nil {
				return 9999
			}
			return int(time.Until(*t).Hours() / 24)
		},
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
		"pct": func(used, total uint64) string {
			if total == 0 {
				return "0.0"
			}
			return fmt.Sprintf("%.1f", float64(used)/float64(total)*100)
		},
		"pctF": func(used, total uint64) float64 {
			if total == 0 {
				return 0
			}
			return float64(used) / float64(total) * 100
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(tmplDir + "/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	_ = tmpl // validate parse

	// Build per-page templates to avoid {{define "content"}} collisions
	pages := []string{"dashboard.html", "hosts.html", "host_form.html", "host_detail.html", "change_password.html"}
	layoutFile := tmplDir + "/layout.html"
	tmpls := make(map[string]*template.Template, len(pages)+1)
	for _, page := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFiles(layoutFile, tmplDir+"/"+page)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		tmpls[page] = t
	}

	// Login page (standalone, no layout)
	loginTmpl, err := template.New("login.html").Funcs(funcMap).ParseFiles(tmplDir + "/login.html")
	if err != nil {
		return nil, fmt.Errorf("parse login.html: %w", err)
	}
	tmpls["login.html"] = loginTmpl

	return &Handler{DB: db, Sessions: sessions, tmpls: tmpls}, nil
}

// RegisterRoutes registers all routes on the gin engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Public routes (no auth required)
	r.GET("/login", h.LoginPage)
	r.POST("/login", h.Login)
	r.GET("/logout", h.Logout)

	// Agent API (uses API key auth, not session)
	api := r.Group("/api/v1")
	{
		api.POST("/report", h.AgentReport)
		api.GET("/hosts", h.APIListHosts)
		api.GET("/hosts/:id", h.APIGetHost)
	}

	// Protected web routes (require login)
	auth := r.Group("/", middleware.AuthRequired(h.Sessions))
	{
		auth.GET("/", h.Dashboard)
		auth.GET("/hosts", h.ListHosts)
		auth.GET("/hosts/new", h.NewHostForm)
		auth.POST("/hosts", h.CreateHost)
		auth.GET("/hosts/:id", h.HostDetail)
		auth.GET("/hosts/:id/edit", h.EditHostForm)
		auth.POST("/hosts/:id", h.UpdateHost)
		auth.POST("/hosts/:id/delete", h.DeleteHost)
		auth.POST("/hosts/:id/genkey", h.GenAPIKey)
		auth.GET("/settings/password", h.ChangePasswordPage)
		auth.POST("/settings/password", h.ChangePassword)
		auth.GET("/export/hosts", h.ExportHosts)
	}
}

// ---------- Auth ----------

func (h *Handler) LoginPage(c *gin.Context) {
	// If already logged in, redirect to dashboard
	if cookie, err := c.Cookie(middleware.CookieName); err == nil {
		if _, ok := h.Sessions.Get(cookie); ok {
			c.Redirect(302, "/")
			return
		}
	}
	msg := c.Query("msg")
	c.Header("Content-Type", "text/html; charset=utf-8")
	h.tmpls["login.html"].ExecuteTemplate(c.Writer, "login.html", gin.H{"Msg": msg})
}

func (h *Handler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	userID, err := h.DB.AuthenticateUser(username, password)
	if err != nil {
		c.Redirect(302, "/login?msg=用户名或密码错误")
		return
	}

	token := h.Sessions.Create(userID, username)
	c.SetCookie(middleware.CookieName, token, 86400*7, "/", "", false, true)
	c.Redirect(302, "/")
}

func (h *Handler) Logout(c *gin.Context) {
	if cookie, err := c.Cookie(middleware.CookieName); err == nil {
		h.Sessions.Delete(cookie)
	}
	c.SetCookie(middleware.CookieName, "", -1, "/", "", false, true)
	c.Redirect(302, "/login")
}

func (h *Handler) ChangePasswordPage(c *gin.Context) {
	h.render(c, "change_password.html", gin.H{"Title": "修改密码"})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	sess, _ := c.Get("session")
	session := sess.(*middleware.Session)

	oldPass := c.PostForm("old_password")
	newPass := c.PostForm("new_password")
	confirmPass := c.PostForm("confirm_password")

	if newPass != confirmPass {
		h.render(c, "change_password.html", gin.H{"Title": "修改密码", "Msg": "两次输入的新密码不一致"})
		return
	}
	if len(newPass) < 6 {
		h.render(c, "change_password.html", gin.H{"Title": "修改密码", "Msg": "新密码长度不能少于6位"})
		return
	}

	// Verify old password
	if _, err := h.DB.AuthenticateUser(session.Username, oldPass); err != nil {
		h.render(c, "change_password.html", gin.H{"Title": "修改密码", "Msg": "当前密码错误"})
		return
	}

	// Update password
	if err := h.DB.ChangePassword(session.UserID, newPass); err != nil {
		h.render(c, "change_password.html", gin.H{"Title": "修改密码", "Msg": "修改失败: " + err.Error()})
		return
	}

	h.render(c, "change_password.html", gin.H{"Title": "修改密码", "Msg": "密码修改成功！", "OK": true})
}

// ---------- Web Pages ----------

func (h *Handler) Dashboard(c *gin.Context) {
	hosts, err := h.DB.ListHostsWithMetrics()
	if err != nil {
		c.String(500, "db error: %v", err)
		return
	}

	expiring, _ := h.DB.ExpiringHosts(30 * 24 * time.Hour)

	// Calculate monthly cost (normalize all billing cycles to monthly)
	var monthlyCost float64
	for _, hw := range hosts {
		switch hw.BillingCycle {
		case "monthly":
			monthlyCost += hw.Cost
		case "quarterly":
			monthlyCost += hw.Cost / 3
		case "yearly":
			monthlyCost += hw.Cost / 12
		default:
			monthlyCost += hw.Cost
		}
	}

	data := gin.H{
		"Title":         "仪表板",
		"Hosts":         hosts,
		"TotalHosts":    len(hosts),
		"OnlineHosts":   countByStatus(hosts, "online"),
		"OfflineHosts":  countByStatus(hosts, "offline"),
		"ExpiringHosts": expiring,
		"MonthlyCost":   monthlyCost,
	}
	h.render(c, "dashboard.html", data)
}

func (h *Handler) ListHosts(c *gin.Context) {
	hosts, err := h.DB.ListHostsWithMetrics()
	if err != nil {
		c.String(500, "db error: %v", err)
		return
	}
	h.render(c, "hosts.html", gin.H{"Title": "主机列表", "Hosts": hosts})
}

func (h *Handler) NewHostForm(c *gin.Context) {
	h.render(c, "host_form.html", gin.H{"Title": "添加主机", "Host": &model.Host{Currency: "USD", BillingCycle: "monthly"}})
}

func (h *Handler) CreateHost(c *gin.Context) {
	host, err := h.parseHostForm(c)
	if err != nil {
		c.String(400, "invalid form: %v", err)
		return
	}
	if err := h.DB.CreateHost(host); err != nil {
		c.String(500, "db error: %v", err)
		return
	}
	c.Redirect(302, fmt.Sprintf("/hosts/%d", host.ID))
}

func (h *Handler) HostDetail(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	host, err := h.DB.GetHost(id)
	if err != nil {
		c.String(404, "host not found")
		return
	}
	metric, _ := h.DB.LatestMetric(id)
	services, _ := h.DB.ListServices(id)
	metrics, _ := h.DB.RecentMetrics(id, 24*time.Hour)
	alerts, _ := h.DB.ListAlerts(id, true)
	hostInfo, _ := h.DB.GetHostInfo(id)
	detail, _ := h.DB.GetLatestDetail(id)
	apiKey := h.DB.GetHostAPIKey(id)

	h.render(c, "host_detail.html", gin.H{
		"Title":    host.Name,
		"Host":     host,
		"Metric":   metric,
		"Services": services,
		"Metrics":  metrics,
		"Alerts":   alerts,
		"HostInfo": hostInfo,
		"Detail":   detail,
		"APIKey":   apiKey,
	})
}

func (h *Handler) EditHostForm(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	host, err := h.DB.GetHost(id)
	if err != nil {
		c.String(404, "host not found")
		return
	}
	h.render(c, "host_form.html", gin.H{"Title": "编辑主机", "Host": host})
}

func (h *Handler) UpdateHost(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	host, err := h.parseHostForm(c)
	if err != nil {
		c.String(400, "invalid form: %v", err)
		return
	}
	host.ID = id
	if err := h.DB.UpdateHost(host); err != nil {
		c.String(500, "db error: %v", err)
		return
	}
	c.Redirect(302, fmt.Sprintf("/hosts/%d", id))
}

func (h *Handler) DeleteHost(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	h.DB.DeleteHost(id)
	c.Redirect(302, "/hosts")
}

func (h *Handler) GenAPIKey(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	key := generateKey()
	h.DB.SetHostAPIKey(id, key)
	c.Redirect(302, fmt.Sprintf("/hosts/%d", id))
}

func (h *Handler) ExportHosts(c *gin.Context) {
	hosts, err := h.DB.ListHosts()
	if err != nil {
		c.String(500, "db error: %v", err)
		return
	}

	format := c.DefaultQuery("format", "csv")
	ts := time.Now().Format("20060102_150405")

	if format == "json" {
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="hostman_export_%s.json"`, ts))
		c.JSON(200, hosts)
		return
	}

	// CSV export
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="hostman_export_%s.csv"`, ts))
	c.Writer.Write([]byte("\xEF\xBB\xBF")) // UTF-8 BOM for Excel

	w := csv.NewWriter(c.Writer)
	w.Write([]string{"名称", "IP", "供应商", "套餐", "费用", "货币", "计费周期", "订阅日期", "到期日期", "状态", "备注"})

	for _, h := range hosts {
		subAt := ""
		if h.SubscribeAt != nil {
			subAt = h.SubscribeAt.Format("2006-01-02")
		}
		expAt := ""
		if h.ExpireAt != nil {
			expAt = h.ExpireAt.Format("2006-01-02")
		}
		w.Write([]string{
			h.Name, h.IP, h.Provider, h.Plan,
			fmt.Sprintf("%.2f", h.Cost), h.Currency, h.BillingCycle,
			subAt, expAt, h.Status, h.Note,
		})
	}
	w.Flush()
}

// ---------- API Endpoints ----------

func (h *Handler) AgentReport(c *gin.Context) {
	apiKey := c.GetHeader("X-API-Key")
	host, err := h.DB.GetHostByAPIKey(apiKey)
	if err != nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}

	var report model.AgentReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	report.Metric.HostID = host.ID
	report.Metric.CollectedAt = time.Now()
	if err := h.DB.InsertMetricWithDetail(&report.Metric, report.Detail); err != nil {
		c.JSON(500, gin.H{"error": "insert metric failed"})
		return
	}

	if len(report.Services) > 0 {
		for i := range report.Services {
			report.Services[i].HostID = host.ID
			report.Services[i].CheckedAt = time.Now()
		}
		h.DB.UpsertServices(host.ID, report.Services)
	}

	// Store host info if provided
	if report.HostInfo != nil {
		h.DB.UpdateHostInfo(host.ID, report.HostInfo)
	}

	h.DB.UpdateHostStatus(host.ID, "online")
	h.DB.UpdateLastSeen(host.ID)

	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) APIListHosts(c *gin.Context) {
	hosts, err := h.DB.ListHostsWithMetrics()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, hosts)
}

func (h *Handler) APIGetHost(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	host, err := h.DB.GetHost(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	metric, _ := h.DB.LatestMetric(id)
	c.JSON(200, gin.H{"host": host, "metric": metric})
}

// ---------- Helpers ----------

func (h *Handler) render(c *gin.Context, name string, data gin.H) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	tmpl, ok := h.tmpls[name]
	if !ok {
		c.String(500, "template not found: %s", name)
		return
	}
	if err := tmpl.ExecuteTemplate(c.Writer, "layout.html", data); err != nil {
		log.Printf("render %s error: %v", name, err)
	}
}

func (h *Handler) parseHostForm(c *gin.Context) (*model.Host, error) {
	cost, _ := strconv.ParseFloat(c.PostForm("cost"), 64)
	host := &model.Host{
		Name:         c.PostForm("name"),
		IP:           c.PostForm("ip"),
		Provider:     c.PostForm("provider"),
		Plan:         c.PostForm("plan"),
		Cost:         cost,
		Currency:     c.PostForm("currency"),
		BillingCycle: c.PostForm("billing_cycle"),
		Note:         c.PostForm("note"),
		Status:       "unknown",
	}
	if host.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if s := c.PostForm("subscribe_at"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err == nil {
			host.SubscribeAt = &t
		}
	}
	if s := c.PostForm("expire_at"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err == nil {
			host.ExpireAt = &t
		}
	}
	return host, nil
}

func countByStatus(hosts []model.HostWithMetric, status string) int {
	n := 0
	for _, h := range hosts {
		if h.Status == status {
			n++
		}
	}
	return n
}

func fmtBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func fmtUptime(seconds int64) string {
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%d天%d时%d分", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%d时%d分", h, m)
	}
	return fmt.Sprintf("%d分", m)
}

func generateKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}
