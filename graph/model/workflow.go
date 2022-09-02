package model

type Step struct {
	Number int    `json:"number" mapstructure:"number"`
	Name   string `json:"name" mapstructure:"step"`
	Done   bool   `json:"done" mapstructure:"done"`
}
