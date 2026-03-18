// Local development server. NEVER deploy as Lambda.
// 로컬 전용 엔트리포인트. inmemory, stub, ConsolePublisher 등 로컬 전용 의존성만 여기서 import.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	// Application services
	appAgent "server/internal/app/agent"
	appControl "server/internal/app/control"
	appStrategy "server/internal/app/strategy"
	appTask "server/internal/app/task"
	appUser "server/internal/app/user"

	// Domain
	"server/internal/domain/llm"
	prompt_pkg "server/infra/out/llm/prompt"
	"server/internal/domain/shared"

	// LOCAL ONLY — persistence & console event bus
	"server/infra/out/event"
	"server/infra/persistence/inmemory"
	"server/infra/persistence/localstore"

	// LLM providers (real + LOCAL ONLY stub fallback)
	"server/infra/out/llm/anthropic"
	"server/infra/out/llm/gemini"
	"server/infra/out/llm/openai"
	"server/infra/out/llm/stub"

	// Config & HTTP handlers
	"server/infra/config"
	"server/infra/in/httpserver"
)

func main() {
	// Structured logging setup.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Load .env
	envLoaded := false
	for _, p := range []string{".env", "server/.env", "../server/.env"} {
		if err := config.LoadEnv(p); err == nil {
			slog.Info("loaded env file", "path", p)
			envLoaded = true
			break
		}
	}
	if !envLoaded {
		slog.Info("no .env file found, running with environment variables only")
	}

	// Typed configuration.
	cfg := config.LoadServerConfig()
	for _, w := range cfg.Validate() {
		slog.Warn("config", "warning", w)
	}
	slog.Info("config loaded", "summary", "\n"+cfg.Summary())

	addr := cfg.Addr
	clock := shared.SystemClock{}

	// ── LLM Providers (stub fallback for local dev) ────────────────
	var (
		analyzer   appAgent.LogAnalyzer
		planner    appAgent.Planner
		summarizer appAgent.Summarizer
	)

	stubProvider := stub.New()

	if cfg.HasOpenAI {
		slog.Info("LLM provider", "role", "LogAnalyzer", "provider", "OpenAI")
		analyzer = openai.NewClient(cfg.OpenAIKey, cfg.OpenAIModel)
	} else {
		analyzer = stubProvider
	}

	if cfg.HasAnthropic {
		slog.Info("LLM provider", "role", "Planner", "provider", "Anthropic")
		planner = anthropic.NewClient(cfg.AnthropicKey, cfg.AnthropicModel)
	} else {
		planner = stubProvider
	}

	if cfg.HasGemini {
		slog.Info("LLM provider", "role", "Summarizer", "provider", "Gemini")
		summarizer = gemini.NewClient(cfg.GeminiKey, cfg.GeminiModel)
	} else {
		summarizer = stubProvider
	}

	// ── Repositories (localstore: ~/.samams/store/) ─────────────────
	fileStore, err := localstore.New("")
	if err != nil {
		log.Fatalf("[local] localstore init failed: %v", err)
	}
	log.Println("[local] Using localstore (~/.samams/store/) — data persists across restarts")

	userRepo := localstore.NewUserRepository(fileStore)
	taskRepo := localstore.NewTaskRepository(fileStore)

	controlRepo := inmemory.NewControlStateRepository()
	meetingRepo := inmemory.NewStrategyMeetingRepository()
	failureRepo := inmemory.NewFailureTrackerRepository()

	// ── Event Bus (LOCAL ONLY: console logger) ─────────────────────
	eventBus := event.NewConsolePublisher()

	// ── Application Services ───────────────────────────────────────
	userSvc := appUser.NewService(userRepo, clock)
	agentSvc := appAgent.NewService(analyzer, planner, summarizer)
	controlSvc := appControl.NewService(controlRepo, eventBus, clock)
	strategySvc := appStrategy.NewService(meetingRepo, failureRepo, eventBus, clock)
	taskSvc := appTask.NewService(taskRepo, nil, summarizer, clock)

	// ── HTTP Handlers ──────────────────────────────────────────────
	mux := http.NewServeMux()

	authHandler := httpserver.NewAuthHandler(userSvc)
	authHandler.Register(mux)

	aiHandler := httpserver.NewAIHandler(agentSvc)
	aiHandler.Register(mux)

	sentinelHandler := httpserver.NewSentinelHandler(controlSvc, strategySvc)
	sentinelHandler.Register(mux)

	sessionHandler := httpserver.NewSessionHandler()
	sessionHandler.Register(mux)

	planHandler := httpserver.NewPlanHandler(fileStore)
	planHandler.Register(mux)
	log.Println("[local] Plan API registered at /plans/*")

	// ── WebSocket Proxy Hub + Channel-Based Event Processor ────────
	proxyHub := httpserver.NewProxyHub(authHandler)
	fg := &llmFrontierGenerator{planner: planner, gemini: summarizer}
	runHandler := httpserver.NewRunHandler(proxyHub, fileStore, fg)
	runHandler.Register(mux)

	eventProc := newEventProcessor(controlSvc, strategySvc, taskSvc, summarizer, planner, runHandler, fileStore)
	proxyHub.SetEventRouter(eventProc)
	mux.HandleFunc("/ws/proxy", proxyHub.HandleWebSocket)
	log.Println("[local] WebSocket proxy hub registered at /ws/proxy")

	// Health check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteOK(w, map[string]string{"status": "ok"})
	})

	// ── Server ─────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         addr,
		Handler:      httpserver.CORS(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		<-sigCh
		log.Println("[local] Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		eventProc.Close()
		proxyHub.Close()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("[local] SAMAMS local dev server listening on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[local] Fatal: %v", err)
	}
}

// llmFrontierGenerator implements httpserver.FrontierGenerator.
// milestoneProposal → Claude (Anthropic), frontiers → Gemini Flash (batch, cheap).
type llmFrontierGenerator struct {
	planner appAgent.Planner    // Claude — milestoneProposal
	gemini  appAgent.Summarizer // Gemini Flash — batch frontier generation
}

func (g *llmFrontierGenerator) GenerateMilestoneProposal(ctx context.Context, proposal, milestoneSummary string, taskSummaries []string) (string, error) {
	tasksText := strings.Join(taskSummaries, "\n- ")
	prompt := fmt.Sprintf(`Based on the following project proposal and milestone summary, generate a detailed milestone specification (milestoneProposal).

## Project Proposal
%s

## Milestone Summary
%s

## Child Tasks
- %s

Generate a comprehensive milestone specification that includes:
1. Milestone objective and scope
2. Technical approach and architecture decisions
3. Key deliverables
4. Dependencies and constraints
5. Success criteria

Output plain text with markdown headers. Be specific and actionable.`, proposal, milestoneSummary, tasksText)

	resp, err := g.planner.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: "You are a technical architect generating detailed milestone specifications for a multi-agent software development system.",
		UserPrompt:   prompt,
		MaxTokens:    4096,
		Temperature:  0.3,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// GenerateTaskFrontiers generates frontiers for all tasks under a milestone in ONE Gemini call.
// Returns map[taskUID] → frontier string.
func (g *llmFrontierGenerator) GenerateTaskFrontiers(ctx context.Context, proposal, milestoneProposal string, tasks []httpserver.TaskFrontierInput) (map[string]string, error) {
	// Build task list with explicit ownership boundaries.
	var taskList strings.Builder
	for i, t := range tasks {
		taskList.WriteString(fmt.Sprintf("\n### Task %d\n- UID: %s\n- Summary: %s\n", i+1, t.UID, t.Summary))
	}

	// Build sibling awareness section — each task must know what NOT to touch.
	var siblingMap strings.Builder
	for _, t := range tasks {
		var others []string
		for _, s := range tasks {
			if s.UID != t.UID {
				others = append(others, s.UID+": "+s.Summary)
			}
		}
		siblingMap.WriteString(fmt.Sprintf("\n%s's FORBIDDEN zones (owned by siblings):\n", t.UID))
		for _, o := range others {
			siblingMap.WriteString(fmt.Sprintf("  - %s\n", o))
		}
	}

	// Build UID list for JSON output format.
	var uidList strings.Builder
	for i, t := range tasks {
		if i > 0 {
			uidList.WriteString(", ")
		}
		uidList.WriteString(fmt.Sprintf(`"%s": "<full frontier>"`, t.UID))
	}

	prompt := fmt.Sprintf(`## Project Proposal (Full Context)
%s

## Milestone Specification (This milestone's scope)
%s

## Tasks Under This Milestone
%s

## Sibling Isolation Map (DDD Bounded Context Boundaries)
%s

## CRITICAL RULES

### 1. DDD Bounded Context Isolation
Each task operates in its OWN bounded context. Tasks MUST NOT share files, types, or packages.
If two tasks need to interact, they do so through INTERFACES defined by the task that OWNS the contract.

### 2. File Ownership — ZERO OVERLAP
Every file is OWNED by exactly ONE task. No two tasks may create or modify the same file.

### 3. Frontier Detail Requirements
Each frontier MUST include: BOUNDED CONTEXT, OBJECTIVE, PRECONDITIONS, FILE OWNERSHIP, IMPLEMENTATION SPEC (exact struct definitions, function signatures, file paths), ISOLATION BOUNDARY, DELIVERABLES, DONE CRITERIA.

### 4. Minimum Detail
Each frontier must be at least 1500 characters. Include actual code structures, file paths, function signatures.

## OUTPUT FORMAT — CRITICAL
Use the EXACT delimiter format below. Do NOT output JSON.
Each task frontier starts with ===TASK_UID=== on its own line, followed by the frontier text.

%s

Write the frontier content directly after each delimiter. No quotes, no escaping, just plain text.`,
		proposal, milestoneProposal, taskList.String(), siblingMap.String(),
		func() string {
			var sb strings.Builder
			for _, t := range tasks {
				sb.WriteString(fmt.Sprintf("===%s===\n<frontier for %s here>\n\n", t.UID, t.UID))
			}
			return sb.String()
		}())

	// Use Gemini Flash — cheap, fast, sufficient for structured generation.
	// MaxTokens scaled to task count: ~2000 tokens per task frontier.
	maxTokens := 4096 * len(tasks)
	if maxTokens < 8192 {
		maxTokens = 8192
	}
	if maxTokens > 32768 {
		maxTokens = 32768
	}

	resp, err := g.gemini.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt_pkg.FrontierCommand,
		UserPrompt:   prompt,
		MaxTokens:    maxTokens,
		Temperature:  0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini batch frontier: %w", err)
	}

	// Parse delimiter-based response: ===TASK-UID===\n<frontier text>
	content := strings.TrimSpace(resp.Content)
	result := make(map[string]string)

	// Split by ===...=== delimiters.
	for _, t := range tasks {
		delimiter := "===" + t.UID + "==="
		idx := strings.Index(content, delimiter)
		if idx == -1 {
			log.Printf("[frontier] Task %s delimiter not found in response", t.UID)
			continue
		}

		// Extract text after this delimiter until next delimiter or end.
		start := idx + len(delimiter)
		remaining := content[start:]
		// Trim leading newline.
		remaining = strings.TrimPrefix(remaining, "\n")

		// Find the next delimiter (===TASK-...) or end of content.
		end := len(remaining)
		nextIdx := strings.Index(remaining, "\n===")
		if nextIdx != -1 {
			end = nextIdx
		}

		frontier := strings.TrimSpace(remaining[:end])
		if frontier != "" {
			result[t.UID] = frontier
		}
	}

	log.Printf("[frontier] Gemini returned %d/%d frontiers (tokens: %d)", len(result), len(tasks), resp.TokensUsed)
	return result, nil
}
