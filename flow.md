flowchart TD
    %% Define Nodes
    Start((Start))
    
    subgraph "1. Validate Request"
        VR_In[/Input: SessionID, Text/]
        VR_Check{Valid?}
        VR_Out[Create GraphState]
        VR_Err((Error))
    end

    subgraph "2. Load/Create State"
        LCS_In[Load from DB]
        LCS_Check{Found?}
        LCS_New[New SessionState]
        LCS_Set[Set in GraphState]
    end

    subgraph "3. Read Memory"
        RM_Read[Read Profile from DB]
        RM_Set[Set MemorySummary]
    end

    subgraph "4. Plan Goal (LLM)"
        PG_Prompt[Prepare Prompt]
        PG_Call[[Call Planner Agent]]
        PG_Set[Set PlanResp]
    end

    subgraph "5. Apply Plan"
        AP_Decide{New Goal?}
        AP_Create[Create New Goal]
        AP_Push[Push to Stack]
        AP_Resume[Resume Old Goal]
        AP_Set[Set ActiveGoal]
    end

    subgraph "6. Dispatch Specialist (LLM)"
        DS_Pick{Goal Type?}
        DS_Sales[[Pick Sales Specialist]]
        DS_Support[[Pick Support Specialist]]
        DS_Run[[Call specialist.Run once]]
        DS_Internal[Internal in specialist<br/>ReAct tool loop + local tool executor<br/>then structured finalize]
        DS_Set[Set Message & Updates]
    end

    subgraph "7. Apply Updates"
        AU_Update[Update Slots]
        AU_Status[Update Status]
        AU_Finish{Mark Done?}
        AU_Pop[Pop Stack]
    end

    subgraph "8. Save State"
        SS_Val[Validate State]
        SS_Save[Save to DB]
    end

    subgraph "9. Write Memory"
        WM_Check{New Info?}
        WM_Save[Save Profile]
    end

    subgraph "10. Finalize"
        FR_Ext[Extract Message]
        FR_Out[/Output Reply/]
    end

    End((End))

    %% Connections
    Start --> VR_In
    VR_In --> VR_Check
    VR_Check -- Yes --> VR_Out
    VR_Check -- No --> VR_Err
    VR_Out --> LCS_In
    
    LCS_In --> LCS_Check
    LCS_Check -- Yes --> LCS_Set
    LCS_Check -- No --> LCS_New --> LCS_Set
    LCS_Set --> RM_Read

    RM_Read --> RM_Set --> PG_Prompt
    
    PG_Prompt --> PG_Call --> PG_Set --> AP_Decide

    AP_Decide -- New --> AP_Create --> AP_Push --> AP_Set
    AP_Decide -- Resume --> AP_Resume --> AP_Set
    AP_Set --> DS_Pick

    DS_Pick -- Sales --> DS_Sales
    DS_Pick -- Support --> DS_Support
    DS_Sales --> DS_Run
    DS_Support --> DS_Run
    DS_Run --> DS_Internal --> DS_Set
    DS_Set --> AU_Update

    AU_Update --> AU_Status --> AU_Finish
    AU_Finish -- Yes --> AU_Pop --> SS_Val
    AU_Finish -- No --> SS_Val
    
    SS_Val --> SS_Save --> WM_Check
    
    WM_Check -- Yes --> WM_Save --> FR_Ext
    WM_Check -- No --> FR_Ext
    
    FR_Ext --> FR_Out --> End

    %% Styles
    style PG_Call fill:#ff9,stroke:#f66,stroke-width:2px
    style DS_Sales fill:#ff9,stroke:#f66,stroke-width:2px
    style DS_Support fill:#ff9,stroke:#f66,stroke-width:2px
    style DS_Run fill:#ff9,stroke:#f66,stroke-width:2px
    style Start fill:#f9f,stroke:#333
    style End fill:#f9f,stroke:#333
    style VR_Err fill:#f00,color:#fff
