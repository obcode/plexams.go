package plexams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PostStudentRegsToZPA(ctx context.Context, jsonOutputFile string, reporter Reporter) ([]*model.ZPAStudentReg, []*model.RegWithError, error) {
	reporter.Step("collecting student registrations")
	if err := p.SetZPA(); err != nil {
		return nil, nil, err
	}

	zpaStudentRegs := make([]*model.ZPAStudentReg, 0)

	for _, program := range p.zpa.fk07programs {
		studentRegs, err := p.dbClient.StudentRegsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("error while getting student regs")
			return nil, nil, err
		}
		// ZPA still uses the (2-letter) external code, while the internal program
		// may be degree-suffixed (e.g. DC-B/DC-M → DC). Translate back on the way out.
		zpaCode := p.zpaCodeForProgram(ctx, program)
		for _, studentReg := range studentRegs {
			zpaStudentReg := p.zpa.client.StudentReg2ZPAStudentReg(studentReg)
			zpaStudentReg.Program = zpaCode
			zpaStudentRegs = append(zpaStudentRegs, zpaStudentReg)
		}
	}

	// delete all studentRegs for semester and ancodes
	ancodesSet := set.NewSet[int]()
	for _, reg := range zpaStudentRegs {
		ancodesSet.Add(reg.AnCode)
	}

	ancodes := make([]*model.ZPAAncodes, 0, ancodesSet.Cardinality())
	for ancode := range ancodesSet.Iter() {
		ancodes = append(ancodes, &model.ZPAAncodes{
			Semester: p.semester,
			AnCode:   ancode,
		})
	}

	status, body, err := p.zpa.client.DeleteStudentRegsFromZPA(ancodes)
	if err != nil {
		log.Error().Err(err).Msg("error while trying to delete the student registrations from ZPA")
	}
	log.Debug().Str("status", status).Bytes("body", body).Msg("got answer from ZPA")

	regsWithErrors := make([]*model.RegWithError, 0)
	chunkSize := 77

	// Writing a local JSON copy is optional: the CLI passes a filename, the
	// GraphQL mutation passes "" to skip it.
	if jsonOutputFile != "" {
		zpaStudentRegsJson, err := json.MarshalIndent(zpaStudentRegs, "", " ")
		if err != nil {
			log.Error().Err(err).Msg("cannot marshal studentregs into json")
		}
		err = os.WriteFile(jsonOutputFile, zpaStudentRegsJson, 0644)
		if err != nil {
			log.Error().Err(err).Msg("cannot write studentregs to file")
		} else {
			fmt.Printf(" saved copy to %s\n", jsonOutputFile)
		}
	}

	reporter.Step(fmt.Sprintf("uploading %d student regs in chunks of %d to ZPA", len(zpaStudentRegs), chunkSize))

	for from := 0; from <= len(zpaStudentRegs); from += chunkSize {
		to := from + chunkSize
		if to > len(zpaStudentRegs) {
			to = len(zpaStudentRegs)
		}

		reporter.Step(fmt.Sprintf("uploading chunk of regs %d-%d", from, to-1))

		_, body, err := p.zpa.client.PostStudentRegsToZPA(zpaStudentRegs[from:to])
		if err != nil {
			log.Error().Err(err).Msg("error while posting student regs to zpa")
			return nil, nil, err
		}

		zpaStudentRegErrors := make([]*model.ZPAStudentRegError, 0)
		err = json.Unmarshal(body, &zpaStudentRegErrors)
		if err != nil {
			log.Error().Err(err).Interface("zpa-errors", zpaStudentRegErrors).Msg("error while unmarshalling errors from ZPA")
			fmt.Printf("%s\n", body)
			return nil, nil, err
		}

		for i, e := range zpaStudentRegErrors {
			if !noZPAStudRegError(e) {
				regsWithErrors = append(regsWithErrors, &model.RegWithError{
					Registration: zpaStudentRegs[from+i],
					Error:        e,
				})
			}
		}

	}

	err = p.dbClient.SetRegsWithErrors(ctx, regsWithErrors)
	if err != nil {
		return nil, nil, err
	}

	p.markCondition(ctx, condStudentRegsUploaded)
	reporter.StopProgress(fmt.Sprintf("uploaded %d regs, %d with errors", len(zpaStudentRegs), len(regsWithErrors)))

	p.logSync(ctx, &model.SyncLogEntry{
		Operation: "zpa-upload-studentregs",
		Label:     "Anmeldungen ins ZPA hochgeladen",
		Direction: "upload",
		System:    "ZPA",
		OK:        true,
		Summary:   fmt.Sprintf("%d Anmeldungen hochgeladen, %d mit Fehlern", len(zpaStudentRegs), len(regsWithErrors)),
	})

	return zpaStudentRegs, regsWithErrors, nil
}

func noZPAStudRegError(zpaStudentRegError *model.ZPAStudentRegError) bool {
	return len(zpaStudentRegError.Semester) == 0 &&
		len(zpaStudentRegError.AnCode) == 0 &&
		len(zpaStudentRegError.Exam) == 0 &&
		len(zpaStudentRegError.Mtknr) == 0 &&
		len(zpaStudentRegError.Program) == 0
}

func (p *Plexams) UploadPlan(ctx context.Context, withRooms, withInvigilators, upload bool, reporter Reporter) ([]*model.ZPAExamPlan, error) {
	what := "exams"
	if withInvigilators {
		what = "exams with rooms and invigilators"
	} else if withRooms {
		what = "exams with rooms"
	}
	reporter.Step("building plan: " + what)

	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
		return nil, err
	}

	exams := make([]*model.ZPAExamPlan, 0)
	for _, exam := range plannedExams {
		if exam.PlanEntry == nil {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		if exam.Ancode >= 1000 {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.DoNotPublish {
			reporter.Printf("do not publish: %d", exam.Ancode)
			continue
		}

		// absolute start time is the source of truth in the time-based model
		start, ok := planEntryStart(exam.PlanEntry)
		if !ok {
			continue
		}

		// FIXME: with rooms -> zpa
		var rooms []*model.ZPAExamPlanRoom
		reserveInvigilatorID := 0
		if withInvigilators {
			invigilator, err := p.invigilatorForRoomAtTime(ctx, "reserve", start)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.Ancode).Time("starttime", start).
					Msg("cannot get reserve invigilator for slot")
				return nil, err
			}
			reserveInvigilatorID = invigilator.ID
		}

		if withRooms {
			roomsForAncode, err := p.dbClient.PlannedRoomsForAncode(ctx, exam.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get rooms for ancode")
			} else {
				if len(roomsForAncode) > 0 {
					type roomNameWithDuration struct {
						name     string
						duration int
					}
					roomsMap := make(map[roomNameWithDuration][]*model.ZPAExamPlanRoom)

					for _, roomForAncode := range roomsForAncode {
						invigilatorID := 0
						if withInvigilators {
							invigilator, err := p.invigilatorForRoomAtTime(ctx, roomForAncode.RoomName, start)
							if err != nil {
								log.Error().Err(err).Int("ancode", exam.Ancode).Str("room", roomForAncode.RoomName).
									Msg("cannot get invigilator for room")
								return nil, err
							}
							invigilatorID = invigilator.ID
						}

						roomName := roomForAncode.RoomName
						if strings.HasPrefix(roomName, "ONLINE") {
							roomName = "ONLINE"
						}

						roomNameWithDuration := roomNameWithDuration{
							name:     roomName,
							duration: roomForAncode.Duration,
						}

						roomWithDuration, ok := roomsMap[roomNameWithDuration]
						if !ok {
							roomWithDuration = make([]*model.ZPAExamPlanRoom, 0, 1)
						}
						roomsMap[roomNameWithDuration] = append(roomWithDuration, &model.ZPAExamPlanRoom{
							RoomName:      roomName,
							InvigilatorID: invigilatorID,
							Duration:      roomForAncode.Duration,
							IsReserve:     roomForAncode.Reserve,
							StudentCount:  len(roomForAncode.StudentsInRoom),
							IsHandicap:    roomForAncode.Handicap,
						})
					}

					mergeRooms := func(roomWithSameDuration []*model.ZPAExamPlanRoom) []*model.ZPAExamPlanRoom {
						for i := 0; i < len(roomWithSameDuration); i++ {
							current := roomWithSameDuration[i]
							if current == nil {
								continue
							}
							for j := i + 1; j < len(roomWithSameDuration); j++ {
								other := roomWithSameDuration[j]
								if other == nil {
									continue
								}
								if current.IsHandicap && other.IsHandicap ||
									!current.IsHandicap && !other.IsHandicap {
									log.Debug().Int("ancode", exam.Ancode).Str("room", current.RoomName).Msg("found rooms to merge")
									roomWithSameDuration[i].StudentCount += other.StudentCount
									roomWithSameDuration[i].IsReserve = false
									roomWithSameDuration[j] = nil
								}
							}
						}

						rooms := make([]*model.ZPAExamPlanRoom, 0)
						for _, room := range roomWithSameDuration {
							if room != nil {
								rooms = append(rooms, room)
							}
						}
						return rooms
					}
					rooms = make([]*model.ZPAExamPlanRoom, 0, len(roomsForAncode))
					for _, roomWithSameDuration := range roomsMap {
						if len(roomWithSameDuration) == 0 {
							continue
						}
						if len(roomWithSameDuration) == 1 {
							rooms = append(rooms, roomWithSameDuration...)
						} else {
							log.Debug().Int("ancode", exam.Ancode).Interface("roomWithSameDuration", roomWithSameDuration).Msg("more than one room with same duration")
							rooms = append(rooms, mergeRooms(roomWithSameDuration)...)
						}
					}
				}
			}
		}

		exams = append(exams, &model.ZPAExamPlan{
			Semester:             p.semester,
			AnCode:               exam.ZpaExam.AnCode,
			Date:                 start.Format("02.01.2006"),
			Time:                 start.Format("15:04"),
			StudentCount:         exam.StudentRegsCount,
			ReserveInvigilatorID: reserveInvigilatorID,
			Rooms:                rooms,
		})
	}

	// publish - additionalExams (publish-only entries from the DB)
	additionalExams, err := p.additionalExamPlans(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get additional exams")
		return nil, err
	}
	for _, exam := range additionalExams {
		reporter.Printf("additional exam: %d. %s. %s", exam.AnCode, exam.Date, exam.Time)
		exams = append(exams, exam)
	}

	if upload {
		// post to ZPA
		reporter.Step(fmt.Sprintf("posting %d exams to ZPA", len(exams)))
		status, body, postErr := p.zpa.client.PostExams(exams)
		if postErr != nil {
			// a rejected upload (non-2xx) or a transport error: surface ZPA's
			// response as an error so it is not mistaken for success — both in the
			// CLI and, via the streaming reporter, in the GUI.
			log.Error().Err(postErr).Str("status", status).Msg("error while posting exams on zpa")
			if msg := strings.TrimSpace(string(body)); msg != "" {
				reporter.Warnf("ZPA response: %s", msg)
			}
			reporter.StopProgressFail(fmt.Sprintf("upload to ZPA failed (status %s)", status))
			return exams, fmt.Errorf("upload to ZPA failed: %w", postErr)
		}

		log.Info().Str("status", status).Msg("exams posted to zpa")
		reporter.StopProgress(fmt.Sprintf("uploaded %d exams to ZPA (status %s)", len(exams), status))
		reporter.Println(string(body))

		scope := "exams"
		if withInvigilators {
			scope = "exams-rooms-invigilators"
		} else if withRooms {
			scope = "exams-rooms"
		}
		p.logSync(ctx, &model.SyncLogEntry{
			Operation: "zpa-upload-plan-" + scope,
			Label:     "Prüfungsplan ins ZPA hochgeladen (" + what + ")",
			Direction: "upload",
			System:    "ZPA",
			OK:        true,
			Summary:   fmt.Sprintf("%d Prüfungen hochgeladen (%s)", len(exams), what),
		})
	} else {
		reporter.StopProgress(fmt.Sprintf("dry run: %d exams prepared, nothing uploaded", len(exams)))
	}

	return exams, err
}
