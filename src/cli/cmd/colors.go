package cmd

import "fmt"

func colorGreen(s string) string { return fmt.Sprintf("\033[32m%s\033[0m", s) }
func colorRed(s string) string   { return fmt.Sprintf("\033[31m%s\033[0m", s) }
