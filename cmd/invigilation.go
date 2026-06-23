package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	invigilationCmd = &cobra.Command{
		Use:   "invigilation [subcommand|roomname]",
		Short: "Add an invigilation",
		Long: `Add an invigilation.
reserve    [daynumber] [slotnumber] [invigilator ID] --- add reserve for slot (daynumber,slotnumber).
[roomname] [daynumber] [slotnumber] [invigilator ID] --- add invigilator for room in slot

With --pre-plan/-p the invigilation is pre-planned instead of added, i.e. it is
stored as a fixed assignment that the automatic invigilation planning respects.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			ctx := context.Background()
			if len(args) < 4 {
				fmt.Println("need day number, slot numbers, and the invigilators id")
				os.Exit(1)
			}

			day, err := strconv.Atoi(args[1])
			if err != nil {
				fmt.Printf("cannot use %s as day number", args[1])
				os.Exit(1)
			}
			slot, err := strconv.Atoi(args[2])
			if err != nil {
				fmt.Printf("cannot use %s as slot number", args[2])
				os.Exit(1)
			}

			room := args[0]
			if room != "reserve" {
				roomnames, err := plexams.PlannedRoomNamesInSlot(ctx, day, slot)
				if err != nil {
					fmt.Printf("error %s", err)
					os.Exit(1)
				}
				found := false
				for _, roomname := range roomnames {
					if room == roomname {
						found = true
					}
				}
				if !found {
					fmt.Printf("room %s not found in slot\n", room)
					os.Exit(1)
				}
			}

			invigilatorID, err := strconv.Atoi(args[3])
			if err != nil {
				// find invigilator by name
				invigilatorName := args[3]
				invigilatorID, err = plexams.GetTeacherIdByRegex(ctx, invigilatorName)
				if err != nil || invigilatorID == 0 {
					fmt.Printf("cannot find invigilator with regex %s", args[3])
					os.Exit(1)
				}
			}

			oldInvigilator, err := plexams.GetInvigilatorInSlot(ctx, room, day, slot)
			if err != nil {
				os.Exit(1)
			}

			newInvigilator, err := plexams.GetInvigilator(ctx, invigilatorID)
			if err != nil {
				os.Exit(1)
			}
			if newInvigilator == nil {
				fmt.Printf("found no invigilator with id %d", invigilatorID)
				os.Exit(1)
			}

			verb := "Add"
			if prePlanInvigilation {
				verb = "Pre-plan"
			}
			if oldInvigilator != nil {
				if !confirm(fmt.Sprintf("%s \"%s\" and override existing invigilator \"%s\" in slot (%d,%d) for \"%s\"?",
					verb, newInvigilator.Teacher.Shortname, oldInvigilator.Shortname, day, slot, room), 1) {
					os.Exit(0)
				}
			} else {
				if !confirm(fmt.Sprintf("%s \"%s\" for \"%s\" in slot (%d,%d)?",
					verb, newInvigilator.Teacher.Shortname, room, day, slot), 1) {
					os.Exit(0)
				}
			}

			if prePlanInvigilation {
				var roomName *string
				if room != "reserve" {
					roomName = &room
				}
				success, err := plexams.PreAddInvigilation(context.Background(), invigilatorID, day, slot, roomName)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
				if success {
					fmt.Printf("successfully pre-planned \"%s\" for \"%s\" in slot (%d,%d)\n",
						newInvigilator.Teacher.Shortname, room, day, slot)
				}
				return
			}

			err = plexams.AddInvigilation(context.Background(), room, day, slot, invigilatorID)
			if err != nil {
				log.Fatalf("got error: %v\n", err)
			}

			fmt.Println("recalculating todos...")
			_, err = plexams.PrepareInvigilationTodos(context.Background())
			if err != nil {
				log.Fatalf("got error: %v\n", err)
			}
			fmt.Println("...done")

		},
	}
	prePlanInvigilation bool

	invigilationProblemCmd = &cobra.Command{
		Use:   "problem",
		Short: "Show the invigilation planning problem (read-only)",
		Long:  `Build the invigilation planning snapshot from the database and print a summary.`,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			if err := plexams.ShowInvigilationProblem(context.Background()); err != nil {
				log.Fatalf("got error: %v\n", err)
			}
		},
	}

	generateDryRun     bool
	generateSeed       int64
	generateIterations int

	invigilationGenerateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate the invigilations automatically",
		Long: `Refresh self-invigilations and todos, then automatically assign invigilators
to all rooms and reserves with a simulated-annealing optimizer that respects the
hard constraints and balances the soft ones.

The result is written to invigilations_other, replacing its previous content
(self-invigilations and pre-planned invigilations are kept). To fix an
assignment across runs, move it to the pre-planning (invigilation -p ...).`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			plxms := initPlexamsConfig()
			opts := plxms.OptimizerOptionsFromConfig(generateSeed, generateIterations)
			if _, err := plxms.GenerateInvigilations(context.Background(), generateDryRun, opts, plexams.NewConsoleReporter()); err != nil {
				log.Fatalf("got error: %v\n", err)
			}
		},
	}

	invigilationRemovePrePlanCmd = &cobra.Command{
		Use:   "rm-pre-plan [roomname|reserve] [day] [slot]",
		Short: "Remove a pre-planned invigilation",
		Long:  `Remove a pre-planned invigilation for a room (or "reserve") in a slot.`,
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			plxms := initPlexamsConfig()
			day, err := strconv.Atoi(args[1])
			if err != nil {
				log.Fatalf("cannot use %s as day number", args[1])
			}
			slot, err := strconv.Atoi(args[2])
			if err != nil {
				log.Fatalf("cannot use %s as slot number", args[2])
			}
			var roomName *string
			if args[0] != "reserve" {
				roomName = &args[0]
			}
			if _, err := plxms.RemovePrePlannedInvigilation(context.Background(), day, slot, roomName); err != nil {
				log.Fatalf("got error: %v\n", err)
			}
			fmt.Printf("removed pre-planned invigilation for %s in slot (%d,%d)\n", args[0], day, slot)
		},
	}

	invigilationPermanentNonInvigilatorCmd = &cobra.Command{
		Use:   "permanent-non-invigilator [list | add <id> <reason...> | rm <id>]",
		Short: "Manage the global (cross-semester) permanent non-invigilators",
		Long: `Manage the teachers who never do invigilation duty again (e.g. retired).
This list is global (plexams DB) and carries over between semesters.

  permanent-non-invigilator list
  permanent-non-invigilator add <teacherID> <reason...>
  permanent-non-invigilator rm  <teacherID>`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plxms := initPlexamsConfig()
			ctx := context.Background()
			switch args[0] {
			case "list":
				list, err := plxms.PermanentNonInvigilators(ctx)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
				fmt.Printf("%d permanent non-invigilator(s):\n", len(list))
				for _, n := range list {
					fmt.Printf("  %d  %s  (%s)\n", n.TeacherID, n.Name, n.Reason)
				}
			case "add":
				if len(args) < 3 {
					log.Fatal("need teacherID and a reason")
				}
				id, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot use %s as teacher id", args[1])
				}
				reason := strings.Join(args[2:], " ")
				// name "" -> backend resolves it from the teacher record if possible
				if _, err := plxms.SetPermanentNonInvigilator(ctx, id, "", reason); err != nil {
					log.Fatalf("got error: %v\n", err)
				}
				fmt.Printf("added permanent non-invigilator %d (%s)\n", id, reason)
			case "rm":
				if len(args) < 2 {
					log.Fatal("need teacherID")
				}
				id, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot use %s as teacher id", args[1])
				}
				removed, err := plxms.RemovePermanentNonInvigilator(ctx, id)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
				fmt.Printf("removed: %v\n", removed)
			default:
				log.Fatalf("unknown subcommand %q (use list|add|rm)", args[0])
			}
		},
	}

	invigilationMigrateConstraintsCmd = &cobra.Command{
		Use:   "migrate-constraints",
		Short: "One-time migration of invigilatorConstraints from the config into the DB",
		Long: `Copy the invigilatorConstraints (isNotInvigilator, excludedDates, timeWindows)
from the semester config (YAML) into the DB collection invigilator_constraints,
so they can be managed via the GUI. After this you can remove the
invigilatorConstraints block from the semester YAML.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			plxms := initPlexamsConfig()
			count, err := plxms.MigrateInvigilatorConstraintsFromConfig(context.Background())
			if err != nil {
				log.Fatalf("got error: %v\n", err)
			}
			fmt.Printf("migrated %d invigilator constraint record(s) into the DB\n", count)
		},
	}
)

func init() {
	rootCmd.AddCommand(invigilationCmd)
	invigilationCmd.Flags().BoolVarP(&prePlanInvigilation, "pre-plan", "p", false, "pre-plan the invigilation instead of adding it")
	invigilationCmd.AddCommand(invigilationProblemCmd)

	invigilationGenerateCmd.Flags().BoolVar(&generateDryRun, "dry-run", false, "optimize and report only, do not write to the database")
	invigilationGenerateCmd.Flags().Int64Var(&generateSeed, "seed", 0, "random seed (0 = config/default)")
	invigilationGenerateCmd.Flags().IntVar(&generateIterations, "iterations", 0, "number of optimizer iterations (0 = config/default)")
	invigilationCmd.AddCommand(invigilationGenerateCmd)
	invigilationCmd.AddCommand(invigilationRemovePrePlanCmd)
	invigilationCmd.AddCommand(invigilationMigrateConstraintsCmd)
	invigilationCmd.AddCommand(invigilationPermanentNonInvigilatorCmd)
}
