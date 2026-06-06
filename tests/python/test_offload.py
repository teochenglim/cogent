import pytest
from unittest.mock import MagicMock, patch
from cogent.sdk.offload import PayloadOffloader


@pytest.fixture
def noop_offloader():
    return PayloadOffloader(endpoint=None, bucket="b", access_key="a", secret_key="s")


@pytest.fixture
def minio_offloader():
    with patch("cogent.sdk.offload.Minio") as mock_minio:
        client = MagicMock()
        mock_minio.return_value = client
        offloader = PayloadOffloader(
            endpoint="localhost:9000",
            bucket="cogent-payloads",
            access_key="minioadmin",
            secret_key="minioadmin",
        )
        yield offloader, client


def test_preview_is_first_500_chars(noop_offloader):
    preview, _, _ = noop_offloader.prepare("prompt", "x" * 1000, "t1", "s1")
    assert preview == "x" * 500


def test_preview_shorter_than_500(noop_offloader):
    preview, _, _ = noop_offloader.prepare("prompt", "short", "t1", "s1")
    assert preview == "short"


def test_noop_ref_is_none(noop_offloader):
    _, ref, _ = noop_offloader.prepare("completion", "hello", "t1", "s1")
    assert ref is None


def test_size_bytes_populated(noop_offloader):
    _, _, size = noop_offloader.prepare("prompt", "hello", "t1", "s1")
    assert size == len("hello".encode())


def test_ref_key_format_with_minio(minio_offloader):
    offloader, _ = minio_offloader
    _, ref, _ = offloader.prepare("prompt", "hello world", "trace-1", "span-1")
    assert ref == "trace-1/span-1/prompt"


def test_all_four_field_names_work(noop_offloader):
    for field in ["prompt", "completion", "tool_input", "tool_output"]:
        preview, ref, size = noop_offloader.prepare(field, "content", "t", "s")
        assert preview == "content"
        assert size > 0
