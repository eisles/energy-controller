package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
)

const confirmWriteValue = "I_UNDERSTAND"

type envGetter func(string) string

type options struct {
	watts                  int
	expectedCurrentLimit   int
	reserveSoc             int
	expectedReserveSoc     int
	reserveSocSet          bool
	expectedReserveSet     bool
	disableTOU             bool
	disableEnergyModes     bool
	expectedTOUMode        bool
	expectedSelfPowered    bool
	expectedScheduled      bool
	expectedIntelligent    bool
	expectedTOUModeSet     bool
	expectedSelfSet        bool
	expectedScheduledSet   bool
	expectedIntelligentSet bool
	wattsSet               bool
	expectedLimitSet       bool
	execute                bool
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv envGetter, out io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if err := validateOptions(opts); err != nil {
		return err
	}
	if !opts.execute {
		fmt.Fprint(out, "dry-run: would")
		if opts.wattsSet {
			fmt.Fprintf(out, " set EcoFlow AC charge power to %dW after read-only current limit check (%dW)", opts.watts, opts.expectedCurrentLimit)
		}
		if opts.reserveSocSet {
			fmt.Fprintf(out, " set backup reserve to %d%% after current reserve check (%d%%)", opts.reserveSoc, opts.expectedReserveSoc)
		}
		if opts.disableEnergyModes {
			fmt.Fprintf(out, " disable energy strategy modes after current checks (tou=%t selfPowered=%t scheduled=%t intelligent=%t)", opts.expectedTOUMode, opts.expectedSelfPowered, opts.expectedScheduled, opts.expectedIntelligent)
		}
		fmt.Fprintln(out, "; no request sent")
		return nil
	}
	if err := validateExecuteEnvironment(getenv); err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	cfg := ecoflow.Config{
		AccessKey:  getenv("ECOFLOW_ACCESS_KEY"),
		SecretKey:  getenv("ECOFLOW_SECRET_KEY"),
		DeviceSN:   getenv("ECOFLOW_DEVICE_SN"),
		BaseURL:    envOrDefault(getenv, "ECOFLOW_BASE_URL", "https://api-e.ecoflow.com"),
		HTTPClient: httpClient,
	}
	reader := ecoflow.NewSignedClient(cfg)
	status, err := reader.GetBatteryStatus(ctx)
	if err != nil {
		return fmt.Errorf("read EcoFlow status before write: %w", err)
	}
	if opts.wattsSet && status.ACChargeLimitW != opts.expectedCurrentLimit {
		return fmt.Errorf("refuse EcoFlow write: current AC charge limit is %dW, expected %dW", status.ACChargeLimitW, opts.expectedCurrentLimit)
	}
	if opts.reserveSocSet {
		if status.BackupReserveSoc == nil {
			return fmt.Errorf("refuse EcoFlow write: current backup reserve SOC is unavailable")
		}
		if *status.BackupReserveSoc != opts.expectedReserveSoc {
			return fmt.Errorf("refuse EcoFlow write: current backup reserve SOC is %d%%, expected %d%%", *status.BackupReserveSoc, opts.expectedReserveSoc)
		}
	}
	if opts.disableEnergyModes {
		if err := validateEnergyModeStatus(status, opts); err != nil {
			return err
		}
	}

	writer := ecoflow.NewSignedWriteClient(cfg, ecoflow.WriteGuards{
		MockMode:           false,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControlEnabled: false,
		ManualOneShot:      true,
	})
	commandsSent := []string{}
	if opts.wattsSet {
		if err := writer.SetACChargePower(ctx, opts.watts); err != nil {
			return fmt.Errorf("set EcoFlow AC charge power: %w", err)
		}
		commandsSent = append(commandsSent, fmt.Sprintf("AC charge power %dW", opts.watts))
	}
	if opts.reserveSocSet {
		if err := writer.SetBackupReserveSoc(ctx, opts.reserveSoc); err != nil {
			if len(commandsSent) > 0 {
				return fmt.Errorf("set EcoFlow backup reserve SOC after prior command(s) were already sent (%s): %w; verify current EcoFlow settings before retrying", strings.Join(commandsSent, ", "), err)
			}
			return fmt.Errorf("set EcoFlow backup reserve SOC: %w", err)
		}
		commandsSent = append(commandsSent, fmt.Sprintf("backup reserve %d%%", opts.reserveSoc))
	}
	if opts.disableEnergyModes {
		if err := writer.SetTOUMode(ctx, false); err != nil {
			if len(commandsSent) > 0 {
				return fmt.Errorf("disable EcoFlow energy strategy modes after prior command(s) were already sent (%s): %w; verify current EcoFlow settings before retrying", strings.Join(commandsSent, ", "), err)
			}
			return fmt.Errorf("disable EcoFlow energy strategy modes: %w", err)
		}
		commandsSent = append(commandsSent, "energy strategy modes false")
	}
	fmt.Fprintf(out, "sent EcoFlow command(s): %s\n", strings.Join(commandsSent, ", "))
	return nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("ecoflow-write-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var opts options
	flags.IntVar(&opts.watts, "watts", 0, "target AC charge power watts")
	flags.IntVar(&opts.expectedCurrentLimit, "expected-current-limit", 0, "expected current AC charge limit watts")
	flags.IntVar(&opts.reserveSoc, "reserve-soc", 0, "optional target backup reserve SOC percent")
	flags.IntVar(&opts.expectedReserveSoc, "expected-current-reserve", 0, "expected current backup reserve SOC percent when --reserve-soc is set")
	flags.BoolVar(&opts.disableEnergyModes, "disable-energy-modes", false, "disable EcoFlow energy strategy modes")
	flags.BoolVar(&opts.disableTOU, "disable-tou", false, "deprecated alias for --disable-energy-modes")
	flags.BoolVar(&opts.expectedTOUMode, "expected-tou-mode", false, "expected current TOU mode when disabling energy modes")
	flags.BoolVar(&opts.expectedSelfPowered, "expected-self-powered-mode", false, "expected current self-powered mode when disabling energy modes")
	flags.BoolVar(&opts.expectedScheduled, "expected-scheduled-mode", false, "expected current scheduled mode when disabling energy modes")
	flags.BoolVar(&opts.expectedIntelligent, "expected-intelligent-schedule-mode", false, "expected current intelligent schedule mode when disabling energy modes")
	flags.BoolVar(&opts.execute, "execute", false, "send one real EcoFlow write command")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "watts":
			opts.wattsSet = true
		case "expected-current-limit":
			opts.expectedLimitSet = true
		case "reserve-soc":
			opts.reserveSocSet = true
		case "expected-current-reserve":
			opts.expectedReserveSet = true
		case "disable-tou":
			opts.disableEnergyModes = true
		case "expected-tou-mode":
			opts.expectedTOUModeSet = true
		case "expected-self-powered-mode":
			opts.expectedSelfSet = true
		case "expected-scheduled-mode":
			opts.expectedScheduledSet = true
		case "expected-intelligent-schedule-mode":
			opts.expectedIntelligentSet = true
		}
	})
	return opts, nil
}

func validateOptions(opts options) error {
	if !opts.wattsSet && !opts.reserveSocSet && !opts.disableEnergyModes {
		return fmt.Errorf("at least one command flag is required")
	}
	if opts.wattsSet {
		if !opts.expectedLimitSet {
			return fmt.Errorf("--expected-current-limit is required when --watts is set")
		}
		if opts.watts <= 0 {
			return fmt.Errorf("--watts must be positive")
		}
		if opts.expectedCurrentLimit <= 0 {
			return fmt.Errorf("--expected-current-limit must be positive")
		}
		if opts.watts == opts.expectedCurrentLimit {
			return fmt.Errorf("--watts must differ from --expected-current-limit")
		}
	} else if opts.expectedLimitSet {
		return fmt.Errorf("--watts is required when --expected-current-limit is set")
	}
	if opts.reserveSoc < 0 || opts.reserveSoc > 100 {
		return fmt.Errorf("--reserve-soc must be 0-100")
	}
	if opts.expectedReserveSoc < 0 || opts.expectedReserveSoc > 100 {
		return fmt.Errorf("--expected-current-reserve must be 0-100")
	}
	if opts.reserveSocSet {
		if !opts.expectedReserveSet {
			return fmt.Errorf("--expected-current-reserve is required when --reserve-soc is set")
		}
		if opts.reserveSoc == opts.expectedReserveSoc {
			return fmt.Errorf("--reserve-soc must differ from --expected-current-reserve")
		}
	} else if opts.expectedReserveSet {
		return fmt.Errorf("--reserve-soc is required when --expected-current-reserve is set")
	}
	if opts.disableEnergyModes {
		if !opts.expectedTOUModeSet || !opts.expectedSelfSet || !opts.expectedScheduledSet || !opts.expectedIntelligentSet {
			return fmt.Errorf("--expected-tou-mode, --expected-self-powered-mode, --expected-scheduled-mode, and --expected-intelligent-schedule-mode are required when disabling energy modes")
		}
		if !opts.expectedTOUMode && !opts.expectedSelfPowered && !opts.expectedScheduled && !opts.expectedIntelligent {
			return fmt.Errorf("at least one expected energy strategy mode must be true when disabling energy modes")
		}
	} else if opts.expectedTOUModeSet || opts.expectedSelfSet || opts.expectedScheduledSet || opts.expectedIntelligentSet {
		return fmt.Errorf("--disable-energy-modes is required when expected energy mode flags are set")
	}
	return nil
}

func validateEnergyModeStatus(status domain.BatteryStatus, opts options) error {
	if status.TOUModeEnabled == nil {
		return fmt.Errorf("refuse EcoFlow write: current TOU mode is unavailable")
	}
	if status.SelfPoweredEnabled == nil {
		return fmt.Errorf("refuse EcoFlow write: current self-powered mode is unavailable")
	}
	if status.ScheduledEnabled == nil {
		return fmt.Errorf("refuse EcoFlow write: current scheduled mode is unavailable")
	}
	if status.IntelligentEnabled == nil {
		return fmt.Errorf("refuse EcoFlow write: current intelligent schedule mode is unavailable")
	}
	if *status.TOUModeEnabled != opts.expectedTOUMode {
		return fmt.Errorf("refuse EcoFlow write: current TOU mode is %t, expected %t", *status.TOUModeEnabled, opts.expectedTOUMode)
	}
	if *status.SelfPoweredEnabled != opts.expectedSelfPowered {
		return fmt.Errorf("refuse EcoFlow write: current self-powered mode is %t, expected %t", *status.SelfPoweredEnabled, opts.expectedSelfPowered)
	}
	if *status.ScheduledEnabled != opts.expectedScheduled {
		return fmt.Errorf("refuse EcoFlow write: current scheduled mode is %t, expected %t", *status.ScheduledEnabled, opts.expectedScheduled)
	}
	if *status.IntelligentEnabled != opts.expectedIntelligent {
		return fmt.Errorf("refuse EcoFlow write: current intelligent schedule mode is %t, expected %t", *status.IntelligentEnabled, opts.expectedIntelligent)
	}
	return nil
}

func validateExecuteEnvironment(getenv envGetter) error {
	checks := []struct {
		key  string
		want string
	}{
		{key: "MOCK_MODE", want: "false"},
		{key: "SIMULATION_MODE", want: "false"},
		{key: "ENABLE_REAL_CONTROL", want: "true"},
		{key: "AUTO_CONTROL_ENABLED", want: "false"},
		{key: "CONFIRM_ECOFLOW_WRITE", want: confirmWriteValue},
	}
	for _, check := range checks {
		if getenv(check.key) != check.want {
			return fmt.Errorf("%s must be %s", check.key, check.want)
		}
	}
	for _, key := range []string{"ECOFLOW_ACCESS_KEY", "ECOFLOW_SECRET_KEY", "ECOFLOW_DEVICE_SN"} {
		if getenv(key) == "" {
			return fmt.Errorf("%s must be set", key)
		}
	}
	if _, err := strconv.Atoi(getenv("ECOFLOW_DEVICE_SN")); err == nil {
		return fmt.Errorf("ECOFLOW_DEVICE_SN looks numeric-only; verify the serial before writing")
	}
	return nil
}

func envOrDefault(getenv envGetter, key string, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}
