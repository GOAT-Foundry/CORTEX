---
name: domain-collection-helper
description: Guide for reading and writing structured application state in CORTEX JSON Collections and scheduling follow-up timers.
license: MIT
---

# CORTEX Domain Collection & Timer Skill

When building or operating domain workflows inside CORTEX:

1. **Inspect existing collections** first using `collection_list` with an empty `collection` parameter.
2. **Read records** using `collection_list` (with `collection` set) or `collection_get` (with `collection` and `id`).
3. **Persist domain artifacts or records** using `collection_put` so the Next.js frontend can immediately render them via `/api/collections/{name}`.
4. **Schedule follow-ups** using `schedule_timer` when a task requires periodic monitoring (`schedule: "15m"` or a cron expression) or a deferred check (`schedule: "once"` with `run_at` in RFC3339).
