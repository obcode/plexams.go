[![GoDoc](https://godoc.org/github.com/obcode/plexams.go?status.svg)](https://godoc.org/github.com/obcode/plexams.go)
[![Go Report Card](https://goreportcard.com/badge/github.com/obcode/plexams.go)](https://goreportcard.com/report/github.com/obcode/plexams.go)

# plexams.go

- Import aus ZPA -- daher nicht verändern!
  - teachers
  - zpaexams

## Datenmodell

```mermaid
erDiagram
    teachers
    zpaexams
    zpaexamsToPlan {
        int ancode
        bool toPlan
    }
    zpaexams ||--|| zpaexamsToPlan: planOrNot
    constraints |o--|| zpaexams: hasConstraint
    zpaexams |o--o{ exams_primuss: connectedExam
    externalExams {
        int ancode
        string program
    }
```

    additionalExam |o--o{ exams_primuss: connectedAdditionalExam

    conflicts_XY
    count_XY
    exams_XY
    studentregs_XY

## Ablauf
