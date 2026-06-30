# Memory Index

- [CLI→GUI migration](cli-to-gui-migration.md) — plan to move all CLI into the GraphQL/Svelte GUI; config to DB; stays local single-user; secrets stay in file.
- [git workflow](git-workflow.md) — semantic-release + feature branches; commit in steps with Conventional Commits; don't commit .claude/settings.json.
- [GUI & CLI sync](gui-and-cli-sync.md) — every backend change: always emit plexams.gui-agent instructions AND adjust/remove the matching CLI command.
- [emails over GraphQL](emails-over-graphql.md) — email send = streaming subscriptions; attachments via REST; dry-run to smtp.testmail.
- [room requests](room-requests.md) — Gebäudemanagement room requests: requestWith/priority, DB collection, generate-once→apply→email workflow.
- [build binary cleanup](build-binary-cleanup.md) — always rm the ./plexams.go binary after testing (breaks gowatch); prefer `go run .`.
- [planning state model](planning-state-model.md) — workflow as condition/event Petri net; publish-email gates lock generation; planningState/setPlanningCondition; defined in Go.
- [ZPA upload via GUI](zpa-upload-via-gui.md) — plan upload runs via GUI streaming subscriptions; surface failures through the Reporter/returned error; post() errors on non-2xx.
- [invigilator no-duty carryover](invigilator-no-duty-carryover.md) — future: carry Präsident/Dekanin/Mutterschutz "keine Aufsicht" across semesters; reason required / "temporär" flag; err toward keeping exclusion.
- [pre-planning SEB/EXaHM](preplanning-seb-exahm.md) — new feature: manual pseudo-exams in next-semester DB to size early Anny room bookings; global StudyProgram entity; GUI-only; link to ZPA ancode later.
- [ZPA import behaviors](zpa-import-behaviors.md) — import auto-presets to-plan (schriftlich/praktisch→plan, rest→not); stale banners only after first generation.
- [exam-planning info email](exam-planning-info-email.md) — consolidated per-examer mail replacing constraints+prepared; examPlanningMailRecipients + sendEmailExamPlanningInfo.
- [MUC.DAI import linking](mucdai-import-linking.md) — import builds explicit mucdai_links (external/zpa/unresolved); candidates + manual set/remove; mucDaiImported state point.
