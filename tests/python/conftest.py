import pytest
from unittest.mock import MagicMock, patch


@pytest.fixture
def mock_producer():
    with patch("cogent.sdk.telemetry.KafkaProducer") as mock_cls:
        producer = MagicMock()
        mock_cls.return_value = producer
        yield producer


@pytest.fixture
def mock_offloader():
    offloader = MagicMock()
    offloader.prepare.side_effect = lambda field, content, trace_id, span_id: (
        content[:500],
        f"{trace_id}/{span_id}/{field}",
        len(content.encode()),
    )
    offloader.fetch.return_value = "full content"
    return offloader
