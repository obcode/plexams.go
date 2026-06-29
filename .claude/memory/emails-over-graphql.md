---
name: emails-over-graphql
description: Plan/design for exposing plexams.go email sending over GraphQL (subscriptions) + attachment upload
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Bringing the `email` CLI commands to the GraphQL/GUI, same Reporter+subscription pattern as
validations and ZPA transfers. Branch: feature work after validation_over_graphql.

Design decisions (2026-06-19):
- **Sends are streaming subscriptions** with a `run: Boolean!` arg (default = dry-run). Each
  `Send*` method takes a `Reporter` (spinners → reporter); CLI passes ConsoleReporter.
- **dry-run goes to `planer.Email`** (the user). The old hard-coded `galority@gmail.com` in
  email_cover_pages.go must be removed entirely. Add a helper like `mailTo(run, real...)`.
- **Guard:** email send = exclusive operation, treated like a ZPA transfer (no send during a
  validation; during a send block writes/validations). Generalize opGuard accordingly. See [[git-workflow]].
- **Binary transfer (PNGs/PDFs) browser→server = REST POST** (chi), decoupled from the send
  (upload first, then send — like ZPA-upload→validate). Reasons: cover-page PDFs arrive as a ZIP
  from a colleague and must be uploadable BY HAND; Svelte makes PNGs as blobs. Plain POST handles
  both + arbitrary sizes; gqlgen Upload/base64 are more friction.
- **Unified attachment store** in Mongo `email_attachments { semester, kind, key, filename,
  contentType, data, uploadedAt }`. kind = "cover-page" (key=teacherID) | "invigilation-image"
  (key=invigilatorID). REST: POST /upload/email-attachment (single) + /upload/email-attachments-zip
  (zip). GraphQL: query emailAttachments(kind) (no data) + mutation clearEmailAttachments(kind).
- **CLI parity is required:** everything that works in the GUI must work from the CLI. So the two
  attachment-based sends look up the store FIRST, then fall back to a config dir:
  cover-pages → `coverPages.dir`, published-invigilations → `invigilationStats.dir`.
- **published-invigilations is reworked:** instead of one mail to all profs, send one individual
  mail per invigilator with that invigilator's PNG attached.

Phases (each its own commit): E1 send foundation (guard + Reporter refactor + simple send
subscriptions + dry-run→planer), E2 attachment infra (collection + REST upload + list/clear),
E3 cover-pages from store (dir fallback), E4 published-invigilations rework.

After each phase, give the user GUI-agent instructions for plexams.gui (they drive a separate
agent there).
