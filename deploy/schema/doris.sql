-- Cogent warm tier — full content + inverted indexes for full-text search.
-- This is the capability that caused Braintrust to abandon ClickHouse.
CREATE DATABASE IF NOT EXISTS cogent;
USE cogent;

CREATE TABLE IF NOT EXISTS agent_telemetry (
  trace_id               VARCHAR(36)   NOT NULL,
  span_id                VARCHAR(36)   NOT NULL,
  parent_span_id         VARCHAR(36),
  start_time             DOUBLE,
  end_time               DOUBLE,
  duration_ms            DOUBLE,
  agent_name             VARCHAR(128),
  operation              VARCHAR(64),
  service_name           VARCHAR(128),
  environment            VARCHAR(32),
  model                  VARCHAR(128),
  provider               VARCHAR(64),
  input_tokens           INT,
  output_tokens          INT,
  cost_usd               DOUBLE,
  finish_reason          VARCHAR(32),
  prompt                 TEXT,
  prompt_preview         VARCHAR(512),
  prompt_ref             VARCHAR(512),
  prompt_size_bytes      BIGINT,
  completion             TEXT,
  completion_preview     VARCHAR(512),
  completion_ref         VARCHAR(512),
  completion_size_bytes  BIGINT,
  tool_name              VARCHAR(128),
  tool_input             TEXT,
  tool_input_preview     VARCHAR(512),
  tool_input_ref         VARCHAR(512),
  tool_input_size_bytes  BIGINT,
  tool_output            TEXT,
  tool_output_preview    VARCHAR(512),
  tool_output_ref        VARCHAR(512),
  tool_output_size_bytes BIGINT,
  tool_error             TEXT,
  eval_score             DOUBLE,
  eval_label             VARCHAR(64),
  eval_reason            TEXT,
  eval_source            VARCHAR(32),
  metadata               TEXT,
  dt                     DATE          NOT NULL
)
DUPLICATE KEY(trace_id, span_id)
PARTITION BY RANGE(dt)(
  PARTITION p_default VALUES LESS THAN ("2099-01-01")
)
DISTRIBUTED BY HASH(trace_id) BUCKETS 8
PROPERTIES ("replication_num" = "1");

-- Inverted indexes for full-text search (requires Doris 2.0+)
CREATE INDEX IF NOT EXISTS idx_prompt
  ON agent_telemetry(prompt) USING INVERTED;
CREATE INDEX IF NOT EXISTS idx_completion
  ON agent_telemetry(completion) USING INVERTED;
CREATE INDEX IF NOT EXISTS idx_tool_output
  ON agent_telemetry(tool_output) USING INVERTED;
CREATE INDEX IF NOT EXISTS idx_tool_error
  ON agent_telemetry(tool_error) USING INVERTED;
