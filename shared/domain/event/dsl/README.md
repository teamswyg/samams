# Event DSL (Event Storming 기반)

이 디렉터리는 **도메인 이벤트** 의 SSOT 이다.

- **스키마·용어:** [docs/dsl.md](../../../../docs/dsl.md) 참고.
- **파일:** `events.json` — Domain event 목록 및 Policy(후속 이벤트) 정의.
- **생성:** `cd shared && go generate ./domain/event/...` → `../event.generated.go`

속성명은 Event Storming 용어를 사용한다: **event**, **policies**, **const**.
