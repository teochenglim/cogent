from __future__ import annotations
import functools
import inspect
from typing import Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from .telemetry import AgentTelemetry

_telemetry_instance: Optional["AgentTelemetry"] = None


def set_telemetry(tel: "AgentTelemetry") -> None:
    """Register the global AgentTelemetry instance used by @observe."""
    global _telemetry_instance
    _telemetry_instance = tel


def _get_telemetry() -> "AgentTelemetry":
    if _telemetry_instance is None:
        raise RuntimeError(
            "No AgentTelemetry registered. Call cogent.set_telemetry(tel) first."
        )
    return _telemetry_instance


def observe(
    operation: Optional[str] = None,
    agent_name: str = "unknown",
    **span_meta,
):
    """
    Decorator that wraps sync and async functions in a Cogent span.
    If the return value is str, it is passed as the completion field.
    Exceptions are captured as tool_error and re-raised.
    """

    def decorator(fn):
        op = operation or fn.__name__

        if inspect.iscoroutinefunction(fn):

            @functools.wraps(fn)
            async def async_wrapper(*args, **kwargs):
                tel = _get_telemetry()
                async with tel.span(op, agent_name=agent_name, **span_meta) as span:
                    result = await fn(*args, **kwargs)
                    log_fields: dict = {}
                    if isinstance(result, str):
                        log_fields["completion"] = result
                    span.log(**log_fields)
                    return result

            return async_wrapper
        else:

            @functools.wraps(fn)
            def sync_wrapper(*args, **kwargs):
                tel = _get_telemetry()
                with tel.span(op, agent_name=agent_name, **span_meta) as span:
                    result = fn(*args, **kwargs)
                    log_fields: dict = {}
                    if isinstance(result, str):
                        log_fields["completion"] = result
                    span.log(**log_fields)
                    return result

            return sync_wrapper

    return decorator
