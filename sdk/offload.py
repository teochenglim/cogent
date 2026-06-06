from __future__ import annotations
import io
import logging
import threading
from typing import Optional

logger = logging.getLogger(__name__)

_PREVIEW_LEN = 500


class PayloadOffloader:
    """
    Splits every content field into a 500-char preview plus an S3 upload.
    No threshold. No conditional logic. All content always goes to S3.
    Pass endpoint=None for no-op mode (local dev without MinIO).
    """

    def __init__(
        self,
        endpoint: Optional[str],
        bucket: str,
        access_key: str,
        secret_key: str,
        secure: bool = False,
    ) -> None:
        self._bucket = bucket
        self._client = None
        if endpoint:
            from minio import Minio

            self._client = Minio(
                endpoint,
                access_key=access_key,
                secret_key=secret_key,
                secure=secure,
            )

    def prepare(
        self,
        field_name: str,
        content: str,
        trace_id: str,
        span_id: str,
    ) -> tuple[str, Optional[str], int]:
        """
        Returns (preview, ref_key, size_bytes).
        Uploads full content to S3 in a background thread (fire and forget).
        ref_key is None in no-op mode (no MinIO endpoint configured).
        """
        preview = content[:_PREVIEW_LEN]
        encoded = content.encode()
        size_bytes = len(encoded)

        if self._client is None:
            return preview, None, size_bytes

        ref_key = f"{trace_id}/{span_id}/{field_name}"
        threading.Thread(
            target=self._upload,
            args=(ref_key, encoded),
            daemon=True,
        ).start()
        return preview, ref_key, size_bytes

    def _upload(self, key: str, data: bytes) -> None:
        try:
            self._client.put_object(
                self._bucket,
                key,
                io.BytesIO(data),
                length=len(data),
            )
        except Exception:
            logger.exception("S3 upload failed for key %s", key)

    def fetch(self, ref_key: str) -> str:
        """Download and return full content by S3 key. Synchronous."""
        if self._client is None:
            raise RuntimeError("PayloadOffloader is in no-op mode; cannot fetch")
        response = self._client.get_object(self._bucket, ref_key)
        try:
            return response.read().decode()
        finally:
            response.close()
            response.release_conn()
