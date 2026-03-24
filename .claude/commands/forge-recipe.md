---
description: Run a forge recipe (analyze, design, blueprint, fix, debug)
argument-hint: "<type> \"description\""
---

Parse the first argument as recipe type: analyze, design, blueprint, fix, or debug.
Use Skill("forge-recipe-{type}") with remaining arguments.

Examples:
  /recipe analyze "auth system"       → Skill("forge-recipe-analyze")
  /recipe design "OAuth2 login"       → Skill("forge-recipe-design")
  /recipe blueprint "API endpoints"   → Skill("forge-recipe-blueprint")
  /recipe fix "login bcrypt error"    → Skill("forge-recipe-fix")
  /recipe debug "session drops after 5min" → Skill("forge-recipe-debug")
