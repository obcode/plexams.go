# Examiner portal API

The examiner portal ("Portal für Prüfende") is a small GraphQL API of its own. The portal
pages in the GUI use exactly this API and nothing else, so everything they can do, a
script can do too -- for example Claude, with a personal access token.

The schema is [`graph/portal/portal.graphqls`](../graph/portal/portal.graphqls). The
endpoint answers introspection, so tools can explore it themselves.

## Endpoints

| Path | Authentication | For |
| --- | --- | --- |
| `/query/portal` | the SSO session of the browser | the portal pages |
| `/query/token` | `Authorization: Bearer <token>` | scripts and tools |

Both serve the same schema. Only `POST` with a JSON body is accepted. The server is only
reachable from the university network or through VPN; the portal's help page shows the
address of the installation.

## Personal access tokens

Create a token in the portal under *Zugang*. You choose a description, a lifetime (90 days
by default, at most 365) and a scope:

- `READ` (default) -- read your data.
- `WRITE` -- also enter and change data.

The token is shown exactly once. It looks like `plexams_<id>_<secret>`; the `<id>` part is
public and shown in the token list. A token

- only works at `/query/token`, never in the planning interface, even if you are a planner;
- only reaches your own data -- the server decides from your login, never from arguments;
- cannot create, list or revoke tokens (that needs a signed-in session);
- stops working at once when revoked, expired or when your portal access is withdrawn.

Store it like a password, e.g. in an environment variable:

```bash
export PLEXAMS_TOKEN=plexams_...
curl -s https://<server>/query/token \
  -H "Authorization: Bearer $PLEXAMS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ session { email teachers { fullname } } inquiries { semester kind open deadline affected } }"}'
```

## Errors

Errors carry a stable code in `extensions.code`; the message is English and may change.

| Code | Meaning |
| --- | --- |
| `FORBIDDEN` | the login has no portal access, or may not act for that person |
| `NOT_FOUND` | no such entry -- also for entries of other people (same answer on purpose) |
| `NOT_AFFECTED` | the inquiry does not concern you (e.g. no exam in the semester) |
| `INQUIRY_CLOSED` | the inquiry does not take answers any more |
| `SEMESTER_READ_ONLY` | the planners have write-protected the semester |
| `REVISION_CONFLICT` | the entry changed since you read it -- read it again |
| `INVALID_INPUT` | the input is invalid; the message says why |
| `READ_ONLY_TOKEN` | a `READ` token tried to change something |
| `INTERACTIVE_ONLY` | token management needs a signed-in session |

HTTP status `401` means the token is invalid, expired or revoked; `429` means too many
requests -- slow down.

## Inquiries

The planners ask; you answer. An inquiry belongs to a semester (`"2027-SS"`) and a kind.
Every operation names the semester explicitly.

```graphql
{ inquiries { semester kind open deadline intro affected nothingToReport } }
```

`affected: false` means the inquiry does not concern you; there is nothing to enter.

### EXaHM/SEB exams (`PREPLAN`)

Report every written exam with EXaHM or SEB you plan for the semester. FK07 professors
may also report for lecturers (`onBehalfOf`, see `preplanTeachers`).

```graphql
# whom you may report for, and the study programs an entry may name
{ preplanTeachers(semester: "2027-SS") { id fullname } studyPrograms { shortname name } }

# your entries (also those you made for somebody else)
{ preplanSubmissions(semester: "2027-SS") {
    id revision withdrawn examKind module programs expectedStudents durationMin remark
    review reviewNote } }
```

```graphql
mutation {
  submitPreplan(semester: "2027-SS", input: {
    examKind: SEB, module: "Webtechnologien", programs: ["IF", "IB"],
    expectedStudents: 60, durationMin: 90, remark: "zwei Gruppen"
  }) { id revision }
}
```

Change an entry by passing its `id` and the `revision` you last read as
`expectedRevision`. If somebody changed it in between you get `REVISION_CONFLICT` and
nothing is written. Saving an unchanged entry is not a change.

```graphql
mutation {
  submitPreplan(semester: "2027-SS", id: 17, expectedRevision: 1, input: {
    examKind: SEB, module: "Webtechnologien", programs: ["IF"], expectedStudents: 45
  }) { revision }
}
mutation { withdrawSubmission(semester: "2027-SS", id: 17, expectedRevision: 2) { withdrawn } }
mutation { setNothingToReport(semester: "2027-SS", kind: PREPLAN, value: true) { nothingToReport } }
```

What happens with an entry:

- It goes into the planners' pre-planning immediately. The planners are notified.
- A change updates only the fields you changed; corrections by the planners to other
  fields stay.
- A withdrawal removes the entry from the pre-planning as long as the planners have not
  worked on it yet; otherwise they see the withdrawal and take care of it.
- `review` shows whether the planners have looked at the current revision; a rejection
  comes with `reviewNote`.

## A hint for Claude

> Check my EXaHM/SEB entries in the Plexams examiner portal for the semester 2027-SS.
> The API is GraphQL at `https://<server>/query/token`; the token is in `PLEXAMS_TOKEN`
> (send it as `Authorization: Bearer`). Explore the schema with introspection first and
> do not change anything unless I ask you to.
