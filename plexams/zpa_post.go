package plexams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) PostStudentRegsToZPA(ctx context.Context) (int, []*model.RegWithError, error) {
	if err := p.SetZPA(); err != nil {
		return 0, nil, err
	}

	zpaStudentRegs := make([]*model.ZPAStudentReg, 0)

	for _, program := range p.zpa.fk07programs {
		studentRegs, err := p.dbClient.StudentRegsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("error while getting student regs")
			return 0, nil, err
		}
		for _, studentReg := range studentRegs {
			zpaStudentRegs = append(zpaStudentRegs, p.zpa.client.StudentReg2ZPAStudentReg(studentReg))
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
	chunkSize := 500

	log.Info().Int("count", len(zpaStudentRegs)).Int("chunk size", chunkSize).Msg("Uploading a lot of regs in chunks.")

	for from := 0; from <= len(zpaStudentRegs); from = from + chunkSize {
		to := from + chunkSize
		if to > len(zpaStudentRegs) {
			to = len(zpaStudentRegs)
		}

		log.Info().Int("from", from).Int("to", to).Msg("Uploading chunk of regs.")

		_, body, err := p.zpa.client.PostStudentRegsToZPA(zpaStudentRegs[from:to])
		if err != nil {
			log.Error().Err(err).Msg("error while posting student regs to zpa")
			return 0, nil, err
		}

		zpaStudentRegErrors := make([]*model.ZPAStudentRegError, 0)
		err = json.Unmarshal(body, &zpaStudentRegErrors)
		if err != nil {
			log.Error().Err(err).Interface("zpa-errors", zpaStudentRegErrors).Msg("error while unmarshalling errors from ZPA")
			return 0, nil, err
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
		return 0, nil, err
	}

	return len(zpaStudentRegs) - len(regsWithErrors), regsWithErrors, nil
}

func noZPAStudRegError(zpaStudentRegError *model.ZPAStudentRegError) bool {
	return len(zpaStudentRegError.Semester) == 0 &&
		len(zpaStudentRegError.AnCode) == 0 &&
		len(zpaStudentRegError.Exam) == 0 &&
		len(zpaStudentRegError.Mtknr) == 0 &&
		len(zpaStudentRegError.Program) == 0
}

func (p *Plexams) UploadPlan(ctx context.Context, withRooms, withInvigilators, upload bool) ([]*model.ZPAExamPlan, error) {
	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
		return nil, err
	}

	doNotPublish := viper.GetIntSlice("donotpublish")
	for _, ancodeNotToPublish := range doNotPublish {
		fmt.Printf("do not publish: %d\n", ancodeNotToPublish)
	}

	exams := make([]*model.ZPAExamPlan, 0)
	for _, exam := range plannedExams {
		if exam.PlanEntry == nil {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		for _, ancodeNotToPublish := range doNotPublish {
			if exam.ZpaExam.AnCode == ancodeNotToPublish {
				continue
			}
		}
		// slot, err := p.SlotForAncode(ctx, exam.Exam.Ancode)
		// if err != nil {
		// 	log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("cannot get slot for ancode")
		// }
		// timeForAncode := p.getSlotTime(slot.DayNumber, slot.SlotNumber)
		// studentCount := 0
		// for _, studentRegs := range exam.Exam.StudentRegs {
		// 	studentCount += len(studentRegs.StudentRegs)
		// }

		// FIXME: with rooms -> zpa
		var rooms []*model.ZPAExamPlanRoom
		reserveInvigilatorID := 0
		// if withInvigilators {
		// 	invigilator, err := p.GetInvigilatorInSlot(ctx, "reserve", slot.DayNumber, slot.SlotNumber)
		// 	if err != nil {
		// 		log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).
		// 			Msg("cannot get reserve invigilator for slot")
		// 		return nil, err
		// 	}
		// 	reserveInvigilatorID = invigilator.ID
		// }

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
						if roomForAncode.RoomName == "No Room" {
							continue
						}

						// 				invigilatorID := 0
						// 				if withInvigilators {
						// 					invigilator, err := p.GetInvigilatorInSlot(ctx, roomForAncode.RoomName, slot.DayNumber, slot.SlotNumber)
						// 					if err != nil {
						// 						log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Str("room", roomForAncode.RoomName).
						// 							Msg("cannot get invigilator for room")
						// 						return nil, err
						// 					}
						// 					invigilatorID = invigilator.ID
						// 				}

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
							InvigilatorID: 0, // invigilatorID,
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

		starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)

		exams = append(exams, &model.ZPAExamPlan{
			Semester:             p.semester,
			AnCode:               exam.ZpaExam.AnCode,
			Date:                 starttime.Local().Format("02.01.2006"),
			Time:                 starttime.Local().Format("15:04"),
			StudentCount:         exam.StudentRegsCount,
			ReserveInvigilatorID: reserveInvigilatorID,
			Rooms:                rooms,
		})
	}

	if upload {
		// post to ZPA
		status, body, err := p.zpa.client.PostExams(exams)
		if err != nil {
			log.Error().Err(err).Msg("error while posting exams on zpa")
		}

		log.Info().Str("status", status).Msg("exams posted to zpa")
		fmt.Println(string(body))
	} else {
		log.Info().Msg("not uploaded to zpa")
	}

	return exams, err
}
