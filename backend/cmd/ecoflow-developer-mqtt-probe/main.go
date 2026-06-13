package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflowdeveloper"
)

type envGetter func(string) string

type options struct {
	sn             string
	privateAPIHost string
	timeout        time.Duration
	watch          time.Duration
}

type output struct {
	Mode             string                            `json:"mode"`
	CycleCount       *int                              `json:"cycleCount,omitempty"`
	CycleCountSource string                            `json:"cycleCountSource,omitempty"`
	Key              string                            `json:"key,omitempty"`
	QuotaKeyCount    int                               `json:"quotaKeyCount"`
	CycleCandidates  []ecoflowdeveloper.CycleCandidate `json:"cycleCandidates,omitempty"`
	Write            map[string]any                    `json:"write"`
}

type watchOutput struct {
	Mode              string                            `json:"mode"`
	Event             string                            `json:"event"`
	TopicKind         string                            `json:"topicKind,omitempty"`
	PayloadBytes      int                               `json:"payloadBytes,omitempty"`
	MessageCount      *int                              `json:"messageCount,omitempty"`
	QuotaMessageCount *int                              `json:"quotaMessageCount,omitempty"`
	CycleCount        *int                              `json:"cycleCount,omitempty"`
	CycleCountSource  string                            `json:"cycleCountSource,omitempty"`
	Key               string                            `json:"key,omitempty"`
	QuotaKeyCount     int                               `json:"quotaKeyCount,omitempty"`
	CycleCandidates   []ecoflowdeveloper.CycleCandidate `json:"cycleCandidates,omitempty"`
	KeyNames          []string                          `json:"keyNames,omitempty"`
	ParseError        string                            `json:"parseError,omitempty"`
	Write             map[string]any                    `json:"write"`
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
	client := ecoflowdeveloper.NewClient(cfg)
	if opts.watch > 0 {
		return watchQuota(ctx, client, opts.watch, out)
	}
	status, err := client.ReadCycleStatus(ctx)
	if err != nil {
		return err
	}
	return writeJSON(out, output{
		Mode:             "read-only",
		CycleCount:       status.CycleCount,
		CycleCountSource: status.CycleCountSource,
		Key:              status.Key,
		QuotaKeyCount:    status.QuotaKeyCount,
		CycleCandidates:  status.CycleCandidates,
		Write: map[string]any{
			"wouldSend": false,
			"sent":      false,
			"reason":    "read-only Developer MQTT quota subscribe",
		},
	})
}

func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("ecoflow-developer-mqtt-probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var timeoutSec int
	var watchSec int
	var opts options
	fs.StringVar(&opts.sn, "sn", "", "EcoFlow device serial number")
	fs.StringVar(&opts.privateAPIHost, "private-api-host", "", "EcoFlow private app API host")
	fs.IntVar(&timeoutSec, "timeout-sec", 0, "MQTT subscribe timeout in seconds")
	fs.IntVar(&watchSec, "watch-sec", 0, "read-only quota/status watch duration in seconds")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if timeoutSec > 0 {
		opts.timeout = time.Duration(timeoutSec) * time.Second
	}
	if watchSec > 0 {
		opts.watch = time.Duration(watchSec) * time.Second
	}
	return opts, nil
}

func buildConfig(opts options, getenv envGetter) ecoflowdeveloper.Config {
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = envDurationSeconds(getenv, "ECOFLOW_DELTA3_TIMEOUT_SEC", 20*time.Second)
	}
	sn := opts.sn
	if sn == "" {
		sn = getenv("ECOFLOW_DEVICE_SN")
	}
	privateAPIHost := opts.privateAPIHost
	if privateAPIHost == "" {
		privateAPIHost = getenv("ECOFLOW_PRIVATE_API_HOST")
	}
	return ecoflowdeveloper.Config{
		AccessKey:      getenv("ECOFLOW_ACCESS_KEY"),
		SecretKey:      getenv("ECOFLOW_SECRET_KEY"),
		BaseURL:        getenv("ECOFLOW_BASE_URL"),
		PrivateAPIHost: privateAPIHost,
		Email:          getenv("ECOFLOW_PRIVATE_EMAIL"),
		Password:       getenv("ECOFLOW_PRIVATE_PASSWORD"),
		DeviceSN:       sn,
		MQTTClientID:   getenv("ECOFLOW_DELTA3_MQTT_CLIENT_ID"),
		Timeout:        timeout,
	}
}

func envDurationSeconds(getenv envGetter, key string, fallback time.Duration) time.Duration {
	raw := getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func watchQuota(ctx context.Context, client *ecoflowdeveloper.Client, duration time.Duration, out io.Writer) error {
	watchCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	encoder := json.NewEncoder(out)
	write := func(v watchOutput) error {
		v.Mode = "read-only-watch"
		if v.Write == nil {
			v.Write = readOnlyWriteState("read-only Developer MQTT quota/status subscribe")
		}
		return encoder.Encode(v)
	}
	if err := write(watchOutput{Event: "start"}); err != nil {
		return err
	}
	messages := 0
	quotaMessages := 0
	err := client.WatchQuota(watchCtx, func(msg ecoflowdeveloper.QuotaMessage) {
		messages++
		if msg.TopicKind == "quota" {
			quotaMessages++
		}
		_ = write(watchOutput{
			Event:            "message",
			TopicKind:        msg.TopicKind,
			PayloadBytes:     msg.PayloadBytes,
			CycleCount:       msg.CycleStatus.CycleCount,
			CycleCountSource: msg.CycleStatus.CycleCountSource,
			Key:              msg.CycleStatus.Key,
			QuotaKeyCount:    msg.CycleStatus.QuotaKeyCount,
			CycleCandidates:  msg.CycleStatus.CycleCandidates,
			KeyNames:         msg.KeyNames,
			ParseError:       msg.ParseError,
		})
	})
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return err
	}
	return write(watchOutput{
		Event:             "complete",
		MessageCount:      &messages,
		QuotaMessageCount: &quotaMessages,
	})
}

func readOnlyWriteState(reason string) map[string]any {
	return map[string]any{
		"wouldSend": false,
		"sent":      false,
		"reason":    reason,
	}
}
