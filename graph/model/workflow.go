package model

import "time"

type Step struct {
	Number   int       `json:"number" mapstructure:"number"`
	Name     string    `json:"name" mapstructure:"step"`
	Done     bool      `json:"done" mapstructure:"done"`
	Deadline time.Time `json:"deadline" mapstructure:"deadline"`
}
