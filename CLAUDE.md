# chetoru

Telegram-бот: чеченско-русский словарь. Go 1.25, SQLite (modernc.org/sqlite) + goose,
Redis-кэш, данные из dosham.app GraphQL API.

**Лицензия данных dosham:** некоммерческое использование, обязательная ссылка на
https://dosham.app. Атрибуция живёт в `/start` (`internal/net/service.go`) — не удалять.

## Skill routing

When the user's request matches an available skill, invoke it via the Skill tool. When in doubt, invoke the skill.

Key routing rules:
- Product ideas/brainstorming → invoke /office-hours
- Strategy/scope → invoke /plan-ceo-review
- Architecture → invoke /plan-eng-review
- Design system/plan review → invoke /design-consultation or /plan-design-review
- Full review pipeline → invoke /autoplan
- Bugs/errors → invoke /investigate
- QA/testing site behavior → invoke /qa or /qa-only
- Code review/diff check → invoke /review
- Visual polish → invoke /design-review
- Ship/deploy/PR → invoke /ship or /land-and-deploy
- Save progress → invoke /context-save
- Resume context → invoke /context-restore
- Author a backlog-ready spec/issue → invoke /spec
