---
name: web-research
description: Methodical web search (ddgs) and page retrieval (WebFetch) workflow for gathering, verifying, and synthesizing factual information with citations.
license: MIT
---

# Web Research & Source Verification Skill

Use this skill whenever a task requires up-to-date facts, external documentation, market/domain research, or multi-source verification:

1. **Formulate Targeted Queries (`web_search`)**:
   - Issue concise, specific search queries via the built-in `web_search` tool (powered by `ddgs` by default, or Brave/Tavily when configured).
   - Run 2–3 distinct queries if the topic spans multiple angles or needs cross-checking.
2. **Inspect Primary Sources (`WebFetch`)**:
   - Do not rely solely on search snippets. Fetch the most authoritative URLs returned by `web_search` using `WebFetch` to read full context.
3. **Cross-Verify & Cite**:
   - Compare claims across at least two independent sources when accuracy is critical.
   - Include concrete source URLs and retrieval dates in your final response or persisted `collection_put` record.
