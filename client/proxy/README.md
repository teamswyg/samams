# SAMAMS Agent Proxy

에이전트(Cursor/Claude/OpenCode)를 관리하는 백그라운드 프록시. 서버와 WebSocket(로컬) 또는 HTTPS Polling(배포)으로 통신.

## 실행 방법

### 1. 서버 실행 (별도 터미널)

```bash
cd server
go run ./cmd/local
# :3000에서 HTTP + WebSocket 서빙
```

### 2. 토큰 획득

```bash
curl -X POST http://localhost:3000/user/auth/google/signup \
  -H "Content-Type: application/json" \
  -d '{"googleIdToken":"test","firebaseToken":"test"}'
# 응답의 access_token 값을 복사
```

### 3. 프록시 실행

```bash
cd client/proxy
SAMAMS_TOKEN=<토큰> go run ./cmd/agent-proxy
```

### 환경변수

| 변수 | 기본값 | 설명 |
|---|---|---|
| `SAMAMS_MODE` | `local` | `local` = WebSocket, `deploy` = HTTPS Polling |
| `SAMAMS_SERVER_URL` | `ws://localhost:3000` | 서버 주소 |
| `SAMAMS_TOKEN` | (필수) | Firebase/로컬 인증 토큰 |
| `SAMAMS_TOKEN_FILE` | `~/.samams/token` | 토큰 파일 경로 (환경변수 없을 때) |
| `CURSOR_AGENT_BIN` | (자동 탐색) | Cursor CLI agent 경로 |
| `AGENT_PROXY_HTTP_ADDR` | `127.0.0.1:8080` | 로컬 디버깅용 HTTP |
| `AGENT_PROXY_WORKDIR` | `~/.samams/workspaces` | 에이전트 작업 루트 디렉토리 |
| `AGENT_MAX_AGENTS` | `6` | 최대 동시 에이전트 수 |
| `HEARTBEAT_INTERVAL` | `10s` | heartbeat 주기 |
| `POLL_INTERVAL_ACTIVE` | `3s` | 에이전트 active 시 polling 간격 (배포) |
| `POLL_INTERVAL_IDLE` | `10s` | 에이전트 idle 시 polling 간격 (배포) |

---

## 데이터 저장 경로

### 로컬 개발 (`~/.samams/`)

```
~/.samams/
├── store/                                  ← 서버 localstore (JSON 파일, S3 키 레이아웃 미러)
│   ├── users/{id}.json                     ← 사용자 정보
│   ├── tasks/{id}.json                     ← 태스크 히스토리
│   ├── projects/{id}.json                  ← 프로젝트 정보
│   └── maal/{projectID}/{ts}.json          ← MAAL 로그 기록
│
├── git-hooks/                              ← git push 차단 hook
│   └── pre-push                            ← SAMAMS-AGENT-GUARD 스크립트
│
├── workspaces/                             ← agent worktrees (프로젝트별 격리)
│   └── {ProjectName}/                      ← 프로젝트 이름 (sanitized)
│       ├── dev-MLST-0001-A/                ← milestone worktree
│       ├── dev-TASK-0001-1/                ← task worktree (에이전트 작업 폴더)
│       └── fix-TASK-0001-2/                ← fix task worktree
│
└── token                                   ← 인증 토큰 파일 (선택)
```

### 배포 (AWS S3)

```
s3://{PERSISTENCE_BUCKET}/
├── proxy-state/{userID}/
│   ├── heartbeat.json                      ← 최신 heartbeat 스냅샷
│   ├── last-seen.json                      ← 마지막 통신 시각
│   └── summaries/{taskID}.json             ← 완료된 태스크 Summary
│
├── proxy-commands/{userID}/
│   ├── pending/{commandId}.json            ← 대기 중인 명령 (Frontend → Proxy)
│   └── completed/{commandId}.json          ← 완료된 명령 + 응답
│
├── users/{id}.json
├── tasks/{id}.json
├── projects/{id}.json
└── maal/{projectID}/{ts}.json
```

### 환경별 저장소 매핑

| 데이터 | 로컬 | 배포 |
|---|---|---|
| 사용자 | `~/.samams/store/users/` | S3 `users/` |
| 태스크 | `~/.samams/store/tasks/` | S3 `tasks/` |
| 프로젝트 | `~/.samams/store/projects/` | S3 `projects/` |
| MAAL 로그 | `~/.samams/store/maal/` | S3 `maal/` |
| 프록시 상태 | 인메모리 (WS 실시간) | S3 `proxy-state/` |
| 에이전트 worktree | `~/.samams/workspaces/` | `~/.samams/workspaces/` (동일) |
| 인증 토큰 | `~/.samams/token` | `~/.samams/token` (동일) |
| git hooks | `~/.samams/git-hooks/` | `~/.samams/git-hooks/` (동일) |

---

## YOLO 모드 & 에이전트 제한

에이전트는 **YOLO 모드** (`--yolo`)로 실행됩니다. 모든 터미널 명령과 파일 수정이 승인 없이 자동 실행됩니다.

### 허용 / 차단 목록

| 동작 | 허용 | 이유 |
|---|---|---|
| 파일 생성/수정 (worktree 내부) | O | YOLO 모드, worktree 스코프 |
| 파일 삭제 (worktree 내부) | O | YOLO 모드, worktree 스코프 |
| 파일 수정/삭제 (worktree 외부) | **X** | Cursor External-File Protection 차단 |
| 셸 명령 (go, npm, make 등) | O | YOLO 모드 |
| `git add` / `git commit` | O | 로컬 작업 |
| `git merge` / `git rebase` | O | 로컬 작업 |
| `git checkout` / `git branch` | O | 로컬 작업 |
| `git init` (새 repo 생성) | O | 로컬 작업 |
| `git stash` | O | 로컬 작업 |
| `git push` | **X** | pre-push hook 차단 |
| `git push --force` | **X** | 동일 hook |

### 파일 접근 격리 (Worktree)

각 에이전트는 **독립 worktree 폴더**에서 작업합니다.

```
에이전트 cmd.Dir = ~/.samams/workspaces/{ProjectName}/dev-TASK-0001-1/
```

- Cursor agent는 이 폴더를 **workspace**로 인식
- 외부 파일 접근 → Cursor External-File Protection이 차단
- 원본 repo와 다른 에이전트의 worktree는 접근 불가
- worktree 삭제 → merge 완료 후 프록시가 자동 정리

### Git Push 차단

프록시 시작/종료 시 자동으로 global git hook 관리:

```
프록시 시작 → ~/.samams/git-hooks/pre-push 생성
           → git config --global core.hooksPath ~/.samams/git-hooks

프록시 종료 → git config --global --unset core.hooksPath (원래대로 복원)
```

모든 작업은 로컬 `.git`에 남습니다. 원격에는 올라가지 않습니다.

### 사용자가 수동으로 push하기

프록시 실행 중에 직접 push하려면:

```bash
git -c core.hooksPath= push
```

프록시 종료 후에는 평소대로 `git push` 가능합니다.

---

## Git 브랜치 전략

태스크 트리의 각 노드가 **독립 git worktree**에서 작업합니다. proposal(head)은 브랜치 없이 main 사용, milestone부터 브랜치 생성.

### Worktree 구조

```
~/.samams/workspaces/{ProjectName}/
├── dev-MLST-0001-A/                    ← milestone worktree (from main)
│   에이전트가 이 폴더에서 작업
├── dev-TASK-0001-1/                    ← task worktree (from dev/MLST-0001-A)
├── fix-TASK-0001-2/                    ← fix task worktree
└── hotfix-TASK-0001-3/                 ← hotfix worktree
```

### 브랜치 이름 규칙

```
main
├── dev/MLST-0001-A              (milestone, from main)
│   ├── dev/TASK-0001-1          (task)
│   ├── fix/TASK-0001-2          (bug fix - summary에 bug/fix/patch 키워드)
│   └── hotfix/TASK-0001-3       (hotfix - summary에 hotfix/critical fix 키워드)
│
└── dev/MLST-0001-B              (milestone, from main)
    └── dev/TASK-0002-1          (task)
```

### Prefix 규칙

| 노드 타입 | Prefix | 기준 |
|---|---|---|
| proposal | (없음) | main 사용, 브랜치 생성하지 않음 |
| milestone | `dev/` | 항상 |
| task (기본) | `dev/` | 일반 개발 작업 |
| task (bug/fix) | `fix/` | summary에 bug, fix, patch, repair, resolve 포함 시 |
| task (hotfix) | `hotfix/` | summary에 hotfix, urgent fix, critical fix 포함 시 |

### Merge 방향 & 라이프사이클

```
1. task 생성  → git worktree add → 에이전트가 worktree에서 작업
2. task 완료  → worktree branch를 parent branch에 merge (--no-ff)
             → git worktree remove → 폴더 삭제 → branch 삭제
3. milestone  → 하위 task 모두 완료 → milestone branch를 main에 merge
             → worktree 제거
```

모든 merge는 **로컬에서만** 이루어집니다. push는 사용자가 수동으로 합니다.

---

## 크로스 플랫폼 지원

| 항목 | Windows | macOS | Linux |
|---|---|---|---|
| 홈 디렉토리 | `C:\Users\{name}` | `/Users/{name}` | `/home/{name}` |
| `.samams/` 경로 | `%USERPROFILE%\.samams\` | `~/.samams/` | `~/.samams/` |
| 경로 구분자 | `filepath.Join()` 사용 (자동) | 동일 | 동일 |
| Agent 바이너리 탐색 | `%LOCALAPPDATA%\cursor-agent\agent.cmd` | `~/.local/bin/agent` | `~/.local/bin/agent` |
| 메모리 모니터링 | 미지원 (graceful skip) | 미지원 (graceful skip) | `/proc/{pid}/status` 사용 |
| git hooks | 동일 | 동일 | 동일 |
| worktree | 동일 | 동일 | 동일 |

---

## 서버 연동 테스트

### 에이전트 목록 조회

```bash
curl http://localhost:3000/run/agents \
  -H "Authorization: Bearer <토큰>"
```

### 태스크 목록 조회

```bash
curl http://localhost:3000/run/tasks \
  -H "Authorization: Bearer <토큰>"
```

### 태스크 생성

```bash
curl -X POST http://localhost:3000/run/start \
  -H "Authorization: Bearer <토큰>" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "n1",
        "uid": "task-001",
        "type": "task",
        "summary": "test task: echo hello",
        "agent": "cursor",
        "status": "pending",
        "priority": "high",
        "parentId": null,
        "boundedContext": "default"
      }
    ],
    "max_agents": 1,
    "project_name": "My Project"
  }'
```

---

## 로컬 디버그 API (프록시 직접)

프록시의 `127.0.0.1:8080`에서 직접 접근 가능:

- **GET /healthz** — 프록시 상태
- **GET /tasks** / **GET /tasks/:id** — 태스크 목록/상세
- **GET /agents** / **GET /agents/:id** — 에이전트 목록/상세
- **POST /tasks** — 태스크 직접 생성
- **POST /tasks/:id/scale** — 에이전트 수 조절
- **POST /tasks/:id/stop** — 태스크 중단
- **POST /tasks/:id/pause** / **resume** / **cancel** / **reset**
- **POST /tasks/:id/retry-increment** — 재시도 카운트 +1
- **PUT /tasks/:id/summary** — 요약 갱신
- **GET /maal/tasks/:id** / **GET /maal/agents/:id** — MAAL 로그
- **GET /notifications** — 알림 목록
- **POST /strategy/pause** / **POST /strategy/apply** — 전략 회의
