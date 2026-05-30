package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
)

type envGetter func(string) string

type options struct {
	sn                      string
	deviceType              string
	privateAPIHost          string
	timeout                 time.Duration
	timeoutSet              bool
	fixturePath             string
	setACChargeW            int
	backupReserveSoc        int
	gridBypassDisabled      bool
	minDischargeSoc         int
	maxChargeSoc            int
	energyBackupEnabled     bool
	energyBackupStartSoc    int
	setACChargeWSet         bool
	backupReserveSocSet     bool
	gridBypassDisabledSet   bool
	minDischargeSocSet      bool
	maxChargeSocSet         bool
	energyBackupEnabledSet  bool
	energyBackupStartSocSet bool
	rawOutputDir            string
	inspectFields           bool
	execute                 bool
	allowPrivateAPIWrite    bool
	allowAutoControlOverlap bool
}

type output struct {
	Mode   string                         `json:"mode"`
	Status ecoflowprivate.Status          `json:"status"`
	Write  map[string]interface{}         `json:"write"`
	Raw    []rawCaptureFile               `json:"raw,omitempty"`
	Fields []ecoflowprivate.SnapshotField `json:"fields,omitempty"`
}

type rawCaptureFile struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	File  string `json:"file"`
	Bytes int    `json:"bytes"`
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
	client := ecoflowprivate.NewClient(cfg)

	if opts.fixturePath != "" {
		raw, err := os.ReadFile(opts.fixturePath)
		if err != nil {
			return err
		}
		status, err := ecoflowprivate.DecodeSnapshot(cfg.DeviceType, cfg.DeviceSN, raw)
		if err != nil {
			return err
		}
		fields, err := inspectRawFields(raw, opts.inspectFields)
		if err != nil {
			return err
		}
		return writeJSON(out, output{Mode: "offline-fixture", Status: status, Write: map[string]interface{}{"wouldSend": false, "sent": false, "reason": "offline fixture decode"}, Fields: fields})
	}

	if opts.hasWriteCandidate() {
		return runWriteCandidate(ctx, opts, getenv, client, out)
	}

	if opts.rawOutputDir != "" || opts.inspectFields {
		status, replies, err := client.ProbeRaw(ctx)
		if err != nil {
			return err
		}
		var rawFiles []rawCaptureFile
		if opts.rawOutputDir != "" {
			rawFiles, err = saveRawMessages(opts.rawOutputDir, replies, time.Now())
			if err != nil {
				return err
			}
		}
		fields, err := inspectReplies(replies, opts.inspectFields)
		if err != nil {
			return err
		}
		reason := "read-only probe"
		if opts.rawOutputDir != "" {
			reason = "read-only probe; raw replies saved"
		}
		return writeJSON(out, output{
			Mode:   "read-only",
			Status: status,
			Write:  map[string]interface{}{"wouldSend": false, "sent": false, "reason": reason},
			Raw:    rawFiles,
			Fields: fields,
		})
	}

	status, err := client.Probe(ctx)
	if err != nil {
		return err
	}
	return writeJSON(out, output{Mode: "read-only", Status: status, Write: map[string]interface{}{"wouldSend": false, "sent": false, "reason": "read-only probe"}})
}

func runWriteCandidate(ctx context.Context, opts options, getenv envGetter, client *ecoflowprivate.Client, out io.Writer) error {
	if opts.writeCandidateCount() != 1 {
		return fmt.Errorf("set only one command per EcoFlow private probe run")
	}
	command := "set_ac_charge_power"
	if opts.backupReserveSocSet {
		command = "set_backup_reserve_soc"
	} else if opts.gridBypassDisabledSet {
		command = "set_grid_bypass_disabled"
	} else if opts.minDischargeSocSet {
		command = "set_min_discharge_soc"
	} else if opts.maxChargeSocSet {
		command = "set_max_charge_soc"
	} else if opts.energyBackupEnabledSet {
		command = "set_energy_backup_enabled"
	}
	if opts.energyBackupEnabledSet && !opts.energyBackupStartSocSet {
		return fmt.Errorf("--energy-backup-start-soc is required with --energy-backup-enabled")
	}
	guards := ecoflowprivate.WriteGuards{
		MockMode:                envBool(getenv, "MOCK_MODE", true),
		SimulationMode:          envBool(getenv, "SIMULATION_MODE", true),
		EnableRealControl:       envBool(getenv, "ENABLE_REAL_CONTROL", false),
		AutoControlEnabled:      envBool(getenv, "AUTO_CONTROL_ENABLED", false),
		AllowAutoControlOverlap: opts.allowAutoControlOverlap,
		ConfirmEcoFlowWrite:     getenv("CONFIRM_ECOFLOW_WRITE"),
		Execute:                 opts.execute,
		AllowPrivateAPIWrite:    opts.allowPrivateAPIWrite,
		Command:                 command,
	}
	if !opts.execute {
		var payload ecoflowprivate.CommandPayload
		var err error
		switch {
		case opts.setACChargeWSet:
			payload, err = client.BuildDryRunACChargePower(opts.setACChargeW)
		case opts.backupReserveSocSet:
			payload, err = client.BuildDryRunBackupReserve(opts.backupReserveSoc)
		case opts.gridBypassDisabledSet:
			payload, err = client.BuildDryRunGridBypassDisabled(opts.gridBypassDisabled)
		case opts.minDischargeSocSet:
			payload, err = client.BuildDryRunMinDischargeSoc(opts.minDischargeSoc)
		case opts.maxChargeSocSet:
			payload, err = client.BuildDryRunMaxChargeSoc(opts.maxChargeSoc)
		case opts.energyBackupEnabledSet:
			payload, err = client.BuildDryRunEnergyBackupEnabled(opts.energyBackupEnabled, opts.energyBackupStartSoc)
		}
		if err != nil {
			return err
		}
		return writeJSON(out, output{
			Mode:   "dry-run",
			Status: ecoflowprivate.Status{},
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

	var status ecoflowprivate.Status
	var err error
	switch {
	case opts.setACChargeWSet:
		status, err = client.ExecuteACChargePower(ctx, opts.setACChargeW, guards)
	case opts.backupReserveSocSet:
		status, err = client.ExecuteBackupReserve(ctx, opts.backupReserveSoc, guards)
	case opts.gridBypassDisabledSet:
		status, err = client.ExecuteGridBypassDisabled(ctx, opts.gridBypassDisabled, guards)
	case opts.minDischargeSocSet:
		status, err = client.ExecuteMinDischargeSoc(ctx, opts.minDischargeSoc, guards)
	case opts.maxChargeSocSet:
		status, err = client.ExecuteMaxChargeSoc(ctx, opts.maxChargeSoc, guards)
	case opts.energyBackupEnabledSet:
		status, err = client.ExecuteEnergyBackupEnabled(ctx, opts.energyBackupEnabled, opts.energyBackupStartSoc, guards)
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
	flags.BoolVar(&opts.gridBypassDisabled, "grid-bypass-disabled", false, "optional grid bypass disabled command for dry-run or one-shot execute")
	flags.IntVar(&opts.minDischargeSoc, "min-discharge-soc", 0, "optional minimum discharge SOC command for dry-run or one-shot execute")
	flags.IntVar(&opts.maxChargeSoc, "max-charge-soc", 0, "optional maximum charge SOC command for dry-run or one-shot execute")
	flags.BoolVar(&opts.energyBackupEnabled, "energy-backup-enabled", false, "optional energy backup enable command for dry-run or one-shot execute")
	flags.IntVar(&opts.energyBackupStartSoc, "energy-backup-start-soc", 0, "required backup start SOC when --energy-backup-enabled is set")
	flags.StringVar(&opts.rawOutputDir, "raw-output-dir", "", "optional directory for saving read-only raw MQTT reply payloads")
	flags.BoolVar(&opts.inspectFields, "inspect-fields", false, "include generic protobuf field observations for read-only or offline-fixture payloads")
	flags.BoolVar(&opts.execute, "execute", false, "send one real private MQTT write command")
	flags.BoolVar(&opts.allowPrivateAPIWrite, "allow-private-api-write", false, "required together with --execute for private MQTT write")
	flags.BoolVar(&opts.allowAutoControlOverlap, "allow-auto-control-overlap", false, "allow one-shot EcoFlow private write while AUTO_CONTROL_ENABLED=true")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "set-ac-charge-w":
			opts.setACChargeWSet = true
		case "backup-reserve-soc":
			opts.backupReserveSocSet = true
		case "grid-bypass-disabled":
			opts.gridBypassDisabledSet = true
		case "timeout":
			opts.timeoutSet = true
		case "min-discharge-soc":
			opts.minDischargeSocSet = true
		case "max-charge-soc":
			opts.maxChargeSocSet = true
		case "energy-backup-enabled":
			opts.energyBackupEnabledSet = true
		case "energy-backup-start-soc":
			opts.energyBackupStartSocSet = true
		}
	})
	return opts, nil
}

func saveRawMessages(dir string, replies []ecoflowprivate.MQTTMessage, now time.Time) ([]rawCaptureFile, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	prefix := now.Format("20060102-150405")
	files := make([]rawCaptureFile, 0, len(replies))
	for index, reply := range replies {
		kind := rawTopicKind(reply.Topic)
		path := filepath.Join(dir, fmt.Sprintf("%s-%02d-%s.bin", prefix, index, kind))
		if err := os.WriteFile(path, reply.Payload, 0o600); err != nil {
			return nil, err
		}
		files = append(files, rawCaptureFile{Index: index, Kind: kind, File: path, Bytes: len(reply.Payload)})
	}
	return files, nil
}

func inspectReplies(replies []ecoflowprivate.MQTTMessage, enabled bool) ([]ecoflowprivate.SnapshotField, error) {
	if !enabled {
		return nil, nil
	}
	fields := []ecoflowprivate.SnapshotField{}
	for _, reply := range replies {
		inspected, err := ecoflowprivate.InspectSnapshotFields(reply.Payload)
		if err != nil {
			return fields, err
		}
		fields = append(fields, inspected...)
	}
	return fields, nil
}

func inspectRawFields(raw []byte, enabled bool) ([]ecoflowprivate.SnapshotField, error) {
	if !enabled {
		return nil, nil
	}
	return ecoflowprivate.InspectSnapshotFields(raw)
}

func rawTopicKind(topic string) string {
	switch {
	case strings.HasSuffix(topic, "/thing/property/get_reply"):
		return "get-reply"
	case strings.Contains(topic, "/device/property/"):
		return "device-property"
	case strings.HasSuffix(topic, "/thing/property/set_reply"):
		return "set-reply"
	default:
		return "reply"
	}
}

func (opts options) hasWriteCandidate() bool {
	return opts.writeCandidateCount() > 0
}

func (opts options) writeCandidateCount() int {
	count := 0
	if opts.setACChargeWSet {
		count++
	}
	if opts.backupReserveSocSet {
		count++
	}
	if opts.gridBypassDisabledSet {
		count++
	}
	if opts.minDischargeSocSet {
		count++
	}
	if opts.maxChargeSocSet {
		count++
	}
	if opts.energyBackupEnabledSet {
		count++
	}
	return count
}

func buildConfig(opts options, getenv envGetter) ecoflowprivate.Config {
	timeout := opts.timeout
	if !opts.timeoutSet {
		if timeoutSeconds := envInt(getenv, "ECOFLOW_DELTA3_TIMEOUT_SEC", 0); timeoutSeconds > 0 {
			timeout = time.Duration(timeoutSeconds) * time.Second
		}
	}
	return ecoflowprivate.Config{
		PrivateAPIHost: firstNonEmpty(opts.privateAPIHost, getenv("ECOFLOW_PRIVATE_API_HOST"), ecoflowprivate.DefaultPrivateAPIHost),
		Email:          getenv("ECOFLOW_PRIVATE_EMAIL"),
		Password:       getenv("ECOFLOW_PRIVATE_PASSWORD"),
		DeviceSN:       firstNonEmpty(opts.sn, getenv("ECOFLOW_DELTA3_DEVICE_SN")),
		DeviceType:     firstNonEmpty(opts.deviceType, getenv("ECOFLOW_DELTA3_DEVICE_TYPE"), "DELTA_3"),
		MQTTClientID:   getenv("ECOFLOW_DELTA3_MQTT_CLIENT_ID"),
		Timeout:        timeout,
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

func envInt(getenv envGetter, key string, def int) int {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return def
	}
	parsed, err := strconv.Atoi(raw)
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
