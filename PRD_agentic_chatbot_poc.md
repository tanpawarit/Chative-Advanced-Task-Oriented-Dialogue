# PRD — Agentic Chatbot POC (ATOD-lite) (Multi-Agent)

Reference: [ArXiv Paper](https://arxiv.org/html/2601.11854v2?utm_source=chatgpt.com)

## Agent Roles

### A) Orchestrator Agent (Manager / Planner / Scheduler)
The **"central brain"** of the system with four core responsibilities:
1.  **Interpret & Plan**: Parse the latest user message and translate it into goal/slot/missing updates.
2.  **Goal Management**: Create new goals, update existing goals, select the active goal.
3.  **Interleaving**: Switch tasks via suspend/resume using the `GoalStack`.
4.  **Execution Control**: The **sole entity** that invokes tools and commits state.

The Orchestrator is an agent because it performs **workflow-level reasoning** — deciding task ordering ("which goal should run first?") and action selection ("what to ask? which tool to call?") based on the current state.

### B) Sales Agent (Domain Specialist: Sales)
A **domain expert for sales** that works exclusively on `sales.*` goals.
- If data is **incomplete** → Asks the most important clarifying question to gather requirements.
- If data is **complete** → Requests the necessary tools (e.g., `inventory.query`), then summarizes findings into a product recommendation and drives toward closing the sale.
- Returns `message` + `slots_patch` (e.g., `candidates`, `chosen_sku`).
- The Sales Agent has **no awareness** of the GoalStack or interleaving logic.

### C) Support Agent (Domain Specialist: Support)
A **domain expert for support** that works exclusively on `support.*` goals.
- If data is **incomplete** → Asks follow-up questions (model, symptoms, error code).
- If data is **complete** → Requests `knowledge_base.search`, then provides a step-by-step resolution.
- Returns `message` + `slots_patch` (e.g., `kb_refs`, `answer_sent`).

---

## Workflow Communication

### Step 1: Receive Message & Load State
- Orchestrator receives the user's text message.
- Calls `Load(SessionState)` to retrieve the current session.
- Immediately knows: what goal is pending (`ActiveGoalID`), the full task stack (`GoalStack`).

### Step 2: Planning
The Orchestrator asks itself three questions:
1.  Does this message relate to the **existing goal**, or is it a **new goal**?
2.  If it's a new goal, is its **priority higher** than the current goal? (Should we interleave?)
3.  What **data is missing** for the target goal?

This produces updates to:
- `Goals[g].Slots` — merge new data from the message.
- `Goals[g].Missing` — recompute what's still needed.
- `Goals[g].NextQuestion` — the single most important question to ask.
- `ActiveGoalID` / `GoalStack` — updated if interleaving occurs.

### Step 3: Dispatch to Sales or Support
- If `ActiveGoal.Type` is `sales.*` → Invoke **Sales Agent**.
- If `ActiveGoal.Type` is `support.*` → Invoke **Support Agent**.

### Step 4: Specialist Agent Executes (Ask or Act)
The specialist agent operates in exactly **two modes**:

| Mode | Condition | Behavior |
|:---|:---|:---|
| **Ask** (blocked) | `Missing` is not empty | Return `NextQuestion` immediately. No tool requests. |
| **Act** (active) | `Missing` is empty | Request required tools via `tool_requests`. Does **not** execute tools directly. |

### Step 5: Orchestrator Executes Tools & Finalizes
- Orchestrator invokes the **Tool Gateway** with the agent's `tool_requests`.
- Passes `tool_results` back to the specialist agent.
- Agent produces a **final answer** + `state_updates`.

### Step 6: Commit State & Resume (if applicable)
- Merge `slots_patch` into the goal.
- If goal is `done`:
  - Call `MarkGoalDone()` → Automatically calls `ResumePrevious()`.
- `Version++`, update `UpdatedAt`.
- Save `SessionState`.

### Step 7: Reply
- Send the final message back to the user.

---

## 1. Goal
Build a **POC Agentic Chatbot** that supports both **"Sales"** and **"Support"** tasks within a single system, featuring:
- **Multi-agent Architecture**: Orchestrator + Sales + Support.
- **ATOD-lite Capabilities**: Goal management, dependency handling (slot-filling), interleaving (suspend/resume), and long-term preference memory.
- **Orchestrator Responsibility**: The sole entity that invokes tools and commits state.
- **Zep Integration**: Long-term memory for **customer preferences**.
- **Eino Framework**: Use Eino framework to build the chatbot.

The system must be **generic**, capable of supporting various store types (computers, outdoor gear, etc.) by simply switching inventory data and knowledge base content.

---

## 2. Non-goals (Out of Scope for POC)
- Real Payment/Checkout integration.
- CRM/Ticketing/Order System integration.
- Observability, Metrics, CI/CD, or Deployment Pipeline.
- Policy Agent / Compliance Layer.
- External Batch Processing and Automation.
- Multi-language Optimization (Supports Thai but without complex localization).

---

## 3. Architecture Choice

### 3.1 Agents

#### 1) Orchestrator Agent
- Owner of `SessionState`.
- Handles perception, planning, and scheduling.
- Manages interleaving/resuming tasks using a **GoalStack**.
- Invokes Sales/Support agents.
- Executes tools via **Tool Gateway**.
- Reads/Writes **Zep preference memory**.

#### 2) Sales Agent
- Works on `sales.*` goals.
- **Modes**:
  - **Blocked**: Asks `NextQuestion` to gather missing information.
  - **Active**: Sends `tool_requests` or provides a finalized answer after receiving `tool_results`.

#### 3) Support Agent
- Works on `support.*` goals.
- Operates similarly to the Sales Agent but primarily utilizes the Knowledge Base (KB).

### 3.2 Tool Ownership
- **Orchestrator invokes ALL tools.**
- Sales/Support agents only "request" tools via `tool_requests`.

### 3.3 Agent Call Pattern (Two-pass loop)
- The Orchestrator calls the specialist agent 1–2 times per user message:
  1. **Pass 1**: Tool planning or asking a question.
  2. **Pass 2**: Finalizing the response after receiving `tool_results` (if tools were requested).

---

## 4. Core Data Model (Source of Truth)

### 4.1 SessionState (Workflow Memory)
- Used for interleaving and dependency management.
- Stores only workflow state; **does not** store permanent preferences.

```go
type SessionState struct {
  SessionID   string `json:"session_id"`
  WorkspaceID string `json:"workspace_id"`
  CustomerID  string `json:"customer_id"`
  ChannelType string `json:"channel_type"`
  Version     int64  `json:"version"`

  ActiveGoalID string           `json:"active_goal_id,omitempty"`
  GoalStack    []string         `json:"goal_stack,omitempty"`
  Goals        map[string]*Goal `json:"goals,omitempty"`

  UpdatedAt   time.Time `json:"updated_at"`
}

type Goal struct {
  ID           string         `json:"id"`
  Type         string         `json:"type"`   // e.g., sales.recommend_item | support.troubleshoot
  Status       GoalStatus     `json:"status"` // active | blocked | suspended | done
  Priority     int            `json:"priority"`
  Slots        map[string]any `json:"slots,omitempty"`
  Missing      []string       `json:"missing,omitempty"`
  NextQuestion string         `json:"next_question,omitempty"`
  UpdatedAt    time.Time      `json:"updated_at"`
}
```

### 4.2 Zep Memory (Customer Preference Memory)
Zep stores cross-session **"customer preferences"**, such as:
- `budget_range`: Price range.
- `preferred_brands` / `disliked_brands`.
- `preferred_features`: e.g., lightweight, quiet, long battery life.
- `communication_style`: e.g., prefers short summaries vs. detailed comparisons.

> **Note**: The Goal Stack and Active Goal are **not** stored in Zep.

---

## 5. Tools (POC)

### 5.1 Tool List

| Tool Name | Description |
| :--- | :--- |
| `math.evaluate` | Pure function, local safe execution. |
| `knowledge_base.search` | Searches the KB and returns snippets + references. |
| `inventory.query` | Queries Google Sheet stock items (Read-only). Returns item list, stock, price, attributes. |

### 5.2 Permission Matrix (Enforced in Tool Gateway)

| Agent | Allowed Tools |
| :--- | :--- |
| **Sales** | `inventory.query`, `math.evaluate` |
| **Support** | `knowledge_base.search`, `math.evaluate` |

---

## 6. Functional Requirements

### FR-1: Dependency / Slot Filling
- **When a goal cannot proceed due to missing data:**
  - Orchestrator sets goal status to `blocked`.
  - Populates `Missing` fields and `NextQuestion` (asks only the most important question).
  - Sales/Support agent must return `NextQuestion` immediately without calling tools.
- **When the user provides more information:**
  - Orchestrator merges data into `Slots`.
  - Recomputes `Missing`.
  - If `Missing` is empty → Set goal status to `active`.

### FR-2: Interleaving / Suspend-Resume
- **The system must support "interruptions":**
  - If a user sends a new intent with **higher priority** than the active goal:
    1. Call `SuspendAndActivate(newGoal)`.
    2. Set old goal status to `suspended`.
    3. Push old goal to stack.
    4. Set `ActiveGoalID` = `newGoal`.
  - When the interrupting goal is finished (`done`):
    1. Call `MarkGoalDone()`.
    2. Automatically resume the previous goal (`ResumePrevious()`).

### FR-3: Tool-mediated Answers
- **Sales Recommendation**: Must rely on `inventory.query` before proposing prices/stock.
- **Support Troubleshooting**: Must rely on `knowledge_base.search` before answering.
- **Constraint**: Agents must **not** hallucinate stock/price/KB info not present in `tool_results`.

### FR-4: Zep Preference Usage
- **Read**: Every turn, the Orchestrator reads the Zep preference summary.
- **Personalize**: Passes the summary to agents for response personalization.
- **Write**:
  - Orchestrator calls `WriteMemory(customer_id, summary_update)` every turn.
  - If no new preferences → Send empty update (Zep performs no-op).
  - If new preferences are detected → Update Zep.

### FR-5: Generic Store Support
- Flow and state must **not** hardcode product categories.
- Switching store types should be achievable by:
  1. Changing the Inventory Sheet Schema Mapping (Minimum requirement).
  2. Updating KB Content.

---

## 7. Non-functional Requirements
- **Deterministic State Transitions**: Traceable goal/stack changes.
- **Stateless Orchestrator**: Processes load/save state every request for horizontal scaling.
- **Optimistic Locking**: Uses `Version` field (short retry on conflict).
- **Tool Timeouts**: e.g., 3–10 seconds per tool.
- **Loop Limit**: Maximum 2 agent loops per turn (Planning + Finalize).

---

## 8. Internal Contracts (POC — No HTTP API)

> **Note**: For the POC phase, all communication is via **in-process Go function calls**. HTTP API endpoints will be added in a later phase.

### 8.1 Ingress
The entry point accepts a **plain text message** from the customer:
```go
func (o *Orchestrator) HandleMessage(sessionID string, text string) (string, error)
```

### 8.2 Orchestrator → Specialist Agent
```go
type AgentRunRequest struct {
  ActiveGoal    *Goal    `json:"active_goal"`
  MemorySummary string   `json:"memory_summary"`
  ToolResults   []ToolResult `json:"tool_results"`
}

type AgentRunResponse struct {
  Message      string            `json:"message"`
  ToolRequests []ToolRequest     `json:"tool_requests"`
  StateUpdates StateUpdates      `json:"state_updates"`
}
```

### 8.3 Orchestrator → Tool Gateway
```go
type ToolRequest struct {
  Tool string         `json:"tool"`
  Args map[string]any `json:"args"`
}

type ToolResult struct {
  Tool   string `json:"tool"`
  Result any    `json:"result"`
}
```

---

## 9. Orchestrator Turn Flow (Algorithm)

1. **Load SessionState** by `session_id` (or create new).
2. **Read Zep Preference Summary** by `customer_id`.
3. **Perception**: Extract intent/entities from user message.
4. **Planning**:
   - Decide whether to update existing goal or create a new goal.
   - Update `goal.Slots`.
   - Compute `goal.Missing` + `goal.NextQuestion`.
   - Set `goal.Status` (blocked/active).
5. **Scheduling**:
   - If new goal priority > current → `SuspendAndActivate(new_goal)`.
6. **Agent Run #1**:
   - Send `active_goal` + `memory_summary` + `tool_results=[]`.
   - If `tool_requests` is empty and `message` is non-empty → **Reply** (Blocked path).
   - If `tool_requests` exist:
     - Execute tools via **Tool Gateway** (enforce permissions).
7. **Agent Run #2** (if tools executed):
   - Send with `tool_results`.
8. **Commit**:
   - Merge `state_updates.slots_patch`.
   - Apply `state_updates.set_status`.
   - If done → `MarkGoalDone()` → `ResumePrevious()`.
   - Increment `Version`, update `UpdatedAt`.
   - **Save SessionState**.
   - **Write Zep Memory Update** (allow no-op).
9. **Reply** to user.

---

## 10. Acceptance Tests (POC Scenarios)

### T1: Sales Dependency
- **User**: "Recommend a gaming mouse."
- **System**: Blocks on budget (if missing) -> Asks 1 question.
- **User**: "1500"
- **System**: Calls `inventory.query` -> Recommends 1-3 options.

### T2: Support Dependency
- **User**: "My laptop keeps freezing."
- **System**: Blocks on model/symptom (if missing) -> Asks 1 question.
- **User**: "Lenovo Legion, freezes when gaming."
- **System**: Calls `kb.search` -> Provides step-by-step fix.

### T3: Interleaving
- **User**: "Recommend a laptop, budget 35k."
- **System**: Sales goal active.
- **User Interrupts**: "Is the one you just recommended in stock?" (Context specific) OR "By the way, do you have that mouse?"
- **System**: Still sales, tool inventory query, answer.
- **User Interrupts**: "My screen is frozen, what do I do?"
- **System**: Creates `support` goal (higher priority) -> Suspends `sales` -> Support done -> Resumes `sales`.

**Pass Criteria:**
- `GoalStack` correctness: Push/pop aligns with interleave/resume.
- `ActiveGoalID` correctness.
- Blocked goals always have `Missing` + `NextQuestion`.
- Tool answers distinct from `tool_results` (No hallucinated price/stock).

---

## 11. Implementation Notes (POC)
- **StateStore**: Use Redis, Firestore, or SQL (Pick one).
- **Session Key**: `{workspace}:{channel}:{customer}` or use `session_id` from ingress.
- **Zep**:
  - Read/Write uses `customer_id` as the primary key.
  - Write allows empty update -> no-op.
