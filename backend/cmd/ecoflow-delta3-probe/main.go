package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflowdelta3"
)

type envGetter func(string) string

type options struct {
	sn                   string
	deviceType           string
	privateAPIHost       string
	timeout              time.Duration
	fixturePath          string
	setACChargeW         int
	backupReserveSoc     int
	setACChargeWSet      bool
	backupReserveSocSet  bool
	execute              bool
	allowPrivateAPIWrite bool
}

type output struct {
	Mode   string                 `json:"mode"`
	Status ecoflowdelta3.Status   `json:"status"`
	Write  map[string]interface{} `json:"write"`
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
	cfg := buildConfig(opts, getenv)
	client := ecoflowdelta3.NewClient(cfg)

	if opts.fixturePath != "" {
		raw, err := os.ReadFile(opts.fixturePath)
		if err != nil {
			return err
		}
		status, err := ecoflowdelta3.DecodeSnapshot(cfg.DeviceType, cfg.DeviceSN, raw)
		if err != nil {
			return err
		}
		return writeJSON(out, output{Mode: "offline-fixture", Status: status, Write: map[string]interface{}{"wouldSend": false, "sent": false, "reason": "offline fixture decode"}})
	}

	if opts.setACChargeWSet || opts.backupReserveSocSet {
		return runWriteCandidate(ctx, opts, getenv, client, out)
	}

	status, err := client.Probe(ctx)
	if err != nil {
		return err
	}
	return writeJSON(out, output{Mode: "read-only", Status: status, Write: map[string]interface{}{"wouldSend": false, "sent": false, "reason": "read-only probe"}})
}

func runWriteCandidate(ctx context.Context, opts options, getenv envGetter, client *ecoflowdelta3.Client, out io.Writer) error {
	if opts.setACChargeWSet && opts.backupReserveSocSet {
		return fmt.Errorf("set only one command per DELTA_3 probe run")
	}
	command := "set_ac_charge_power"
	if opts.backupReserveSocSet {
		command = "set_backup_reserve_soc"
	}
	guards := ecoflowdelta3.WriteGuards{
		MockMode:             envBool(getenv, "MOCK_MODE", true),
		SimulationMode:       envBool(getenv, "SIMULATION_MODE", true),
		EnableRealControl:    envBool(getenv, "ENABLE_REAL_CONTROL", false),
		AutoControlEnabled:   envBool(getenv, "AUTO_CONTROL_ENABLED", false),
		ConfirmEcoFlowWrite:  getenv("CONFIRM_ECOFLOW_WRITE"),
		Execute:              opts.execute,
		AllowPrivateAPIWrite: opts.allowPrivateAPIWrite,
		Command:              command,
	}
	if !opts.execute {
		var payload ecoflowdelta3.CommandPayload
		var err error
		if opts.setACChargeWSet {
			payload, err = client.BuildDryRunACChargePower(opts.setACChargeW)
		} else {
			payload, err = client.BuildDryRunBackupReserve(opts.backupReserveSoc)
		}
		if err != nil {
			return err
		}
		return writeJSON(out, output{
			Mode:   "dry-run",
			Status: ecoflowdelta3.Status{},
			Write: map[string]interface{}{
				"wouldSend": true,
				"sent":      false,
				"reason":    "dry-run; no MQTT set publish",
				"command":   payload.Command,
				"topic":     payload.Topic,
				"hex":       payload.Hex,
			},
		})
	}

	var status ecoflowdelta3.Status
	var err error
	if opts.setACChargeWSet {
		status, err = client.ExecuteACChargePower(ctx, opts.setACChargeW, guards)
	} else {
		status, err = client.ExecuteBackupReserve(ctx, opts.backupReserveSoc, guards)
	}
	if err != nil {
		return err
	}
	return writeJSON(out, output{
		Mode:   "execute",
		Status: status,
		Write: map[string]interface{}{
			"wouldSend": true,
			"sent":      true,
			"command":   command,
		},
	})
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("ecoflow-delta3-probe", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var opts options
	flags.StringVar(&opts.sn, "sn", "", "DELTA_3 device serial number")
	flags.StringVar(&opts.deviceType, "device-type", "", "DELTA_3 device type")
	flags.StringVar(&opts.privateAPIHost, "private-api-host", "", "EcoFlow private API host")
	flags.DurationVar(&opts.timeout, "timeout", 20*time.Second, "MQTT request timeout")
	flags.StringVar(&opts.fixturePath, "offline-fixture", "", "decode a raw/base64 protobuf payload file without network access")
	flags.IntVar(&opts.setACChargeW, "set-ac-charge-w", 0, "optional AC charge power command for dry-run or one-shot execute")
	flags.IntVar(&opts.backupReserveSoc, "backup-reserve-soc", 0, "optional backup reserve SOC command for dry-run or one-shot execute")
	flags.BoolVar(&opts.execute, "execute", false, "send one real private MQTT write command")
	flags.BoolVar(&opts.allowPrivateAPIWrite, "allow-private-api-write", false, "required together with --execute for private MQTT write")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "set-ac-charge-w":
			opts.setACChargeWSet = true
		case "backup-reserve-soc":
			opts.backupReserveSocSet = true
		}
	})
	return opts, nil
}

func buildConfig(opts options, getenv envGetter) ecoflowdelta3.Config {
	return ecoflowdelta3.Config{
		PrivateAPIHost: firstNonEmpty(opts.privateAPIHost, getenv("ECOFLOW_PRIVATE_API_HOST"), ecoflowdelta3.DefaultPrivateAPIHost),
		Email:          getenv("ECOFLOW_PRIVATE_EMAIL"),
		Password:       getenv("ECOFLOW_PRIVATE_PASSWORD"),
		DeviceSN:       firstNonEmpty(opts.sn, getenv("ECOFLOW_DELTA3_DEVICE_SN")),
		DeviceType:     firstNonEmpty(opts.deviceType, getenv("ECOFLOW_DELTA3_DEVICE_TYPE"), "DELTA_3"),
		MQTTClientID:   getenv("ECOFLOW_DELTA3_MQTT_CLIENT_ID"),
		Timeout:        opts.timeout,
	}
}

func envBool(getenv envGetter, key string, def bool) bool {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return def
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
