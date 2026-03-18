# Project Domain Concepts

---

## Project

### 정의
현재 이 문서에서 project 는 SAMAMS 에서 관리되는 모든 것을 포함하는 최상위 컨테이너 개념을 의미한다.
하나의 project 안에 Proposal, Milestone, Task, Maps, History 등 모든 하위 구조가 속한다.

### 역할
- 시스템 내 작업과 계획의 소유 경계를 정한다.
- Tenant, Owner 등 접근 제어의 기준 단위가 된다.
- 하위의 모든 도메인 개념(Proposal, Maps, Task 등)을 하나의 범위로 묶는다.

### 규칙
- project 는 도메인 최상위 단위다.
- project 자체는 트리 노드가 아니다 — 트리 노드의 최상위 루트는 Proposal 이다.
- project 는 Proposal, Maps, History 등을 소유하는 컨테이너 역할만 한다.

### 한 줄 암기법
project 는 "Proposal 부터 Task 까지 모든 것을 담는 최상위 컨테이너"다.

---

## History (cross-domain 전달 구조)

### 정의
history 는 Proposal, Milestone, Summary, Frontier 로 이루어진 수직 계층형 전달 문서다.
작업의 시작점, 수행 결과, 다음 실행 사이의 경계를 정리하여 새로운 agent 에게 전달하기 위한 구조다.

> History는 독립 도메인이 아니라 project(Proposal, Milestone)와 task(Summary, Frontier) 도메인의 조합이다.

### 구성

```
Proposal
  └─ Milestone
       ├─ Summary (완료된 작업의 압축)  → task 도메인
       └─ Frontier (다음 작업의 명령)   → task 도메인
```

### 규칙
- history 는 수직적으로만 전달된다.
- 형제 agent 끼리는 각자의 summary 를 공유하지 않는다.
- 각 agent 는 오직 1:1 부모 관계 안에서 전달되는 history 만 받는다.

### 한 줄 암기법
history 는 "부모-자식 관계 안에서만 전달되는 수직적 작업 문맥"이다.

---

## Maps

### 정의
현재 이 문서에서 maps 는 Riido의 three-tiered project architecture 를 의미한다.

### 역할
- Project, Milestone/Object, Task의 구조를 정의한다.
- 계층 간 관계와 흐름 방향을 고정한다.
- 동일 계층 중첩 규칙을 명확히 한다.

### 규칙
- maps 는 Riido의 3단 구조를 전제로 한다.
- 트리 노드 구조는 Proposal → Milestone → Task 순으로 이어진다.
- 이 흐름은 왼쪽에서 오른쪽으로 진행되는 수직적 구조로 본다.
- 같은 계층끼리는 중첩되지 않는다.
- 단, Task 계층은 예외적으로 중첩되거나 겹쳐질 수 있다면 그 규칙을 별도로 명시해야 한다.

### Events
1. **태스크 지도가 초기화 됨** — 첫 계획에서 미리 만들어둔다(실행중이 아님). Bounded Context 개념을 프롬프트로 주입받아 각 태스크를 잘 분리한다.

### 한 줄 암기법
maps 는 "왼쪽에서 오른쪽으로 흐르는 Riido의 3계층 프로젝트 구조"다.

---

## Proposal

### 정의
현재 이 문서에서 proposal 은 새로운 agent 에게 전달되는 최초 제안 문서를 의미한다.
이는 프로젝트의 목표, 기능, 기술 구조, 추상 설계를 한 번에 정리하여, 별도의 긴 문맥 없이도 적은 code peek 만으로 효율적으로 작업할 수 있게 하기 위한 기준 문서다.

### 역할
- context 가 없는 새로운 agent 에게 프로젝트의 출발점을 제공한다.
- 현재 Task(작업)을 빠르게 이해하게 한다.
- 기술적 구조와 설계 방향을 한 번에 전달한다.
- 불필요한 코드 탐색을 줄이고 작업 효율을 높인다.

### 규칙
- proposal 은 최초 진입 문서다.
- proposal 은 설명이 아니라 작업 기준점이다.
- 새로운 agent 가 전체 코드를 깊게 보지 않아도 되도록 핵심만 담아야 한다.
- 기획, 기술, 추상 설계를 함께 담아야 한다.

### 구성
- Title
- Goal
- Description
- Features
- Technology Specification
  - Tech Stack, Architecture, Folder Structure, Framework, Coding Conventions, Bounded Contexts
- Abstract Specification
  - Domain Overview, Aggregates, Events, Workflows
- Created At
- Updated At

### 한 줄 암기법
proposal 은 "새로운 agent 가 최소한의 code peek 으로 바로 작업할 수 있게 만드는 최초 기준 문서"다.

---

## Milestone

### 정의
현재 이 문서에서 milestone 은 Proposal 의 직계 하향 노드를 의미한다.
이는 Proposal 을 실제로 구현하기 위해 나누어진 큰 작업 단위이자, 해당 범위를 설명하는 실행 기획 문서다.
Milestone 은 내부 기획을 더 작은 실행 단위로 분해하고, 이를 atomic 한 task 로 분류하여 하위에 분배한다.

### 역할
- Proposal 을 실행 가능한 큰 단위로 나눈다.
- 전체 목표를 중간 규모의 실행 계획으로 변환한다.
- 하위 task 들이 어떤 맥락과 목적 아래 움직이는지 정리한다.
- 큰 작업을 atomic 한 task 들로 분해하고 배분하는 기준점이 된다.

### 규칙
- milestone 은 Proposal 의 직계 하향 노드다.
- milestone 은 큰 작업 단위이자 실행 기획서다.
- milestone 자체가 최종 실행 단위는 아니다.
- milestone 은 atomic한 task 들의 집합이다.
- milestone 은 내부 내용을 atomic 하게 나누어 task 로 분류한다.
- task 는 milestone 의 하위에서 분배되고 실행된다.
- milestone 은 방향과 범위를 잡고, task 는 실제 수행을 담당한다.

### 한 줄 암기법
milestone 은 "Proposal 을 큰 실행 단위로 풀어내고, 이를 task 로 쪼개 분배하는 중간 기획 노드"다.
