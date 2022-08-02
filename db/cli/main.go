package main

import (
	"fmt"

	"github.com/obcode/plexams.go/db"
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

	client, _ := db.NewClient(viper.GetString("db.uri"))

	semester, _ := client.GetSemester()

	for _, s := range semester {
		fmt.Println(s)
	}

}
