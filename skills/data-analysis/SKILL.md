---
name: data-analysis
description: Structured data aggregation, JSON/CSV statistical processing, and Markdown/JSON report generation using CORTEX collections and Python/shell scripts.
license: MIT
---

# Data Analysis & Reporting Skill

Use this skill when analyzing datasets, summarizing CORTEX JSON collections, or generating quantitative reports:

1. **Load & Inspect Data**:
   - For application records, use `collection_list` and `collection_get` to inspect schema and sample documents.
   - For local CSV/JSON/log files, use `Read` or a short Python script via `Bash` to inspect headers, row counts, and missing values.
2. **Compute Deterministically**:
   - Never guess aggregates or math in free text. Execute a compact Python one-liner (`python3 -c "..."` or `python -c "..."`) to calculate exact sums, averages, percentiles, and groupings.
3. **Persist & Present Results**:
   - Store structured output back into a domain collection via `collection_put` (e.g. `collection: "reports"` or `"metrics"`) so the Next.js UI can render it.
   - Present a clear Markdown summary with tables and key takeaways in the user's active locale.
