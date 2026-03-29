# HostMan 🖥️

主机管理系统 — 订阅管理、运行监控、资源追踪

## 功能

- **主机管理**：添加、编辑、删除主机信息
- **订阅跟踪**：供应商、费用、到期提醒
- **资源监控**：CPU / 内存 / 磁盘 / 网络 / 负载（通过 Agent 上报）
- **服务状态**：Docker 容器 / systemd 服务监控
- **Web 仪表板**：暗色主题，响应式设计
- **REST API**：供 Agent 上报和外部集成

## 快速开始

```bash
# 编译
make build

# 运行
./bin/hostman-server -addr :8080 -templates web/templates

# 访问 http://localhost:8080
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | 监听地址 |
| `-db` | `hostman.db` | SQLite 数据库路径 |
| `-templates` | 自动检测 | 模板目录 |
| `-debug` | `false` | 调试模式 |

## 项目结构

```
hostman/
├── cmd/
│   ├── server/main.go    # Server 入口
│   └── agent/main.go     # Agent 入口 (Phase 2)
├── internal/
│   ├── model/model.go    # 数据模型
│   ├── store/sqlite.go   # SQLite 数据层
│   └── handler/handler.go # HTTP 处理器
├── web/templates/         # HTML 模板
├── go.mod
├── Makefile
└── README.md
```

## API

### Agent 上报

```bash
curl -X POST http://localhost:8080/api/v1/report \
  -H "X-API-Key: YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "metric": {
      "cpu_percent": 45.2,
      "mem_total": 8589934592,
      "mem_used": 4294967296,
      "disk_total": 107374182400,
      "disk_used": 53687091200,
      "load_1": 1.5,
      "load_5": 1.2,
      "load_15": 0.9,
      "uptime": 864000
    },
    "services": [
      {"name": "nginx", "type": "systemd", "status": "running"},
      {"name": "redis", "type": "docker", "status": "running"}
    ]
  }'
```

### 查询主机列表

```bash
curl http://localhost:8080/api/v1/hosts
```

## 开发路线

- [x] Phase 1: Server + SQLite + 主机 CRUD + 订阅管理 + Web 仪表板
- [x] Phase 2: Agent 资源采集 + 心跳上报
- [ ] Phase 3: 资源历史图表 + 趋势分析
- [ ] Phase 4: 告警系统 + Telegram 通知
