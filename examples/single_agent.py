"""Cogent SDK — single agent example with randomised realistic payloads."""

import os
import pathlib
import random

from cogent.sdk import AgentTelemetry, PayloadOffloader

# Load .env from repo root if env vars aren't already set
_env = pathlib.Path(__file__).parent.parent / ".env"
if _env.exists():
    for _line in _env.read_text().splitlines():
        _line = _line.strip()
        if _line and not _line.startswith("#") and "=" in _line:
            _k, _, _v = _line.partition("=")
            os.environ.setdefault(_k.strip(), _v.strip())

offloader = PayloadOffloader(
    endpoint=os.getenv("MINIO_ENDPOINT"),
    bucket=os.getenv("MINIO_BUCKET", "cogent-payloads"),
    access_key=os.getenv("MINIO_ACCESS_KEY", "minioadmin"),
    secret_key=os.getenv("MINIO_SECRET_KEY", "minioadmin"),
)

telemetry = AgentTelemetry(
    bootstrap_servers=os.getenv("BOOTSTRAP_SERVERS", "localhost:9092"),
    offloader=offloader,
    service_name="support-bot",
    environment=os.getenv("ENVIRONMENT", "production"),
)

SCENARIOS = [
    {
        "agent": "support-classifier",
        "model": "claude-haiku-4-5-20251001",
        "provider": "anthropic",
        "prompt": (
            "You are a customer support triage agent. Classify the following ticket "
            "into one of: billing, technical, account, general.\n\n"
            "Ticket: My payment failed three times this week but I can see the charges "
            "on my credit card statement. I need this resolved urgently before my renewal date."
        ),
        "completion": (
            "Category: billing\n"
            "Priority: high\n"
            "Reason: Customer reports duplicate failed charges appearing on statement. "
            "Renewal deadline creates urgency. Route to billing team with escalation flag."
        ),
        "input_tokens": 112,
        "output_tokens": 48,
        "cost_usd": 0.000032,
        "finish_reason": "stop",
    },
    {
        "agent": "support-classifier",
        "model": "claude-haiku-4-5-20251001",
        "provider": "anthropic",
        "prompt": (
            "You are a customer support triage agent. Classify the following ticket "
            "into one of: billing, technical, account, general.\n\n"
            "Ticket: I can't log in — it says my password is wrong but I just reset it "
            "five minutes ago. Chrome, MacOS 15. Tried incognito, same issue."
        ),
        "completion": (
            "Category: technical\n"
            "Priority: medium\n"
            "Reason: Password reset not propagating — likely cache or token invalidation issue. "
            "Recommend account team check session store. Not billing-related."
        ),
        "input_tokens": 98,
        "output_tokens": 44,
        "cost_usd": 0.000028,
        "finish_reason": "stop",
    },
    {
        "agent": "draft-responder",
        "model": "claude-sonnet-4-6",
        "provider": "anthropic",
        "prompt": (
            "Draft a professional support reply for the following customer ticket. "
            "Be empathetic, concise, and include next steps.\n\n"
            "Ticket: I've been a customer for 4 years and this is the third outage this month. "
            "My team relies on your API for production traffic. This is unacceptable. "
            "I want compensation and a post-mortem report."
        ),
        "completion": (
            "Hi Sarah,\n\n"
            "Thank you for reaching out and for being a long-standing customer — four years means a "
            "great deal to us, and I completely understand your frustration.\n\n"
            "You're right that three incidents in a single month is not the standard we hold ourselves "
            "to. I've flagged your account for our reliability team and escalated your request for "
            "a formal post-mortem. You'll receive a detailed incident report within 48 hours.\n\n"
            "Regarding compensation: I've applied a 30-day credit to your account, which will appear "
            "on your next invoice. Our team lead will also reach out directly to discuss SLA options "
            "that better fit your production requirements.\n\n"
            "Again, I'm sorry for the disruption to your team. We're committed to doing better.\n\n"
            "Best,\nAlex — Cogent Support"
        ),
        "input_tokens": 187,
        "output_tokens": 193,
        "cost_usd": 0.00189,
        "finish_reason": "stop",
    },
    {
        "agent": "kb-search-agent",
        "model": "claude-sonnet-4-6",
        "provider": "anthropic",
        "prompt": (
            "A customer is asking: 'How do I export my data in GDPR format and what fields are included?'\n\n"
            "Search the knowledge base and return a direct, accurate answer. "
            "If the answer is not in the knowledge base, say so clearly."
        ),
        "completion": (
            "To export your data in GDPR-compliant format:\n\n"
            "1. Navigate to **Settings → Privacy → Data Export**\n"
            "2. Select 'GDPR Package' from the format dropdown\n"
            "3. Click 'Request Export' — you'll receive an email within 24 hours\n\n"
            "The export includes: account profile, all traces and spans from the last 24 months, "
            "billing history, API keys (hashed), and audit logs. Raw prompt/completion payloads "
            "are included only if you have the 'Include Payload Data' setting enabled under Privacy.\n\n"
            "Export files are encrypted with AES-256 and the download link expires after 72 hours."
        ),
        "input_tokens": 143,
        "output_tokens": 141,
        "cost_usd": 0.00134,
        "finish_reason": "stop",
    },
]

scenario = random.choice(SCENARIOS)

with telemetry.span("llm_call", agent_name=scenario["agent"]) as span:
    span.log(
        prompt=scenario["prompt"],
        completion=scenario["completion"],
        model=scenario["model"],
        provider=scenario["provider"],
        input_tokens=scenario["input_tokens"] + random.randint(-10, 10),
        output_tokens=scenario["output_tokens"] + random.randint(-8, 8),
        cost_usd=round(scenario["cost_usd"] * random.uniform(0.85, 1.15), 6),
        finish_reason=scenario["finish_reason"],
    )
    print(f"[{scenario['agent']}] span={span._span_id[:8]}  trace={span._trace_id[:8]}")
