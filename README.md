[![GoDoc](https://godoc.org/github.com/obcode/plexams.go?status.svg)](https://godoc.org/github.com/obcode/plexams.go)
[![Go Report Card](https://goreportcard.com/badge/github.com/obcode/plexams.go)](https://goreportcard.com/report/github.com/obcode/plexams.go)

# plexams.go

- Import aus ZPA -- daher nicht verändern!
  - teachers
  - zpaexams

```mermaid
erDiagram
    teachers
    zpaexams
    zpaexamsToPlan {
        int ancode
        bool toPlan
    }
    zpaexams ||--|| zpaexamsToPlan: references
```
