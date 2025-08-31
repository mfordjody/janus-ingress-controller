package config

import (
	"encoding/json"
	"fmt"
	"time"
)

var (
	ControllerConfig = NewDefaultConfig()
)

const (
	DefaultLeaderElectionID = "janus-ingress-gateway-leader"
	DefaultControllerName   = "janus.apache.org/janus-ingress-controller"
	DefaultLogLevel         = "info"
	DefaultMetricsAddr      = ":8080"
	DefaultProbeAddr        = ":8081"
)

type Config struct {
	LogLevel         string          `json:"log_level" yaml:"log_level"`
	ControllerName   string          `json:"controller_name" yaml:"controller_name"`
	LeaderElectionID string          `json:"leader_election_id" yaml:"leader_election_id"`
	MetricsAddr      string          `json:"metrics_addr" yaml:"metrics_addr"`
	EnableHTTP2      bool            `json:"enable_http2" yaml:"enable_http2"`
	ProbeAddr        string          `json:"probe_addr" yaml:"probe_addr"`
	SecureMetrics    bool            `json:"secure_metrics" yaml:"secure_metrics"`
	LeaderElection   *LeaderElection `json:"leader_election" yaml:"leader_election"`
}

type LeaderElection struct {
	LeaseDuration TimeDuration `json:"lease_duration,omitempty" yaml:"lease_duration,omitempty"`
	RenewDeadline TimeDuration `json:"renew_deadline,omitempty" yaml:"renew_deadline,omitempty"`
	RetryPeriod   TimeDuration `json:"retry_period,omitempty" yaml:"retry_period,omitempty"`
	Disable       bool         `json:"disable,omitempty" yaml:"disable,omitempty"`
}

func SetControllerConfig(cfg *Config) {
	ControllerConfig = cfg
}

func NewDefaultConfig() *Config {
	return &Config{
		LogLevel:         DefaultLogLevel,
		ControllerName:   DefaultControllerName,
		LeaderElectionID: DefaultLeaderElectionID,
		ProbeAddr:        DefaultProbeAddr,
		MetricsAddr:      DefaultMetricsAddr,
		LeaderElection:   NewLeaderElection(),
	}
}

func NewLeaderElection() *LeaderElection {
	return &LeaderElection{
		LeaseDuration: TimeDuration{Duration: 30 * time.Second},
		RenewDeadline: TimeDuration{Duration: 20 * time.Second},
		RetryPeriod:   TimeDuration{Duration: 2 * time.Second},
		Disable:       false,
	}
}

// TimeDuration is yet another time.Duration but implements json.Unmarshaler
// and json.Marshaler, yaml.Unmarshaler and yaml.Marshaler interfaces so one
// can use "1h", "5s" and etc in their json/yaml configurations.
//
// Note the format to represent time is same as time.Duration.
// See the comments about time.ParseDuration for more details.
type TimeDuration struct {
	time.Duration `json:",inline"`
}

func (d *TimeDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *TimeDuration) UnmarshalJSON(data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	switch v := value.(type) {
	case float64:
		d.Duration = time.Duration(v)
	case string:
		dur, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		d.Duration = dur
	default:
		return fmt.Errorf("unknown type: %T", v)
	}
	return nil
}

func (d *TimeDuration) MarshalYAML() (any, error) {
	return d.String(), nil
}

func (d *TimeDuration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}
