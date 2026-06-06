"""Cogent SDK — multi-agent example with randomised realistic payloads."""

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
    service_name="research-pipeline",
    environment=os.getenv("ENVIRONMENT", "production"),
)

PIPELINES = [
    {
        "topic": "climate change mitigation technologies",
        "orchestrator": {
            "agent": "planner",
            "model": "claude-opus-4-8",
            "provider": "anthropic",
            "prompt": (
                "You are a research orchestration agent. A user has requested a comprehensive "
                "briefing on climate change mitigation technologies.\n\n"
                "Break this into specific sub-tasks for specialist agents: "
                "web search, academic literature review, and data synthesis. "
                "Output a structured task plan."
            ),
            "completion": (
                "Task plan for: Climate change mitigation technologies\n\n"
                "1. [web-researcher] Search recent news and reports (2023-2025) on "
                "carbon capture, green hydrogen, and direct air capture costs\n"
                "2. [arxiv-researcher] Pull top-cited papers on CDR efficiency metrics "
                "and IPCC scenario modelling\n"
                "3. [synthesiser] Merge findings, resolve conflicts, produce 500-word briefing "
                "with cost per tonne CO2 estimates\n\n"
                "Priority: cost trends and scalability. Exclude opinion pieces."
            ),
            "input_tokens": 156,
            "output_tokens": 121,
            "cost_usd": 0.00612,
        },
        "steps": [
            {
                "agent": "web-researcher",
                "operation": "tool_call",
                "tool_name": "web_search",
                "tool_input": "carbon capture direct air capture cost per tonne 2024 2025 site:iea.org OR site:nature.com OR site:bloomberg.com",
                "tool_output": (
                    "Found 8 results.\n"
                    "1. IEA (2024): DAC costs fell 18% YoY to ~$400/tonne. Target: <$100 by 2030.\n"
                    "2. Nature Energy (2024): Orca 2 plant (Iceland) operating at $380/tonne, "
                    "capacity 36,000t/yr. Scaling bottleneck: sorbent manufacturing.\n"
                    "3. Bloomberg NEF (2025): Green hydrogen at $3.20/kg in MENA, parity with grey H2 "
                    "expected 2027. 14 GW electrolyser pipeline announced Q1 2025.\n"
                    "4. Carbon Brief (2024): Enhanced weathering trials show 2-8t CO2/ha/yr at "
                    "$50-180/tonne — cheapest engineered CDR pathway identified so far."
                ),
            },
            {
                "agent": "arxiv-researcher",
                "operation": "tool_call",
                "tool_name": "arxiv_search",
                "tool_input": "carbon dioxide removal CDR efficiency IPCC AR7 cost learning rate",
                "tool_output": (
                    "3 papers retrieved.\n"
                    "• Smith et al. 2024 (2847 citations): CDR learning rates 15-22% per doubling "
                    "of capacity, comparable to solar PV trajectory 2005-2015.\n"
                    "• IPCC WG3 technical annex (2024): 1.5°C pathways require 10 GtCO2/yr CDR by "
                    "2050. Current capacity: 0.01 GtCO2/yr. Gap factor: 1000x.\n"
                    "• Fasihi et al. 2025: Hybrid DAC+geothermal achieves $210/tonne at 10 GW scale. "
                    "Iceland, Kenya, Ethiopia identified as low-cost deployment zones."
                ),
            },
            {
                "agent": "synthesiser",
                "operation": "llm_call",
                "model": "claude-sonnet-4-6",
                "provider": "anthropic",
                "prompt": (
                    "Synthesise the following research findings into a 500-word executive briefing "
                    "on climate change mitigation technologies. Focus on cost trends and scalability. "
                    "Include specific numbers.\n\n"
                    "Web search findings:\n"
                    "DAC costs ~$400/tonne, down 18% YoY. Green hydrogen $3.20/kg in MENA. "
                    "Enhanced weathering $50-180/tonne.\n\n"
                    "Academic findings:\n"
                    "CDR learning rates 15-22%. IPCC requires 10 GtCO2/yr by 2050 vs 0.01 GtCO2/yr "
                    "today. Hybrid DAC+geothermal achieves $210/tonne at scale."
                ),
                "completion": (
                    "# Climate Mitigation Technologies: Cost & Scale Outlook\n\n"
                    "**Direct Air Capture (DAC)** remains the most capital-intensive pathway at "
                    "~$400/tonne CO2, but the cost trajectory is encouraging: an 18% year-on-year "
                    "decline (IEA, 2024) mirrors early solar PV learning curves. Academic analysis "
                    "confirms CDR technologies achieve 15-22% cost reduction per capacity doubling — "
                    "if deployment accelerates, sub-$100/tonne DAC is achievable by the early 2030s. "
                    "The primary bottleneck is sorbent manufacturing, not engineering.\n\n"
                    "**Green hydrogen** has crossed a critical threshold: $3.20/kg in MENA, with "
                    "grid parity against grey hydrogen expected by 2027. A 14 GW electrolyser "
                    "pipeline announced in Q1 2025 suggests industrial confidence in this timeline. "
                    "Hydrogen's dual role — as clean fuel and DAC energy source — makes it a "
                    "structural enabler for the broader CDR stack.\n\n"
                    "**Enhanced weathering** is the dark horse. At $50-180/tonne and deployable "
                    "on existing farmland, it offers the lowest cost floor of any engineered CDR "
                    "pathway identified. Trials show 2-8t CO2/ha/yr removal — meaningful at "
                    "agricultural scale, requiring no new infrastructure.\n\n"
                    "**The gap remains existential.** IPCC WG3 requires 10 GtCO2/yr CDR by 2050. "
                    "Current global capacity is 0.01 GtCO2/yr — a 1000x shortfall. No single "
                    "technology closes this alone. A portfolio approach, with enhanced weathering "
                    "providing near-term volume and DAC scaling toward the 2035+ cost target, "
                    "is the most defensible pathway.\n\n"
                    "**Recommended action:** Prioritise policy environments that accelerate DAC "
                    "learning (45Q-style credits, offtake mandates) while funding enhanced "
                    "weathering field trials to resolve MRV uncertainty."
                ),
                "input_tokens": 312,
                "output_tokens": 387,
                "cost_usd": 0.00421,
            },
        ],
    },
    {
        "topic": "customer churn risk scoring",
        "orchestrator": {
            "agent": "risk-orchestrator",
            "model": "claude-sonnet-4-6",
            "provider": "anthropic",
            "prompt": (
                "You are a churn risk assessment orchestrator. Analyse account ID acct_8821 "
                "for churn risk. Coordinate: (1) pull account activity metrics, "
                "(2) check support ticket sentiment, (3) score and produce recommendation."
            ),
            "completion": (
                "Initiating churn risk pipeline for acct_8821.\n\n"
                "Step 1: Dispatch data-fetcher to pull 90-day activity metrics.\n"
                "Step 2: Dispatch sentiment-analyser on last 5 support tickets.\n"
                "Step 3: Aggregate signals → risk score → recommendation.\n\n"
                "Risk threshold: >0.65 = high risk, trigger outreach workflow."
            ),
            "input_tokens": 143,
            "output_tokens": 78,
            "cost_usd": 0.00071,
        },
        "steps": [
            {
                "agent": "data-fetcher",
                "operation": "tool_call",
                "tool_name": "db_query",
                "tool_input": "SELECT logins_90d, api_calls_90d, feature_adoption_pct, last_active_days FROM accounts WHERE id='acct_8821'",
                "tool_output": "logins_90d=3, api_calls_90d=412, feature_adoption_pct=18, last_active_days=22",
            },
            {
                "agent": "sentiment-analyser",
                "operation": "llm_call",
                "model": "claude-haiku-4-5-20251001",
                "provider": "anthropic",
                "prompt": (
                    "Score the sentiment of these 5 support tickets for account acct_8821. "
                    "Return a JSON object with overall_sentiment (positive/neutral/negative), "
                    "frustration_score (0-1), and key_themes.\n\n"
                    "Ticket 1: 'Your API docs are outdated, cost me 2 days.'\n"
                    "Ticket 2: 'Third time asking — where is the webhook retry feature?'\n"
                    "Ticket 3: 'Response time is fine but the dashboard is unusable on mobile.'\n"
                    "Ticket 4: 'Still waiting on the refund from last month\\'s outage.'\n"
                    "Ticket 5: 'Can you explain why my rate limit dropped without notice?'"
                ),
                "completion": (
                    '{"overall_sentiment": "negative", "frustration_score": 0.82, '
                    '"key_themes": ["documentation gaps", "unresolved requests", '
                    '"billing dispute", "communication failures", "missing features"]}'
                ),
                "input_tokens": 198,
                "output_tokens": 52,
                "cost_usd": 0.000088,
            },
            {
                "agent": "risk-scorer",
                "operation": "llm_call",
                "model": "claude-sonnet-4-6",
                "provider": "anthropic",
                "prompt": (
                    "Score churn risk for acct_8821 given these signals:\n\n"
                    "Activity: logins_90d=3 (low), api_calls_90d=412 (moderate), "
                    "feature_adoption=18% (low), last_active=22 days ago\n\n"
                    "Support sentiment: negative, frustration_score=0.82, "
                    "themes: documentation gaps, unresolved requests, billing dispute\n\n"
                    "Output JSON: {risk_score, risk_tier, top_factors, recommended_action}"
                ),
                "completion": (
                    "{\n"
                    '  "risk_score": 0.79,\n'
                    '  "risk_tier": "high",\n'
                    '  "top_factors": [\n'
                    '    "login frequency dropped 70% vs cohort average",\n'
                    '    "unresolved billing dispute (22 days open)",\n'
                    '    "frustration score 0.82 — top 5% of accounts",\n'
                    '    "feature adoption 18% vs 54% for retained accounts"\n'
                    "  ],\n"
                    '  "recommended_action": "executive sponsor outreach within 48h, '
                    'credit the disputed amount immediately, schedule product demo on webhook roadmap"\n'
                    "}"
                ),
                "input_tokens": 221,
                "output_tokens": 134,
                "cost_usd": 0.00143,
            },
        ],
    },
]

pipeline = random.choice(PIPELINES)
orc = pipeline["orchestrator"]

with telemetry.span("llm_call", agent_name=orc["agent"]) as orchestrator_span:
    orchestrator_span.log(
        prompt=orc["prompt"],
        completion=orc["completion"],
        model=orc["model"],
        provider=orc["provider"],
        input_tokens=orc["input_tokens"] + random.randint(-12, 12),
        output_tokens=orc["output_tokens"] + random.randint(-8, 8),
        cost_usd=round(orc["cost_usd"] * random.uniform(0.9, 1.1), 6),
        finish_reason="stop",
    )
    print(f"[{orc['agent']}] orchestrator  trace={orchestrator_span._trace_id[:8]}")

    for step in pipeline["steps"]:
        with telemetry.span(step["operation"], agent_name=step["agent"]) as child:
            kwargs = {}
            if step["operation"] == "tool_call":
                kwargs = dict(
                    tool_name=step["tool_name"],
                    tool_input=step["tool_input"],
                    tool_output=step["tool_output"],
                )
            else:
                kwargs = dict(
                    prompt=step["prompt"],
                    completion=step["completion"],
                    model=step["model"],
                    provider=step["provider"],
                    input_tokens=step["input_tokens"] + random.randint(-15, 15),
                    output_tokens=step["output_tokens"] + random.randint(-12, 12),
                    cost_usd=round(step["cost_usd"] * random.uniform(0.9, 1.1), 6),
                    finish_reason="stop",
                )
            child.log(**kwargs)
            print(f"  [{step['agent']}] {step['operation']}  span={child._span_id[:8]}")
