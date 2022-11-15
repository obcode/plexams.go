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
	exams-to-plan    --- ZPA exams which will be in the plan
	same-module-name --- print exam which should be in same slot`,
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

			case "same-module-name":
				if len(Outfile) == 0 {
					Outfile = "PrüfungenMitGleichenModulnamen.pdf"
				}
				fmt.Printf("generating %s\n", Outfile)
				err := plexams.SameModulNames(context.Background(), Outfile)
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
