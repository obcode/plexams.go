package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	pdfCmd = &cobra.Command{
		Use:   "pdf",
		Short: "pdf [subcommand]",
		Long: `Generate various PDFs.
	exams-to-plan --- ZPA exams which will be in the plan`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "exams-to-plan":
				if len(Outfile) == 0 {
					Outfile = "PrüfungenImPrüfungszeitraum.pdf"
				}
				fmt.Printf("generating %s\n", Outfile)
				err := plexams.GenerateExamsToPlanPDF(context.Background(), Outfile)
				if err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("pdf called with unkown sub command")
			}
		},
	}
	Outfile string
)

func init() {
	rootCmd.AddCommand(pdfCmd)
	pdfCmd.Flags().StringVarP(&Outfile, "out", "o", "", "output (pdf) file")
}
