sequenceDiagram
    autonumber
    participant U as User
    participant O as Orchestrator Agent
    participant S as StateStore
    participant Z as Zep Memory
    participant SA as Sales Agent
    participant SU as Support Agent
    participant T as Tool Gateway

    U->>O: message(text)

    O->>S: Load(SessionState by session_id)
    S-->>O: SessionState(ActiveGoalID, GoalStack, Goals, Version)

    Note over O: Perception + Planning (update/create goals)\n- fill Slots\n- compute Missing + NextQuestion\n- set GoalStatus\n- decide interleave using Priority

    alt Interleave (new goal priority > current)
        Note over O: SuspendAndActivate(new_goal)\n- current goal -> suspended\n- GoalStack push\n- ActiveGoalID = new_goal
    else No interleave
        Note over O: Keep/Set ActiveGoalID
    end

    %% Zep is always used (not optional)
    O->>Z: ReadMemory(customer_id)
    Z-->>O: memory_summary

    alt Active goal is sales.*
        O->>SA: AgentRun(active_goal, memory_summary, tool_results=[])
        alt Goal blocked (Missing not empty)
            SA-->>O: {message=NextQuestion, tool_requests=[]}
        else Goal active
            SA-->>O: {tool_requests=[...], message=""}
            O->>T: ExecuteTools(agent_type="sales", tool_requests)
            T-->>O: tool_results
            O->>SA: AgentRun(active_goal, memory_summary, tool_results)
            SA-->>O: {message=final_answer, state_updates}
        end
    else Active goal is support.*
        O->>SU: AgentRun(active_goal, memory_summary, tool_results=[])
        alt Goal blocked (Missing not empty)
            SU-->>O: {message=NextQuestion, tool_requests=[]}
        else Goal active
            SU-->>O: {tool_requests=[...], message=""}
            O->>T: ExecuteTools(agent_type="support", tool_requests)
            T-->>O: tool_results
            O->>SU: AgentRun(active_goal, memory_summary, tool_results)
            SU-->>O: {message=final_answer, state_updates}
        end
    end

    Note over O: Commit state (Layer 4)\n- merge slots_patch\n- apply set_status\n- if active goal done: MarkGoalDone() -> ResumePrevious()\n- Version++ UpdatedAt

    O->>S: Save(SessionState)
    S-->>O: OK

    %% Zep write-back is always used (not optional)
    O->>Z: WriteMemory(customer_id, summary_update)
    Z-->>O: OK

    O-->>U: reply(message)


```mermaid
flowchart TD

    %% ── Entry ──────────────────────────────────────────────
    subgraph ENTRY ["Entry"]
        U(["User"])
        O["Orchestrator"]
    end
    U -->|Inbound message| O

    %% ── State & Memory Loading ─────────────────────────────
    subgraph LOAD ["State & Memory Loading"]
        SS[("SessionState store")]
        Z[("Zep memory")]
    end
    O -->|Load state| SS
    SS -->|SessionState| O
    O -->|Read preferences| Z
    Z -->|Preference summary| O

    %% ── Perception & Planning ──────────────────────────────
    subgraph PLAN ["Perception & Planning"]
        P["Update or create goals"]
        Dep["Set Missing & NextQuestion"]
    end
    O -->|Perception and planning| P
    P -->|Compute missing and question| Dep

    %% ── Goal Interleaving ──────────────────────────────────
    subgraph INTERLEAVE ["Goal Interleaving"]
        I{"Interleave?"}
        SUSP["Suspend current goal"]
        PUSH["Push new goal to GoalStack"]
        SETA["Set ActiveGoalID → new goal"]
        SETA2["Keep current ActiveGoalID"]
        SEL["Selected active goal"]
    end
    Dep --> I
    I -->|Yes| SUSP
    SUSP --> PUSH
    PUSH --> SETA
    I -->|No| SETA2
    SETA --> SEL
    SETA2 --> SEL

    %% ── Agent Dispatch ─────────────────────────────────────
    subgraph DISPATCH ["Agent Dispatch"]
        GT{"Goal type?"}
        SA["Sales agent"]
        SU["Support agent"]
        M{"Missing info?"}
    end
    SEL --> GT
    GT -->|Sales| SA
    GT -->|Support| SU
    SA --> M
    SU --> M

    %% ── Agent Execution ────────────────────────────────────
    subgraph EXEC ["Agent Execution"]
        ASK["Ask NextQuestion"]
        REQ["Request tools"]
        TG["Tool gateway"]
        FIN["Agent finalize"]
    end
    M -->|Yes| ASK
    M -->|No| REQ
    REQ -->|Execute| TG
    TG -->|Tool results| FIN

    %% ── State Persistence ──────────────────────────────────
    subgraph PERSIST ["State Persistence"]
        COMMIT["Commit slots & status"]
        SAVE1[("Save SessionState")]
        SAVE2[("Save SessionState")]
    end
    ASK -->|Save| SAVE1
    FIN -->|Commit updates| COMMIT
    COMMIT -->|Save| SAVE2

    %% ── Response ───────────────────────────────────────────
    subgraph REPLY ["Response"]
        R(["Reply to user"])
    end
    SAVE1 -->|Reply| R
    SAVE2 -->|Reply| R
    R -.->|Next message starts new turn| U

    %% ── Styles ─────────────────────────────────────────────
    style ENTRY   fill:#e8f5e9,stroke:#388e3c,stroke-width:2px,color:#1b5e20
    style LOAD    fill:#e3f2fd,stroke:#1565c0,stroke-width:2px,color:#0d47a1
    style PLAN    fill:#fff3e0,stroke:#ef6c00,stroke-width:2px,color:#e65100
    style INTERLEAVE fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px,color:#4a148c
    style DISPATCH fill:#fce4ec,stroke:#c62828,stroke-width:2px,color:#b71c1c
    style EXEC    fill:#e0f7fa,stroke:#00838f,stroke-width:2px,color:#006064
    style PERSIST fill:#f1f8e9,stroke:#558b2f,stroke-width:2px,color:#33691e
    style REPLY   fill:#ede7f6,stroke:#4527a0,stroke-width:2px,color:#311b92

    style U    fill:#a5d6a7,stroke:#2e7d32,stroke-width:2px,color:#1b5e20
    style O    fill:#81d4fa,stroke:#0277bd,stroke-width:2px,color:#01579b
    style I    fill:#ce93d8,stroke:#6a1b9a,stroke-width:2px,color:#4a148c
    style GT   fill:#ef9a9a,stroke:#c62828,stroke-width:2px,color:#b71c1c
    style M    fill:#ef9a9a,stroke:#c62828,stroke-width:2px,color:#b71c1c
    style R    fill:#b39ddb,stroke:#4527a0,stroke-width:2px,color:#311b92
    style SS   fill:#90caf9,stroke:#1565c0,stroke-width:2px,color:#0d47a1
    style Z    fill:#90caf9,stroke:#1565c0,stroke-width:2px,color:#0d47a1
    style SAVE1 fill:#c5e1a5,stroke:#558b2f,stroke-width:2px,color:#33691e
    style SAVE2 fill:#c5e1a5,stroke:#558b2f,stroke-width:2px,color:#33691e
    style TG   fill:#80deea,stroke:#00838f,stroke-width:2px,color:#006064
```