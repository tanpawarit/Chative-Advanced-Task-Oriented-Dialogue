# Graph-Only Runtime Flow

This project now runs on **Eino Graph only** for both model and orchestrator layers.
No chain fallback path exists.

## 1. Planner Graph (`planner.model_graph`)

Input: `map[string]any{"input": "<json payload>"}`
Output: `plannerLLMOutput`

Node sequence:
1. `prompt` (`AddChatTemplateNode`)
2. `model` (`AddChatModelNode`)
3. `parse_json` (`AddLambdaNode` with `compose.MessageParser`)

Edges:
`START -> prompt -> model -> parse_json -> END`

Used by:
- `plannerImpl.Plan()` marshals request context and invokes this graph.
- Output is normalized into `PlannerResponse` and schema-validated.

## 2. Specialist Graphs

### 2.1 Structured Specialist Graph (`specialist.structured_graph`)

Input: `map[string]any{"input": "<json payload>"}`
Output: `specialistLLMOutput`

Node sequence:
1. `prompt`
2. `model`
3. `parse_json`

Edges:
`START -> prompt -> model -> parse_json -> END`

### 2.2 Tool Planning Graph (`specialist.tool_planning_graph`)

Input: `map[string]any{"input": "<json payload>"}`
Output: `*schema.Message` (tool calls or direct content)

Node sequence:
1. `prompt`
2. `model`

Edges:
`START -> prompt -> model -> END`

### 2.3 Specialist Runtime Graph (`specialist.runtime_graph`)

Input: `SpecialistRequest`
Output: `SpecialistResponse`

Node sequence:
1. `validate_and_prepare`
2. Branch:
`tool_path` when `!isBlocked && len(tool_results)==0`
`structured_path` otherwise

Edges:
- `START -> validate_and_prepare`
- `tool_path -> END`
- `structured_path -> END`

Behavior:
- `tool_path`: calls tool-planning graph and validates allowed tool list.
- `structured_path`: calls structured graph for `ask/finalize`.

## 3. Orchestrator Graph (`orchestrator.handle_message`)

Input: `orchestratorGraphInput{session_id, text}`
Output: `orchestratorGraphOutput{reply}`

Node sequence:
1. `validate_request`
2. `load_or_create_state`
3. `read_memory`
4. `plan_goal`
5. `apply_plan`
6. `dispatch_specialist`
7. `apply_state_updates`
8. `validate_and_save_state`
9. `write_memory`
10. `finalize_reply`

Edges:
`START -> validate_request -> load_or_create_state -> read_memory -> plan_goal -> apply_plan -> dispatch_specialist -> apply_state_updates -> validate_and_save_state -> write_memory -> finalize_reply -> END`

Behavior guarantees:
- State is always validated before save.
- Memory write happens after save.
- Empty specialist message is rejected at `finalize_reply`.
- Errors are fail-fast and returned to caller.

## 4. Dispatch Subflow (inside `dispatch_specialist`)

For active `sales.*` or `support.*` goal:
1. Specialist pass-1 run.
2. If no tool requests: return message/state updates.
3. If tool requests exist:
- execute tools
- specialist pass-2 run with `tool_results`
- merge state updates from both passes

Constraint:
- pass-2 must not request tools again.

## 5. Error Handling Policy

- Graph compile errors fail startup.
- Runtime node errors are returned immediately.
- No runtime fallback to legacy/chain execution.

