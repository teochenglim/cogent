import json
from cogent.sdk.schema import AgentEvent


def test_minimal_valid_event():
    event = AgentEvent(
        trace_id="t1",
        span_id="s1",
        start_time=1700000000.0,
        end_time=1700000001.0,
        duration_ms=1000.0,
        agent_name="test-agent",
        operation="llm_call",
        service_name="svc",
        environment="test",
    )
    assert event.trace_id == "t1"
    assert event.parent_span_id is None
    assert event.model is None


def test_fully_populated_event():
    event = AgentEvent(
        trace_id="t1",
        span_id="s1",
        parent_span_id="p1",
        start_time=1700000000.0,
        end_time=1700000001.2,
        duration_ms=1200.0,
        agent_name="agent",
        operation="llm_call",
        service_name="svc",
        environment="prod",
        model="claude-sonnet-4-6",
        provider="anthropic",
        input_tokens=100,
        output_tokens=50,
        cost_usd=0.0021,
        finish_reason="stop",
        prompt_preview="Hello",
        prompt_ref="t1/s1/prompt",
        prompt_size_bytes=5,
        completion_preview="Hi",
        completion_ref="t1/s1/completion",
        completion_size_bytes=2,
        eval_score=0.9,
        eval_label="good",
        eval_source="realtime",
    )
    assert event.model == "claude-sonnet-4-6"
    assert event.eval_score == 0.9


def test_optional_fields_default_none():
    event = AgentEvent(
        trace_id="t1",
        span_id="s1",
        start_time=1.0,
        end_time=2.0,
        duration_ms=1000.0,
        agent_name="a",
        operation="llm_call",
        service_name="s",
        environment="e",
    )
    assert event.model is None
    assert event.tool_name is None
    assert event.eval_score is None
    assert event.metadata is None


def test_json_round_trip():
    event = AgentEvent(
        trace_id="t1",
        span_id="s1",
        parent_span_id="p1",
        start_time=1700000000.123456,
        end_time=1700000001.0,
        duration_ms=876.544,
        agent_name="agent",
        operation="tool_call",
        service_name="svc",
        environment="prod",
        tool_name="web_search",
        tool_input_preview="query",
        tool_input_ref="t1/s1/tool_input",
        tool_input_size_bytes=5,
    )
    data = json.loads(event.model_dump_json())
    restored = AgentEvent(**data)
    assert restored.trace_id == event.trace_id
    assert restored.tool_name == "web_search"
    assert restored.start_time == event.start_time
