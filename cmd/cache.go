package cmd

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

var (
	cacheCmd = &cobra.Command{
		Use:   "cache",
		Short: "cache [subcommand]",
		Long: `cache collections.
	exam <ancode>
	exams
	rm-exams
`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {

			case "exam":
				if len(args) < 2 {
					log.Fatal("need ancode")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					fmt.Printf("cannot use %s as ancode", args[1])
					os.Exit(1)
				}

				err = plexams.CacheExam(ancode)
				if err != nil {
					os.Exit(1)
				}

			case "exams":
				err := plexams.CacheExams()
				if err != nil {
					os.Exit(1)
				}

			case "rm-exams":
				err := plexams.RmCacheExams()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Println(aurora.Green("successfully removed the cached exams"))

			default:
				fmt.Println("cache called with unknown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(cacheCmd)
}
