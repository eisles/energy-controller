package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflow"
)

const confirmWriteValue = "I_UNDERSTAND"

type envGetter func(string) string

type options struct {
	watts                int
	expectedCurrentLimit int
	reserveSoc           int
	expectedReserveSoc   int
	reserveSocSet        bool
	expectedReserveSet   bool
	execute              bool
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
		fmt.Fprintf(out, "dry-run: would set EcoFlow AC charge power to %dW after read-only current limit check (%dW)", opts.watts, opts.expectedCurrentLimit)
		if opts.reserveSocSet {
			fmt.Fprintf(out, " and backup reserve to %d%% after current reserve check (%d%%)", opts.reserveSoc, opts.expectedReserveSoc)
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
	if status.ACChargeLimitW != opts.expectedCurrentLimit {
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

	writer := ecoflow.NewSignedWriteClient(cfg, ecoflow.WriteGuards{
		MockMode:           false,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControlEnabled: false,
		ManualOneShot:      true,
	})
	if err := writer.SetACChargePower(ctx, opts.watts); err != nil {
		return fmt.Errorf("set EcoFlow AC charge power: %w", err)
	}
	if opts.reserveSocSet {
		if err := writer.SetBackupReserveSoc(ctx, opts.reserveSoc); err != nil {
			return fmt.Errorf("set EcoFlow backup reserve SOC after AC charge power command was already sent (%dW): %w; verify current AC charge limit before retrying", opts.watts, err)
		}
		fmt.Fprintf(out, "sent one EcoFlow AC charge power command: %dW and backup reserve command: %d%% (previous limit/reserve confirmed: %dW/%d%%)\n", opts.watts, opts.reserveSoc, opts.expectedCurrentLimit, opts.expectedReserveSoc)
		return nil
	}
	fmt.Fprintf(out, "sent one EcoFlow AC charge power command: %dW (previous limit confirmed: %dW)\n", opts.watts, opts.expectedCurrentLimit)
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
	flags.BoolVar(&opts.execute, "execute", false, "send one real EcoFlow write command")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "reserve-soc":
			opts.reserveSocSet = true
		case "expected-current-reserve":
			opts.expectedReserveSet = true
		}
	})
	return opts, nil
}

func validateOptions(opts options) error {
	if opts.watts <= 0 {
		return fmt.Errorf("--watts must be positive")
	}
	if opts.expectedCurrentLimit <= 0 {
		return fmt.Errorf("--expected-current-limit must be positive")
	}
	if opts.watts == opts.expectedCurrentLimit {
		return fmt.Errorf("--watts must differ from --expected-current-limit")
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
