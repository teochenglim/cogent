from __future__ import annotations
import logging
import time
import uuid
from contextvars import ContextVar, Token
from typing import Optional, TYPE_CHECKING

from .schema import AgentEvent

try:
    from kafka import KafkaProducer
except ImportError:
    KafkaProducer = None  # type: ignore[assignment,misc]

if TYPE_CHECKING:
    from .offload import PayloadOffloader

logger = logging.getLogger(__name__)

_CONTENT_FIELDS = ("prompt", "completion", "tool_input", "tool_output")

_current_trace_id: ContextVar[Optional[str]] = ContextVar(
    "cogent_trace_id", default=None
)
_current_span_id: ContextVar[Optional[str]] = ContextVar("cogent_span_id", default=None)


class AgentTelemetry:
    """
    Initialise once per process. Reuse across all agents.
    KafkaProducer is thread-safe and created once.
    """

    def __init__(
        self,
        bootstrap_servers: str,
        topic: str = "cogent-telemetry",
        offloader: Optional["PayloadOffloader"] = None,
        service_name: str = "default",
        environment: str = "production",
    ) -> None:
        self._producer = KafkaProducer(bootstrap_servers=bootstrap_servers)
        self._topic = topic
        self._offloader = offloader
        self._service_name = service_name
        self._environment = environment

    def span(self, operation: str, agent_name: str, **meta) -> "_Span":
        """Create a span context manager. Extra kwargs stored as event fields."""
        return _Span(self, operation, agent_name, meta)


class _Span:
    """Context manager for one agent action. Supports sync (with) and async (async with)."""

    def __init__(
        self, tel: AgentTelemetry, operation: str, agent_name: str, meta: dict
    ):
        self._tel = tel
        self._operation = operation
        self._agent_name = agent_name
        self._meta = meta
        self._logged = False
        self._trace_token: Optional[Token] = None
        self._span_token: Optional[Token] = None
        self._span_id: Optional[str] = None
        self._trace_id: Optional[str] = None
        self._parent_span_id: Optional[str] = None
        self._start_time: float = 0.0

    def _enter(self) -> "_Span":
        self._span_id = str(uuid.uuid4())
        trace_id = _current_trace_id.get()
        if trace_id is None:
            trace_id = str(uuid.uuid4())
        self._trace_id = trace_id
        self._parent_span_id = _current_span_id.get()
        self._start_time = time.time()
        self._trace_token = _current_trace_id.set(self._trace_id)
        self._span_token = _current_span_id.set(self._span_id)
        return self

    def _exit(self, exc_val: Optional[BaseException]) -> None:
        _current_trace_id.reset(self._trace_token)
        _current_span_id.reset(self._span_token)
        if not self._logged:
            fields: dict = {}
            if exc_val is not None:
                fields["tool_error"] = str(exc_val)
            self.log(**fields)

    def __enter__(self) -> "_Span":
        return self._enter()

    def __exit__(self, exc_type, exc_val, exc_tb):
        self._exit(exc_val)
        return False

    async def __aenter__(self) -> "_Span":
        return self._enter()

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        self._exit(exc_val)
        return False

    def log(self, **fields) -> None:
        """Build, validate, and emit the event. Idempotent — second call is a no-op."""
        if self._logged:
            return
        self._logged = True

        end_time = time.time()
        extra: dict = {}

        for field in _CONTENT_FIELDS:
            content = fields.pop(field, None)
            if content is None:
                continue
            if self._tel._offloader is not None:
                preview, ref, size = self._tel._offloader.prepare(
                    field, content, self._trace_id, self._span_id
                )
            else:
                preview = content[:500]
                ref = None
                size = len(content.encode())
            extra[f"{field}_preview"] = preview
            if ref is not None:
                extra[f"{field}_ref"] = ref
            extra[f"{field}_size_bytes"] = size

        combined = {**self._meta, **fields, **extra}

        event = AgentEvent(
            trace_id=self._trace_id,
            span_id=self._span_id,
            parent_span_id=self._parent_span_id,
            start_time=self._start_time,
            end_time=end_time,
            duration_ms=(end_time - self._start_time) * 1000,
            agent_name=self._agent_name,
            operation=self._operation,
            service_name=self._tel._service_name,
            environment=self._tel._environment,
            **combined,
        )

        payload = event.model_dump_json(exclude_none=True).encode()
        try:
            self._tel._producer.send(self._tel._topic, payload)
            self._tel._producer.flush(timeout=10)
        except Exception:
            logger.exception("Failed to emit span %s to Kafka", self._span_id)
