package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// subprocess holds a long-running command started from the GUI.
type subprocess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func (p *subprocess) running() bool {
	return p != nil && p.cmd != nil && p.cmd.Process != nil
}

func (p *subprocess) stop() {
	if p == nil || p.cmd == nil {
		return
	}
	if p.cancel != nil {
		p.cancel()
	}
	_ = p.cmd.Process.Kill()
}

// repoRoot returns the repository root (directory containing server/, front/, client/).
func repoRoot() string {
	cwd, _ := os.Getwd()
	for _, dir := range []string{cwd, filepath.Join(cwd, ".."), filepath.Join(cwd, "..", "..")} {
		if _, err := os.Stat(filepath.Join(dir, "server", "go.mod")); err == nil {
			return dir
		}
	}
	return cwd
}

// checkEndpoint returns true if the URL responds (e.g. GET) within timeout.
func checkEndpoint(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// checkTCPPort checks if a TCP port is accepting connections (lsof-style).
func checkTCPPort(host, port string, timeout time.Duration) bool {
	port = strings.TrimSpace(port)
	if port == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func checkTCPPortAny(hosts []string, port string, timeout time.Duration) bool {
	for _, h := range hosts {
		if checkTCPPort(h, port, timeout) {
			return true
		}
	}
	return false
}

// samamsTheme applies design-system colors on top of the default theme.
type samamsTheme struct {
	fyne.Theme
	forceVariant *fyne.ThemeVariant
}

func (t *samamsTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if t.forceVariant != nil {
		variant = *t.forceVariant
	}
	switch name {
	case theme.ColorNameBackground:
		if variant == theme.VariantDark {
			return &color.RGBA{R: 0x1a, G: 0x1b, B: 0x1e, A: 0xff}
		}
		return &color.RGBA{R: 0xf5, G: 0xf6, B: 0xf8, A: 0xff}
	case theme.ColorNamePrimary:
		return &color.RGBA{R: 0x0d, G: 0x6e, B: 0xd4, A: 0xff}
	case theme.ColorNameButton:
		if variant == theme.VariantDark {
			return &color.RGBA{R: 0x2d, G: 0x2e, B: 0x32, A: 0xff}
		}
		return &color.RGBA{R: 0xe8, G: 0xea, B: 0xee, A: 0xff}
	case theme.ColorNameInputBackground:
		if variant == theme.VariantDark {
			return &color.RGBA{R: 0x2d, G: 0x2e, B: 0x32, A: 0xff}
		}
		return &color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	default:
		return t.Theme.Color(name, variant)
	}
}

func (t *samamsTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.Theme.Font(style)
}

func (t *samamsTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.Theme.Size(name)
}

func main() {
	a := app.New()
	// Fyne does not expose a public SetThemeVariant API; we implement a theme wrapper
	// that can force a light/dark variant for our design system colors.
	a.Settings().SetTheme(&samamsTheme{Theme: theme.DefaultTheme()})
	w := a.NewWindow("SAMAMS Launcher — Backend · Frontend · Proxy")
	w.Resize(fyne.NewSize(960, 680))

	root := repoRoot()

	// ── Port settings (monitoring + connections) ─────────────────────────
	serverPortEntry := widget.NewEntry()
	serverPortEntry.SetText("3000")
	frontPortEntry := widget.NewEntry()
	frontPortEntry.SetText("5173")
	proxyPortEntry := widget.NewEntry()
	proxyPortEntry.SetText("8080")

	serverURLEntry := widget.NewEntry()
	serverURLEntry.SetText("ws://localhost:3000")

	modeSelect := widget.NewSelect([]string{"local", "deploy"}, func(string) {})
	modeSelect.SetSelected("local")

	workdirEntry := widget.NewEntry()
	workdirEntry.SetPlaceHolder("Default (uses ~/.samams/workspaces)")

	maxAgentsEntry := widget.NewEntry()
	maxAgentsEntry.SetText("6")

	darkModeCheck := widget.NewCheck("Dark mode", func(on bool) {
		fyne.Do(func() {
			if on {
				v := theme.VariantDark
				a.Settings().SetTheme(&samamsTheme{Theme: theme.DefaultTheme(), forceVariant: &v})
			} else {
				v := theme.VariantLight
				a.Settings().SetTheme(&samamsTheme{Theme: theme.DefaultTheme(), forceVariant: &v})
			}
		})
	})
	// Default: follow Fyne/OS; but our launcher wants deterministic visuals.
	darkModeCheck.SetChecked(true)

	tokenInfoLabel := widget.NewLabel("Token: ~/.samams/token.json (browser login via agent-proxy)")

	statusLabel := widget.NewLabel("Status: stopped")
	healthLabel := widget.NewLabel("Health: N/A")

	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Proxy stdout/stderr logs appear here.")
	logOutput.Wrapping = fyne.TextWrapWord

	tasksList := widget.NewMultiLineEntry()
	agentsList := widget.NewMultiLineEntry()

	// Monitoring status labels (Backend / Frontend / Proxy) — updated via periodic polling.
	backendStatusLabel := widget.NewLabel("DOWN (127.0.0.1:3000)")
	backendStatusLabel.Importance = widget.WarningImportance
	frontendStatusLabel := widget.NewLabel("DOWN (127.0.0.1:5173)")
	frontendStatusLabel.Importance = widget.WarningImportance
	proxyStatusLabel := widget.NewLabel("DOWN (127.0.0.1:8080)")
	proxyStatusLabel.Importance = widget.WarningImportance

	var serverProc, frontProc, proxyProc *subprocess

	// Display-only fields without disabled styling (keeps colors readable).
	var (
		updatingLogs   bool
		updatingTasks  bool
		updatingAgents bool
		lastLogsText   string
		lastTasksText  string
		lastAgentsText string
	)

	setTextSafe := func(e *widget.Entry, text string, updating *bool, last *string) {
		*updating = true
		*last = text
		e.SetText(text)
		*updating = false
	}

	logOutput.OnChanged = func(s string) {
		if updatingLogs {
			return
		}
		// Revert user edits; keep the latest programmatic value.
		fyne.Do(func() { logOutput.SetText(lastLogsText) })
	}
	tasksList.OnChanged = func(s string) {
		if updatingTasks {
			return
		}
		fyne.Do(func() { tasksList.SetText(lastTasksText) })
	}
	agentsList.OnChanged = func(s string) {
		if updatingAgents {
			return
		}
		fyne.Do(func() { agentsList.SetText(lastAgentsText) })
	}

	appendLog := func(line string) {
		fyne.Do(func() {
			current := lastLogsText
			if current == "" {
				setTextSafe(logOutput, line, &updatingLogs, &lastLogsText)
			} else {
				setTextSafe(logOutput, current+"\n"+line, &updatingLogs, &lastLogsText)
			}
		})
	}

	// ── Backend (Server) ─────────────────────────────────────────────────
	startServerBtn := widget.NewButton("Start Backend", func() {
		if serverProc != nil && serverProc.running() {
			appendLog("Backend is already running.")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "go", "run", "./cmd/local")
		cmd.Dir = filepath.Join(root, "server")
		cmd.Env = append(os.Environ(), "API_ADDR=:"+serverPortEntry.Text)
		if err := cmd.Start(); err != nil {
			appendLog(fmt.Sprintf("Failed to start Backend: %v", err))
			cancel()
			return
		}
		serverProc = &subprocess{cmd: cmd, cancel: cancel}
		appendLog("Started Backend (server) process. Port " + serverPortEntry.Text)
		go func() {
			_ = cmd.Wait()
			fyne.Do(func() { serverProc = nil })
		}()
	})
	stopServerBtn := widget.NewButton("Stop Backend", func() {
		if serverProc != nil && serverProc.running() {
			serverProc.stop()
			appendLog("Sent request to stop Backend.")
		}
	})

	// ── Frontend ─────────────────────────────────────────────────────────
	startFrontBtn := widget.NewButton("Start Frontend", func() {
		if frontProc != nil && frontProc.running() {
			appendLog("Frontend is already running.")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		// Bind to 127.0.0.1 so TCP probe on IPv4 always works.
		cmd := exec.CommandContext(ctx, "npx", "vite", "--host", "127.0.0.1", "--port", strings.TrimSpace(frontPortEntry.Text))
		cmd.Dir = filepath.Join(root, "front")
		if err := cmd.Start(); err != nil {
			appendLog(fmt.Sprintf("Failed to start Frontend: %v (check npm install)", err))
			cancel()
			return
		}
		frontProc = &subprocess{cmd: cmd, cancel: cancel}
		appendLog("Started Frontend process. Port " + frontPortEntry.Text)
		go func() {
			_ = cmd.Wait()
			fyne.Do(func() { frontProc = nil })
		}()
	})
	stopFrontBtn := widget.NewButton("Stop Frontend", func() {
		if frontProc != nil && frontProc.running() {
			frontProc.stop()
			appendLog("Sent request to stop Frontend.")
		}
	})

	// ── Proxy ────────────────────────────────────────────────────────────
	startBtn := widget.NewButton("Start Proxy", func() {
		if proxyProc != nil && proxyProc.running() {
			appendLog("Proxy is already running.")
			return
		}
		port := proxyPortEntry.Text
		if port == "" {
			port = "8080"
		}
		serverURL := serverURLEntry.Text
		if serverURL == "" {
			serverURL = "ws://localhost:" + serverPortEntry.Text
		}
		frontURL := "http://localhost:" + frontPortEntry.Text
		mode := modeSelect.Selected
		if mode == "" {
			mode = "local"
		}
		workdir := workdirEntry.Text
		maxAgents := maxAgentsEntry.Text
		if maxAgents == "" {
			maxAgents = "6"
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "go", "run", "./cmd/agent-proxy")
		cmd.Dir = filepath.Join(root, "client", "proxy")
		env := os.Environ()
		env = append(env,
			"SAMAMS_MODE="+mode,
			"SAMAMS_SERVER_URL="+serverURL,
			"SAMAMS_FRONTEND_URL="+frontURL,
			"AGENT_MAX_AGENTS="+maxAgents,
			"AGENT_PROXY_HTTP_ADDR=127.0.0.1:"+port,
		)
		if workdir != "" {
			env = append(env, "AGENT_PROXY_WORKDIR="+workdir)
		}
		cmd.Env = env

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		if err := cmd.Start(); err != nil {
			appendLog(fmt.Sprintf("Failed to start Proxy: %v", err))
			cancel()
			return
		}

		proxyProc = &subprocess{cmd: cmd, cancel: cancel}
		fyne.Do(func() {
			statusLabel.SetText("Proxy: running")
		})
		appendLog("Started Proxy process. Port " + port)

		go func() {
			io.Copy(&uiLogWriter{append: appendLog}, stdout)
		}()
		go func() {
			io.Copy(&uiLogWriter{append: appendLog}, stderr)
		}()
		go func() {
			err := cmd.Wait()
			fyne.Do(func() {
				if err != nil {
					appendLog(fmt.Sprintf("Proxy exited: %v", err))
				} else {
					appendLog("Proxy exited cleanly.")
				}
				statusLabel.SetText("Proxy: stopped")
			})
			proxyProc = nil
		}()
	})

	stopBtn := widget.NewButton("Stop Proxy", func() {
		if proxyProc == nil || !proxyProc.running() {
			appendLog("No Proxy is currently running.")
			return
		}
		proxyProc.stop()
		appendLog("Sent request to stop Proxy.")
		statusLabel.SetText("Proxy: stopping...")
	})

	refreshBtn := widget.NewButton("Refresh health/status", func() {
		healthStr, tasksBody, agentsBody := fetchProxyStatus(proxyPortEntry.Text, appendLog)
		fyne.Do(func() {
			if healthStr != "" {
				healthLabel.SetText(healthStr)
			}
			if tasksBody != "" {
				setTextSafe(tasksList, tasksBody, &updatingTasks, &lastTasksText)
			}
			if agentsBody != "" {
				setTextSafe(agentsList, agentsBody, &updatingAgents, &lastAgentsText)
			}
		})
	})

	shortcutHint := widget.NewLabel("Shortcuts: ⌘↵ Start Proxy  ·  ⌘. Stop Proxy  ·  ⌘R Refresh  ·  F5 Refresh  ·  ⌘Q Quit")

	// Monitor panel: per-component address + status (updated via periodic polling).
	monitorCard := container.NewVBox(
		widget.NewLabelWithStyle("Monitoring (IP:Port)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(3,
			container.NewVBox(widget.NewLabel("Backend"), backendStatusLabel),
			container.NewVBox(widget.NewLabel("Frontend"), frontendStatusLabel),
			container.NewVBox(widget.NewLabel("Proxy"), proxyStatusLabel),
		),
	)

	header := container.NewVBox(
		widget.NewLabelWithStyle("SAMAMS Launcher — Backend · Frontend · Proxy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(3,
			container.NewVBox(
				widget.NewLabel("Backend port"),
				serverPortEntry,
				container.NewHBox(startServerBtn, stopServerBtn),
			),
			container.NewVBox(
				widget.NewLabel("Frontend port"),
				frontPortEntry,
				container.NewHBox(startFrontBtn, stopFrontBtn),
			),
			container.NewVBox(
				widget.NewLabel("Proxy port"),
				proxyPortEntry,
				container.NewHBox(startBtn, stopBtn),
			),
		),
		container.NewHBox(
			darkModeCheck,
			layout.NewSpacer(),
		),
		monitorCard,
		container.NewGridWithColumns(2,
			container.NewVBox(
				widget.NewLabel("Server URL (for Proxy)"),
				serverURLEntry,
			),
			container.NewVBox(
				widget.NewLabel("Mode / Max Agents"),
				container.NewHBox(modeSelect, maxAgentsEntry),
			),
		),
		container.NewHBox(
			widget.NewLabel("Workdir"),
			workdirEntry,
			layout.NewSpacer(),
			refreshBtn,
		),
		tokenInfoLabel,
		container.NewHBox(
			statusLabel,
			layout.NewSpacer(),
			healthLabel,
		),
		shortcutHint,
	)

	logsPanel := container.NewVBox(
		widget.NewLabelWithStyle("Proxy Logs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewMax(logOutput),
	)

	statePanel := container.NewVBox(
		widget.NewLabelWithStyle("Tasks (/tasks)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewMax(tasksList),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Agents (/agents)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewMax(agentsList),
	)

	content := container.NewBorder(
		header,
		nil,
		nil,
		nil,
		container.NewHSplit(logsPanel, statePanel),
	)

	w.SetContent(content)

	// UX shortcuts (⌘ on macOS, Ctrl on Windows/Linux)
	mod := fyne.KeyModifierShortcutDefault
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: mod}, func(fyne.Shortcut) {
		if startBtn.Disabled() == false {
			startBtn.OnTapped()
		}
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPeriod, Modifier: mod}, func(fyne.Shortcut) {
		stopBtn.OnTapped()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: mod}, func(fyne.Shortcut) {
		refreshBtn.OnTapped()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF5, Modifier: 0}, func(fyne.Shortcut) {
		refreshBtn.OnTapped()
	})

	// Periodic monitoring: check Backend / Frontend / Proxy ports and update status labels.
	go func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for range tick.C {
			// Read ports from entries on every tick (reflect changes while running).
			sp := strings.TrimSpace(serverPortEntry.Text)
			fp := strings.TrimSpace(frontPortEntry.Text)
			pp := strings.TrimSpace(proxyPortEntry.Text)
			if sp == "" {
				sp = "3000"
			}
			if fp == "" {
				fp = "5173"
			}
			if pp == "" {
				pp = "8080"
			}
			backendOK := checkTCPPortAny([]string{"127.0.0.1", "localhost", "::1"}, sp, 500*time.Millisecond)
			frontOK := checkTCPPortAny([]string{"127.0.0.1", "localhost", "::1"}, fp, 500*time.Millisecond)
			proxyOK := checkTCPPortAny([]string{"127.0.0.1", "localhost", "::1"}, pp, 500*time.Millisecond)

			fyne.Do(func() {
				setMonitorStatus(backendStatusLabel, backendOK, "127.0.0.1:"+sp)
				setMonitorStatus(frontendStatusLabel, frontOK, "127.0.0.1:"+fp)
				setMonitorStatus(proxyStatusLabel, proxyOK, "127.0.0.1:"+pp)
			})
		}
	}()

	// Periodic refresh of Proxy health/status (every 5 seconds).
	go func() {
		for range time.Tick(5 * time.Second) {
			healthStr, tasksBody, agentsBody := fetchProxyStatus(proxyPortEntry.Text, func(s string) { _ = s })
			fyne.Do(func() {
				if healthStr != "" {
					healthLabel.SetText(healthStr)
				}
				if tasksBody != "" {
					setTextSafe(tasksList, tasksBody, &updatingTasks, &lastTasksText)
				}
				if agentsBody != "" {
					setTextSafe(agentsList, agentsBody, &updatingAgents, &lastAgentsText)
				}
			})
		}
	}()

	w.ShowAndRun()
}

func setMonitorStatus(lbl *widget.Label, up bool, addr string) {
	if up {
		lbl.SetText("RUNNING (" + addr + ")")
		lbl.Importance = widget.SuccessImportance
	} else {
		lbl.SetText("DOWN (" + addr + ")")
		lbl.Importance = widget.WarningImportance
	}
}

type uiLogWriter struct {
	append func(string)
}

func (w *uiLogWriter) Write(p []byte) (int, error) {
	lines := bytes.Split(p, []byte("\n"))
	for _, l := range lines {
		if len(bytes.TrimSpace(l)) == 0 {
			continue
		}
		txt := string(l)
		t := time.Now().Format("15:04:05")
		w.append(fmt.Sprintf("[%s] %s", t, txt))
	}
	return len(p), nil
}

func fetchProxyStatus(proxyPort string, appendLog func(string)) (healthStr, tasksBody, agentsBody string) {
	if proxyPort == "" {
		proxyPort = "8080"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	proxyBase := "http://127.0.0.1:" + proxyPort

	resp, err := client.Get(proxyBase + "/healthz")
	if err != nil {
		return "Health: unreachable", "", ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	healthStr = fmt.Sprintf("Health: %s (%d)", string(body), resp.StatusCode)

	if tr, err := client.Get(proxyBase + "/tasks"); err == nil {
		defer tr.Body.Close()
		b, _ := io.ReadAll(tr.Body)
		tasksBody = string(b)
	} else {
		appendLog(fmt.Sprintf("Failed to fetch /tasks: %v", err))
	}
	if ar, err := client.Get(proxyBase + "/agents"); err == nil {
		defer ar.Body.Close()
		b, _ := io.ReadAll(ar.Body)
		agentsBody = string(b)
	} else {
		appendLog(fmt.Sprintf("Failed to fetch /agents: %v", err))
	}
	return healthStr, tasksBody, agentsBody
}

