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

NEXT (planned): Phase 2 = DB-backed, GUI-editable templates as an override layer over the embedded Markdown defaults (validate on save, reset-to-default); Phase 3 = extract a `plexams/email` package (needs MailSender/TemplateStore/config interfaces to break the *Plexams coupling). See [[emails-over-graphql]].
