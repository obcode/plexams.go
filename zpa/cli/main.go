package main

import (
	"fmt"

	"github.com/obcode/plexams.go/zpa"
	"github.com/spf13/viper"
)

func main() {
	viper.SetConfigName("plexams")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	zpa, err := zpa.NewZPA(
		viper.GetString("zpa.baseurl"),
		viper.GetString("zpa.username"),
		viper.GetString("zpa.password"),
		viper.GetString("semester"),
	)
	if err != nil {
		fmt.Printf("cannot get new zpa: %v", err)
	}

	for _, teacher := range zpa.GetTeachers() {
		fmt.Printf("%+v\n", teacher)
	}

	for _, exam := range zpa.GetExams() {
		fmt.Printf("%+v\n", exam)
	}

	for _, supReq := range zpa.GetSupervisorRequirements() {
		fmt.Printf("%+v\n", supReq)
	}

	fmt.Printf("%d teachers, %d exams, %d supervisor requirements\n",
		len(zpa.GetTeachers()), len(zpa.GetExams()), len(zpa.GetSupervisorRequirements()))
}
