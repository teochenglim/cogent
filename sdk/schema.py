from __future__ import annotations
from typing import Optional
from pydantic import BaseModel


class AgentEvent(BaseModel):
    """Single agent action event. The unit of observability in Cogent."""

    # Correlation — always present
    trace_id: str
    span_id: str
    parent_span_id: Optional[str] = None
    start_time: float
    end_time: float
    duration_ms: float

    # Agent context
    agent_name: str
    operation: (
        str  # llm_call|tool_call|retrieval|handoff|evaluation|human_annotation|custom
    )
    service_name: str
    environment: str

    # LLM metadata — no inline content
    model: Optional[str] = None
    provider: Optional[str] = None
    input_tokens: Optional[int] = None
    output_tokens: Optional[int] = None
    cost_usd: Optional[float] = None
    finish_reason: Optional[str] = None

    # Payload previews (first 500 chars; set whenever content exists)
    prompt_preview: Optional[str] = None
    completion_preview: Optional[str] = None
    tool_input_preview: Optional[str] = None
    tool_output_preview: Optional[str] = None

    # S3 reference keys
    prompt_ref: Optional[str] = None
    completion_ref: Optional[str] = None
    tool_input_ref: Optional[str] = None
    tool_output_ref: Optional[str] = None

    # Full payload sizes (set whenever content exists)
    prompt_size_bytes: Optional[int] = None
    completion_size_bytes: Optional[int] = None
    tool_input_size_bytes: Optional[int] = None
    tool_output_size_bytes: Optional[int] = None

    # Tool metadata
    tool_name: Optional[str] = None
    tool_error: Optional[str] = None  # always inline; errors are small

    # Evaluation
    eval_score: Optional[float] = None
    eval_label: Optional[str] = None
    eval_reason: Optional[str] = None
    eval_source: Optional[str] = None  # realtime|batch_eval|human_annotation

    # Arbitrary metadata
    metadata: Optional[dict] = None
