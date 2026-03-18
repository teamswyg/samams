# SAMAMS Launcher GUI

Backend(server), Frontend(Vite), Proxy를 한 창에서 실행·중지하고, 각 포트 상태를 모니터링하는 데스크톱 앱입니다.

## 실행 방법

**저장소 루트** 또는 **client/gui** 디렉터리에서 실행하세요. (저장소 루트는 `server/go.mod` 존재 여부로 자동 탐지됩니다.)

```bash
# 저장소 루트에서
go run ./client/gui

# 또는 client/gui에서
cd client/gui && go run .
```

## 기능

- **Backend (server)**  
  - 포트 기본값: `3000`  
  - `go run ./cmd/local` 실행  
  - 모니터: `http://127.0.0.1:{port}/healthz`

- **Frontend (Vite)**  
  - 포트 기본값: `5173`  
  - `npx vite --port {port}` 실행  
  - 모니터: `http://127.0.0.1:{port}/`

- **Proxy (agent-proxy)**  
  - 포트 기본값: `8080`  
  - `go run ./cmd/agent-proxy` 실행 (Server URL / Mode / Workdir 등 설정 사용)  
  - 모니터: `http://127.0.0.1:{port}/healthz`

각 컴포넌트는 **모니터링** 영역에서 **IP:Port**와 **🟢 실행 중 / 🔴 꺼짐**으로 표시되며, 약 2초마다 자동 갱신됩니다. 포트 입력값을 바꾸면 다음 폴링부터 해당 주소로 확인합니다.

## 요구 사항

- Go 1.26+
- Node.js / npm (Frontend 시작 시)
- 저장소 내 `server/`, `front/`, `client/proxy/` 구조 유지
