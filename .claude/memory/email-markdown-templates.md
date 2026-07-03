---
name: email-markdown-templates
description: All email bodies render from one Markdown template each via renderMarkdownEmail (single source); text+HTML no longer maintained separately.
metadata:
  type: project
---

Email rendering (refactored 2026-07, branch `refactor/email-templates`): every email body is ONE Markdown Go-template (`tmpl/<name>.md.tmpl`), rendered by `plexams.renderMarkdownEmail(name, jira, data) (text, html, err)` in `plexams/email_markdown.go`:
- **text part** = the rendered Markdown (readable as-is) + shared chrome (JIRA note when jira, generator footer), added once in `assembleTextEmail` so text/HTML can't drift.
- **html part** = Markdown→HTML (blackfriday, CommonExtensions+**HardLineBreak** so authored line breaks survive), wrapped in `emailBaseHTML.tmpl` (+ `jiraOnHTML.tmpl` callout when jira) via `wrapContentHTML`.

Killed the old per-email `xEmail.tmpl` (text) + `xEmailHTML.tmpl` (HTML) double-maintenance and the `renderMailHTML` helper. Only `emailBaseHTML.tmpl` + `jiraOnHTML.tmpl` layout templates remain. Templates embedded via glob `//go:embed tmpl/*.tmpl`.

Per-email JIRA note is generic (chrome); email-specific ticket links (e.g. /create/249) live in the Markdown body. Standalone attachments that happen to be Markdown (e.g. `assembledExamMarkdown.tmpl` — the registrations list) are separate documents, already single-source, left as-is.

Safety net: `plexams/email_golden_test.go` — `TestAllEmailTemplatesParse` (parses every embedded template) + per-email golden tests against `plexams/testdata/email/*` (refresh with `go test ./plexams/ -run <T> -update`). `testdata/` is excluded from the trailing-whitespace/end-of-file pre-commit hooks so goldens stay byte-exact.

Dead code removed during the sweep: `handicapEmail`(+HTML) + HandicapsEmail/HandicapExam/HandicapStudent structs; `publishedEmailInvigilations`(+HTML) broadcast (superseded by the per-teacher personal mail).

PHASE 2 DONE (2026-07, branch feat/db-email-templates): DB-backed, GUI-editable templates as an override layer. Global collection `email_templates` in the "plexams" DB (name→markdown override); db.EmailTemplateOverride(s)/Set/Delete. renderMarkdownEmail resolves override→embedded default via markdownTemplateSource (nil dbClient → default, so goldens unaffected). plexams.EmailTemplates/SetEmailTemplate(validated: must parse)/ResetEmailTemplate. GraphQL: query emailTemplates ([{name, markdown (effective), isDefault, defaultMarkdown}]), mutations setEmailTemplate(name,markdown)/resetEmailTemplate(name). Only *.md.tmpl bodies are editable (not the emailBaseHTML/jiraOnHTML layout). Integration-tested against real MongoDB.

PHASE 3 DONE (2026-07, branch refactor/email-package): extracted the mail toolkit into package `plexams/email` — email.Renderer (Markdown→text+HTML, layout, override store List/Set/Reset) with INJECTED funcs+jiraURL (shared helpers pluralN/jiraURL/zpaURL/constraintsText stay in plexams, also used by pdf/statistics/preplan → no cycle), email.TemplateStore interface (*db.DB satisfies), and email.Sender (SMTPConfig + Attachment, buildMsg/SMTP/dry-run/.eml collector — go-mail confined here). plexams keeps thin delegates (mailRenderer/sendMail/recipientInfo/Begin+FlushMailCollection/SendTestMail) + `type mailAttachment = email.Attachment`; sender built in NewPlexams. The Send* data-gathering funcs stay in plexams (application layer). Behaviour-preserving (goldens/parse guard/override integration test green).

NEXT (planned): GUI editor for the DB-editable templates (Markdown textarea + variable hints + preview + reset). See [[emails-over-graphql]].
