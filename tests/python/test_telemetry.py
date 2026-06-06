import asyncio
import json
import pytest
from cogent.sdk.telemetry import AgentTelemetry
from cogent.sdk.decorators import observe, set_telemetry


def make_tel(mock_producer, mock_offloader=None):
    tel = AgentTelemetry(
        bootstrap_servers="localhost:9092",
        topic="cogent-telemetry",
        offloader=mock_offloader,
        service_name="test-svc",
        environment="test",
    )
    tel._producer = mock_producer
    return tel


def test_span_emits_one_kafka_message(mock_producer, mock_offloader):
    tel = make_tel(mock_producer, mock_offloader)
    with tel.span("llm_call", agent_name="agent") as span:
        span.log(prompt="hello", completion="world")
    assert mock_producer.send.call_count == 1


def test_trace_id_propagates_to_nested_spans(mock_producer, mock_offloader):
    tel = make_tel(mock_producer, mock_offloader)
    emitted = []
    mock_producer.send.side_effect = lambda topic, value: emitted.append(value)

    with tel.span("llm_call", agent_name="parent") as outer:
        outer.log(prompt="p")
        with tel.span("tool_call", agent_name="child") as inner:
            inner.log(tool_name="search", tool_input="q", tool_output="r")

    assert len(emitted) == 2
    outer_ev = json.loads(emitted[0])
    inner_ev = json.loads(emitted[1])
    assert outer_ev["trace_id"] == inner_ev["trace_id"]


def test_parent_span_id_set_for_child(mock_producer, mock_offloader):
    tel = make_tel(mock_producer, mock_offloader)
    emitted = []
    mock_producer.send.side_effect = lambda topic, value: emitted.append(value)

    with tel.span("llm_call", agent_name="parent") as outer:
        outer.log(prompt="p")
        with tel.span("tool_call", agent_name="child") as inner:
            inner.log(tool_name="s", tool_input="q", tool_output="r")

    outer_ev = json.loads(emitted[0])
    inner_ev = json.loads(emitted[1])
    assert inner_ev["parent_span_id"] == outer_ev["span_id"]


def test_auto_emit_on_exit_when_log_not_called(mock_producer):
    tel = make_tel(mock_producer)
    with tel.span("llm_call", agent_name="agent"):
        pass
    assert mock_producer.send.call_count == 1


def test_exception_captured_as_tool_error_and_reraised(mock_producer):
    tel = make_tel(mock_producer)
    emitted = []
    mock_producer.send.side_effect = lambda topic, value: emitted.append(value)

    with pytest.raises(ValueError, match="boom"):
        with tel.span("tool_call", agent_name="agent"):
            raise ValueError("boom")

    assert len(emitted) == 1
    event = json.loads(emitted[0])
    assert "boom" in event["tool_error"]


async def test_concurrent_asyncio_tasks_isolated(mock_producer):
    tel = make_tel(mock_producer)
    emitted = []
    mock_producer.send.side_effect = lambda topic, value: emitted.append(value)

    async def run_a():
        async with tel.span("llm_call", agent_name="a") as s:
            await asyncio.sleep(0.01)
            s.log(prompt="a")

    async def run_b():
        async with tel.span("llm_call", agent_name="b") as s:
            await asyncio.sleep(0.01)
            s.log(prompt="b")

    await asyncio.gather(run_a(), run_b())
    assert len(emitted) == 2
    evs = [json.loads(e) for e in emitted]
    # independent tasks start with no inherited trace → each gets its own
    assert len({e["trace_id"] for e in evs}) == 2


def test_observe_decorator_sync(mock_producer):
    tel = make_tel(mock_producer)
    set_telemetry(tel)
    emitted = []
    mock_producer.send.side_effect = lambda topic, value: emitted.append(value)

    @observe(operation="llm_call", agent_name="decorated")
    def my_fn() -> str:
        return "result"

    my_fn()
    assert len(emitted) == 1
    event = json.loads(emitted[0])
    assert event["operation"] == "llm_call"
    assert event["completion_preview"] == "result"
