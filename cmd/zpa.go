/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// zpaCmd represents the zpa command
var zpaCmd = &cobra.Command{
	Use:   "zpa",
	Short: "zpa to/from db",
	Long: `Fetch from zpa and post to zpa.
	teacher --- fetch teacher`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexams()
		switch args[0] {
		case "teacher":
			t := true
			teachers, err := plexams.GetTeachers(context.Background(), &t)
			if err != nil {
				log.Fatal().Err(err).Msg("cannot get teachers")
			}
			for i, teacher := range teachers {
				fmt.Printf("%3d. %s\n", i+1, teacher.Fullname)
			}
		case "exams":
			t := true
			exams, err := plexams.GetZPAExams(context.Background(), &t)
			if err != nil {
				log.Fatal().Err(err).Msg("cannot get teachers")
			}
			for _, exam := range exams {
				fmt.Printf("%3d. %s (%s)\n", exam.AnCode, exam.Module, exam.MainExamer)
			}
		default:
			fmt.Println("zpa called with unkown sub command")
		}
	},
}

func init() {
	rootCmd.AddCommand(zpaCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// zpaCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// zpaCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
