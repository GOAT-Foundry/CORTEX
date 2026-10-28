---
name: scheduled-monitoring
description: Patterns for recurring Cron/interval checks, one-shot deferred follow-ups, and stateful alerting using schedule_timer and CORTEX collections.
license: MIT
---

# Scheduled Monitoring & Autonomous Follow-Up Skill

Use this skill when the user asks to monitor a resource periodically, run a daily/hourly digest, or schedule a future reminder/check:

1. **Check Existing Timers First**:
   - Call `list_timers` before creating a new timer to avoid duplicate schedules.
2. **Choose the Right Schedule Format (`schedule_timer`)**:
   - **Fixed Interval**: `"15m"`, `"1h"`, `"6h"` (minimum `5s`) for continuous polling.
   - **Cron Expression**: 5-field UTC cron such as `"0 * * * *"` (hourly) or `"0 9 * * 1-5"` (weekdays at 09:00 UTC).
   - **One-Shot Deferred Task**: `schedule: "once"` with an explicit RFC3339 `run_at` timestamp (e.g. `"2026-12-31T09:00:00Z"`).
3. **Design Self-Contained Timer Prompts**:
   - Each timer execution runs in an isolated agent session. Write the `prompt` field so it explicitly states what to check, which collection to compare against (`collection_get`), and where to write the updated state or alert (`collection_put`).
