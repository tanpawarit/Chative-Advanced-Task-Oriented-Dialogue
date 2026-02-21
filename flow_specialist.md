# Specialist Internal Flow

```mermaid
flowchart TD
    Start([Start]) --> IN["Input SpecialistRequest"]

    IN --> V1{"ActiveGoal exists"}
    V1 -- No --> E1["ErrValidation"]
    V1 -- Yes --> V2{"ActiveGoal.Type set"}
    V2 -- No --> E1
    V2 -- Yes --> B{"Blocked (goal.IsBlocked or missing fields)"}

    B -- Yes --> SAsk["runStructured ask mode"]
    SAsk --> Out([SpecialistResponse])

    B -- No --> TR{"Has ToolResults already"}
    TR -- Yes --> SFinal["runStructured finalize mode"]
    SFinal --> Out

    TR -- No --> RE["runReAct act mode"]

    RE --> R0{"Trace has tool results"}
    R0 -- Yes --> SetTR["Set req.ToolResults from trace"]
    SetTR --> SFinal

    R0 -- No --> M{"actMessage non-empty"}
    M -- Yes --> MsgOnly["Return message only"]
    MsgOnly --> Out
    M -- No --> SFallback["runStructured finalize mode (fallback)"]
    SFallback --> Out
```

## ReAct Tool Execution Subflow

```mermaid
flowchart TD
    A["runReAct creates trace (react.WithMessageFuture)"] --> B["reactAgent Generate"]
    B --> C["reactToolAdapter InvokableRun"]
    C --> D["Parse tool args JSON"]
    D --> E["Execute local tool handler"]
    E --> F["Return ToolResult JSON to ReAct"]
    F --> G["Read trace messages"]
    G --> H["Parse tool messages to []ToolResult (skip invalid payload)"]
```

## Notes

- Specialist still owns tool execution in this phase (tools run inside ReAct via `reactToolAdapter`).
- Orchestrator contract remains unchanged: it still calls `specialist.Run(...)` once per turn.
