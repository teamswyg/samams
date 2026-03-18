package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"proxy/internal/adapter/inbound/httpapi"
	"proxy/internal/adapter/outbound/cursor"
	"proxy/internal/adapter/outbound/gitbranch"
	"proxy/internal/adapter/outbound/wsconn"
	"proxy/internal/app"
	"proxy/internal/port"
)

func main() {
	cfg := loadConfig()

	log.Printf("starting agent proxy (mode: %s, cursor bin: %s)", cfg.Mode, cfg.CursorBin)

	r := cursor.NewRunner(cfg.CursorBin, cfg.WorkDir)

	if err := r.EnsureGitHooks(); err != nil {
		log.Printf("[hooks] Warning: failed to install git hooks: %v", err)
	} else {
		log.Println("[hooks] Git pre-push guard installed (push blocked, merge local only)")
	}

	branchMgr := gitbranch.New(cfg.WorkDir)
	log.Println("[branch] Git branch manager initialized")

	svcOpts := []app.Option{
		app.WithLogLines(cfg.AgentLogLines),
		app.WithMaxTasks(cfg.MaxTasks),
		app.WithMaxAgents(cfg.MaxAgents),
		app.WithBranchManager(branchMgr),
	}

	// ── Token acquisition ────────────────────────────────────────────
	frontendURL := getenv("SAMAMS_FRONTEND_URL", "http://localhost:5173")

	tokenData, err := wsconn.LoadTokenData()

	// Try to validate/refresh existing token.
	if tokenData != nil && err == nil {
		if tokenData.IsExpired() && tokenData.RefreshToken != "" {
			log.Println("[auth] Token expired, refreshing...")
			newData, refreshErr := wsconn.RefreshAccessToken(cfg.ServerURL, tokenData.RefreshToken)
			if refreshErr != nil {
				log.Printf("[auth] Refresh failed: %v", refreshErr)
				tokenData = nil // force browser login
			} else {
				tokenData = newData
				tokenData.ServerURL = cfg.ServerURL
				_ = wsconn.SaveTokenData(tokenData)
			}
		} else {
			// Token not expired, but verify it's still valid on the server.
			if !wsconn.ValidateToken(cfg.ServerURL, tokenData.AccessToken) {
				log.Println("[auth] Token rejected by server (server may have restarted)")
				// Try refresh first.
				if tokenData.RefreshToken != "" {
					newData, refreshErr := wsconn.RefreshAccessToken(cfg.ServerURL, tokenData.RefreshToken)
					if refreshErr == nil {
						tokenData = newData
						tokenData.ServerURL = cfg.ServerURL
						_ = wsconn.SaveTokenData(tokenData)
					} else {
						tokenData = nil // force browser login
					}
				} else {
					tokenData = nil
				}
			}
		}
	}

	// No valid token — launch browser login.
	if tokenData == nil {
		log.Println("[auth] No valid token, launching browser login...")
		tokenData, err = wsconn.BrowserLogin(frontendURL)
		if err != nil {
			log.Fatalf("[auth] Browser login failed: %v", err)
		}
		tokenData.ServerURL = cfg.ServerURL
		if err := wsconn.SaveTokenData(tokenData); err != nil {
			log.Printf("[auth] Warning: failed to save token: %v", err)
		}
		log.Println("[auth] Login successful, token saved to ~/.samams/token.json")
	}

	token := tokenData.AccessToken

	// ── Server connection (build-tag selected: local=WS, deploy=HTTPS) ──
	var conn port.ServerConnection
	if token != "" && cfg.ServerURL != "" {
		conn = createConnection(cfg, token, &svcOpts)
	}

	svc := app.NewTaskService(r, svcOpts...)

	if conn != nil {
		setupConnectionHandler(conn, svc, cfg, tokenData)

		connCtx, connCancel := context.WithCancel(context.Background())
		go func() {
			if err := conn.Run(connCtx); err != nil && connCtx.Err() == nil {
				log.Printf("[conn] Connection loop exited: %v", err)
			}
		}()
		log.Printf("[conn] Connecting to server at %s (mode: %s)", cfg.ServerURL, cfg.Mode)

		// Auto-refresh token before expiry.
		go wsconn.StartAutoRefresh(connCtx, cfg.ServerURL, tokenData, func(newToken string) {
			updateConnectionToken(conn, newToken)
		})

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			<-sigCh
			log.Println("shutting down...")
			connCancel()
			conn.Close()
			r.RemoveGitHooks()
			log.Println("[hooks] Global git hooks removed")
			os.Exit(0)
		}()
	} else {
		if token == "" {
			log.Println("[conn] SAMAMS_TOKEN not set — running without server connection")
		}
		if cfg.ServerURL == "" {
			log.Println("[conn] SAMAMS_SERVER_URL not set — running without server connection")
		}
	}

	// ── Local HTTP API (debug only, localhost) ─────────────────────
	api := httpapi.NewServer(svc)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("[http] Local debug API on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server error: %v", err)
	}
}

type config struct {
	Mode               string
	CursorBin          string
	WorkDir            string
	HTTPAddr           string
	MaxTasks           int
	MaxAgents          int
	AgentLogLines      int
	ServerURL          string
	HeartbeatInterval  time.Duration
	PollIntervalActive time.Duration
	PollIntervalIdle   time.Duration
}

func loadConfig() config {
	mode := getenv("SAMAMS_MODE", "local")
	cursorBin := getenv("CURSOR_AGENT_BIN", resolveAgentBin())
	httpAddr := getenv("AGENT_PROXY_HTTP_ADDR", "127.0.0.1:8080")
	workDir := getenv("AGENT_PROXY_WORKDIR", "")
	serverURL := getenv("SAMAMS_SERVER_URL", "ws://localhost:3000")

	maxTasks := getenvInt("AGENT_MAX_TASKS", 100)
	maxAgents := getenvInt("AGENT_MAX_AGENTS", 6)
	logLines := getenvInt("AGENT_AGENT_LOG_LINES", 200)

	heartbeatInterval := getenvDuration("HEARTBEAT_INTERVAL", 10*time.Second)
	pollActive := getenvDuration("POLL_INTERVAL_ACTIVE", 3*time.Second)
	pollIdle := getenvDuration("POLL_INTERVAL_IDLE", 10*time.Second)

	return config{
		Mode:               mode,
		CursorBin:          cursorBin,
		WorkDir:            workDir,
		HTTPAddr:           httpAddr,
		MaxTasks:           maxTasks,
		MaxAgents:          maxAgents,
		AgentLogLines:      logLines,
		ServerURL:          serverURL,
		HeartbeatInterval:  heartbeatInterval,
		PollIntervalActive: pollActive,
		PollIntervalIdle:   pollIdle,
	}
}

func resolveAgentBin() string {
	if p, err := exec.LookPath("agent"); err == nil {
		return p
	}

	home, _ := os.UserHomeDir()

	if runtime.GOOS == "windows" {
		candidates := []string{}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, "cursor-agent", "agent.cmd"))
		}
		if home != "" {
			candidates = append(candidates, filepath.Join(home, "AppData", "Local", "cursor-agent", "agent.cmd"))
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				log.Printf("[resolve] Found agent binary: %s", c)
				return c
			}
		}
	} else {
		if home != "" {
			c := filepath.Join(home, ".local", "bin", "agent")
			if _, err := os.Stat(c); err == nil {
				log.Printf("[resolve] Found agent binary: %s", c)
				return c
			}
		}
	}

	log.Println("[resolve] agent binary not found, using bare 'agent'")
	return "agent"
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
