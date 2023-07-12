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

	dbName := viper.GetString("db.database")
	var databaseName *string
	if dbName == "" {
		databaseName = nil
	} else {
		databaseName = &dbName
	}

	client, _ := db.NewDB(viper.GetString("db.uri"), viper.GetString("semester"), databaseName)

	semester, _ := client.AllSemesterNames()

	for _, s := range semester {
		fmt.Println(s)
	}

}
