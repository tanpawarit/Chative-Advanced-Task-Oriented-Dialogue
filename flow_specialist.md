# Specialist Internal Flow

```mermaid
flowchart TD
    Start([Start]) --> IN["Input SpecialistRequest"]

    IN --> V1{"ActiveGoal exists"}
    V1 -- No --> E1["ErrValidation"]
    V1 -- Yes --> V2{"ActiveGoal.Type set"}
    V2 -- No --> E1
    V2 -- Yes --> RE("specialist.runReAct")

    RE --> JParse{"Parse JSON from LLM"}
    JParse -- Success --> Out([SpecialistResponse])
    JParse -- Fail --> E2["ErrModelInvoke"]
```

## ReAct Tool Execution Subflow

```mermaid
flowchart TD
    A["runReAct"] --> B["reactAgent Generate"]
    B --> C["reactToolAdapter InvokableRun"]
    C --> D["Parse tool args JSON"]
    D --> E["Execute local tool handler"]
    E --> F["Return ToolResult JSON to ReAct"]
```

## Notes

- Specialist still owns tool execution in this phase (tools run inside ReAct via `reactToolAdapter`).
- Orchestrator contract remains unchanged: it still calls `specialist.Run(...)` once per turn.
