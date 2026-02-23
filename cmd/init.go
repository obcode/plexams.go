package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type initDefaults struct {
	From             string
	FromFK07         string
	Until            string
	Slots            []string
	GoDay0           string
	ForbiddenDays    []string
	Profs            string
	Lbas             string
	LbasLastSemester string
	Fs               string
	Sekr             string
	AdditionalExamer []string
}

var (
	initCmd = &cobra.Command{
		Use:   "init [semester]",
		Short: "Initialize new semester",
		Long:  `Initialize new semester. This interactively creates a semester config file named <semester>.yaml.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			semester := strings.TrimSpace(args[0])
			if !regexp.MustCompile(`^\d{4}-(SS|WS)$`).MatchString(semester) {
				return fmt.Errorf("invalid semester '%s' (expected format YYYY-SS or YYYY-WS)", semester)
			}

			reader := bufio.NewReader(os.Stdin)
			defaults := getInitDefaults()
			fmt.Printf("Creating semester config for %s\n\n", semester)
			fmt.Println("Press Enter to use defaults. For list fields with a default, enter '-' to clear the value.")
			fmt.Println()

			from, err := askDate(reader, "from (YYYY-MM-DD)", defaults.From)
			if err != nil {
				return err
			}
			fromFK07, err := askDate(reader, "fromFK07 (YYYY-MM-DD)", defaults.FromFK07)
			if err != nil {
				return err
			}
			until, err := askDate(reader, "until (YYYY-MM-DD)", defaults.Until)
			if err != nil {
				return err
			}

			slots, err := askList(reader, "slots (comma separated, e.g. 08:30,10:30,12:30)", defaults.Slots, true)
			if err != nil {
				return err
			}
			for _, slot := range slots {
				if _, err := time.Parse("15:04", slot); err != nil {
					return fmt.Errorf("invalid slot '%s' (expected HH:MM)", slot)
				}
			}

			goDay0, err := askDate(reader, "goDay0 (YYYY-MM-DD)", defaults.GoDay0)
			if err != nil {
				return err
			}

			forbiddenDays, err := askList(reader, "forbiddenDays (comma separated YYYY-MM-DD, empty allowed)", defaults.ForbiddenDays, false)
			if err != nil {
				return err
			}
			for _, day := range forbiddenDays {
				if _, err := time.Parse("2006-01-02", day); err != nil {
					return fmt.Errorf("invalid forbidden day '%s' (expected YYYY-MM-DD)", day)
				}
			}

			profs, err := askNonEmpty(reader, "emails.profs", defaults.Profs)
			if err != nil {
				return err
			}
			lbas, err := askNonEmpty(reader, "emails.lbas", defaults.Lbas)
			if err != nil {
				return err
			}
			lbasLastSemester, err := askNonEmpty(reader, "emails.lbaslastsemester", defaults.LbasLastSemester)
			if err != nil {
				return err
			}
			fs, err := askNonEmpty(reader, "emails.fs", defaults.Fs)
			if err != nil {
				return err
			}
			sekr, err := askNonEmpty(reader, "emails.sekr", defaults.Sekr)
			if err != nil {
				return err
			}

			additionalExamer, err := askList(reader, "additionalexamer (comma separated emails, empty allowed)", defaults.AdditionalExamer, false)
			if err != nil {
				return err
			}

			fromDate, err := time.Parse("2006-01-02", from)
			if err != nil {
				return fmt.Errorf("cannot parse from date: %w", err)
			}
			fromFK07Date, err := time.Parse("2006-01-02", fromFK07)
			if err != nil {
				return fmt.Errorf("cannot parse fromFK07 date: %w", err)
			}
			untilDate, err := time.Parse("2006-01-02", until)
			if err != nil {
				return fmt.Errorf("cannot parse until date: %w", err)
			}
			goDay0Date, err := time.Parse("2006-01-02", goDay0)
			if err != nil {
				return fmt.Errorf("cannot parse goDay0 date: %w", err)
			}

			forbiddenDayTimes := make([]time.Time, 0, len(forbiddenDays))
			for _, day := range forbiddenDays {
				parsed, parseErr := time.Parse("2006-01-02", day)
				if parseErr != nil {
					return fmt.Errorf("cannot parse forbidden day '%s': %w", day, parseErr)
				}
				forbiddenDayTimes = append(forbiddenDayTimes, parsed)
			}

			starttimes := make([]*model.Starttime, 0, len(slots))
			for i, slot := range slots {
				starttimes = append(starttimes, &model.Starttime{Number: i + 1, Start: slot})
			}

			cfg := &model.SemesterConfig{
				From:       fromDate,
				FromFk07:   fromFK07Date,
				Until:      untilDate,
				GoDay0:     goDay0Date,
				Starttimes: starttimes,
				Emails: &model.Emails{
					Profs:            profs,
					Lbas:             lbas,
					LbasLastSemester: lbasLastSemester,
					Fs:               fs,
					Sekr:             sekr,
					AdditionalExamer: additionalExamer,
				},
			}

			out, err := yaml.Marshal(semesterConfigYAMLDocument(cfg, forbiddenDayTimes))
			if err != nil {
				return fmt.Errorf("cannot marshal yaml: %w", err)
			}

			basePath := viper.GetString("semester-path")
			if basePath == "" {
				basePath = "."
			}
			basePath = os.ExpandEnv(basePath)

			if strings.HasPrefix(basePath, "~") {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("cannot resolve home dir: %w", err)
				}
				basePath = filepath.Join(home, strings.TrimPrefix(basePath, "~"))
			}

			if err := os.MkdirAll(basePath, 0o755); err != nil {
				return fmt.Errorf("cannot create semester path '%s': %w", basePath, err)
			}

			filename := filepath.Join(basePath, fmt.Sprintf("%s.yaml", semester))
			if _, err := os.Stat(filename); err == nil {
				overwrite, askErr := askYesNo(reader, fmt.Sprintf("file '%s' exists. Overwrite? [y/N]: ", filename))
				if askErr != nil {
					return askErr
				}
				if !overwrite {
					return errors.New("aborted")
				}
			}

			if err := os.WriteFile(filename, out, 0o644); err != nil {
				return fmt.Errorf("cannot write '%s': %w", filename, err)
			}

			fmt.Printf("\nWrote semester config: %s\n", filename)
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(initCmd)
}

func getInitDefaults() initDefaults {
	defaults := initDefaults{
		Slots: []string{"08:30", "10:30", "12:30", "14:30", "16:30"},
	}

	if !viper.GetTime("semesterConfig.from").IsZero() {
		defaults.From = viper.GetTime("semesterConfig.from").Format("2006-01-02")
	}
	if !viper.GetTime("semesterConfig.fromFK07").IsZero() {
		defaults.FromFK07 = viper.GetTime("semesterConfig.fromFK07").Format("2006-01-02")
	}
	if !viper.GetTime("semesterConfig.until").IsZero() {
		defaults.Until = viper.GetTime("semesterConfig.until").Format("2006-01-02")
	}

	if slots := viper.GetStringSlice("semesterConfig.slots"); len(slots) > 0 {
		defaults.Slots = slots
	}

	if !viper.GetTime("semesterConfig.goDay0").IsZero() {
		defaults.GoDay0 = viper.GetTime("semesterConfig.goDay0").Format("2006-01-02")
	}

	defaults.ForbiddenDays = getDateSliceFromViper("semesterConfig.forbiddenDays")

	defaults.Profs = viper.GetString("semesterConfig.emails.profs")
	defaults.Lbas = viper.GetString("semesterConfig.emails.lbas")
	defaults.LbasLastSemester = viper.GetString("semesterConfig.emails.lbaslastsemester")
	defaults.Fs = viper.GetString("semesterConfig.emails.fs")
	defaults.Sekr = viper.GetString("semesterConfig.emails.sekr")
	defaults.AdditionalExamer = viper.GetStringSlice("semesterConfig.additionalexamer")

	return defaults
}

func getDateSliceFromViper(key string) []string {
	raw := viper.Get(key)
	if raw == nil {
		return []string{}
	}

	result := make([]string, 0)
	switch values := raw.(type) {
	case []time.Time:
		for _, v := range values {
			result = append(result, v.Format("2006-01-02"))
		}
	case []interface{}:
		for _, value := range values {
			switch v := value.(type) {
			case time.Time:
				result = append(result, v.Format("2006-01-02"))
			case string:
				if _, err := time.Parse("2006-01-02", strings.TrimSpace(v)); err == nil {
					result = append(result, strings.TrimSpace(v))
				}
			}
		}
	case []string:
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				result = append(result, value)
			}
		}
	}

	return result
}

func askNonEmpty(reader *bufio.Reader, prompt, defaultValue string) (string, error) {
	for {
		line, err := readTrimmedLine(reader, withDefault(prompt, defaultValue))
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" && defaultValue != "" {
			return defaultValue, nil
		}
		if value != "" {
			return value, nil
		}
		fmt.Println("value is required")
	}
}

func askDate(reader *bufio.Reader, prompt, defaultValue string) (string, error) {
	for {
		value, err := askNonEmpty(reader, prompt, defaultValue)
		if err != nil {
			return "", err
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			fmt.Println("invalid date format, expected YYYY-MM-DD")
			continue
		}
		return value, nil
	}
}

func askList(reader *bufio.Reader, prompt string, defaultValues []string, required bool) ([]string, error) {
	for {
		line, err := readTrimmedLine(reader, withDefault(prompt, strings.Join(defaultValues, ",")))
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "-" {
			return []string{}, nil
		}
		if trimmed == "" {
			if len(defaultValues) > 0 {
				return append([]string(nil), defaultValues...), nil
			}
			if required {
				fmt.Println("at least one value is required")
				continue
			}
			return []string{}, nil
		}

		items := make([]string, 0)
		for _, part := range strings.Split(trimmed, ",") {
			value := strings.TrimSpace(part)
			if value != "" {
				items = append(items, value)
			}
		}

		if required && len(items) == 0 {
			fmt.Println("at least one value is required")
			continue
		}

		return items, nil
	}
}

func askYesNo(reader *bufio.Reader, prompt string) (bool, error) {
	line, err := readTrimmedLine(reader, prompt)
	if err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func readTrimmedLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func withDefault(prompt, defaultValue string) string {
	if defaultValue == "" {
		return prompt + ": "
	}
	return fmt.Sprintf("%s [%s]: ", prompt, defaultValue)
}

func semesterConfigYAMLDocument(cfg *model.SemesterConfig, forbiddenDays []time.Time) map[string]any {
	slots := make([]string, 0, len(cfg.Starttimes))
	for _, starttime := range cfg.Starttimes {
		slots = append(slots, starttime.Start)
	}

	forbidden := make([]string, 0, len(forbiddenDays))
	for _, day := range forbiddenDays {
		forbidden = append(forbidden, day.Format("2006-01-02"))
	}

	additionalExamer := []string{}
	if cfg.Emails != nil {
		additionalExamer = cfg.Emails.AdditionalExamer
	}

	emails := map[string]string{}
	if cfg.Emails != nil {
		emails["profs"] = cfg.Emails.Profs
		emails["lbas"] = cfg.Emails.Lbas
		emails["lbaslastsemester"] = cfg.Emails.LbasLastSemester
		emails["fs"] = cfg.Emails.Fs
		emails["sekr"] = cfg.Emails.Sekr
	}

	return map[string]any{
		"semesterConfig": map[string]any{
			"from":             cfg.From.Format("2006-01-02"),
			"fromFK07":         cfg.FromFk07.Format("2006-01-02"),
			"until":            cfg.Until.Format("2006-01-02"),
			"slots":            slots,
			"goDay0":           cfg.GoDay0.Format("2006-01-02"),
			"forbiddenDays":    forbidden,
			"emails":           emails,
			"additionalexamer": additionalExamer,
		},
	}
}
