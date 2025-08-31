package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"text/template"
)

func GetConfigFromFile(filename string) (*Config, error) {
	cfg := NewDefaultConfig()
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	envVarMap := map[string]string{}
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		envVarMap[pair[0]] = pair[1]
	}

	tpl := template.New("text").Option("missingkey=error")
	tpl, err = tpl.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("error parsing configuration template %v", err)
	}
	buf := bytes.NewBufferString("")
	err = tpl.Execute(buf, envVarMap)
	if err != nil {
		return nil, fmt.Errorf("error execute configuration template %v", err)
	}

	if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
		err = yaml.Unmarshal(buf.Bytes(), cfg)
	} else {
		err = json.Unmarshal(buf.Bytes(), cfg)
	}

	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func GetControllerName() string {
	return ControllerConfig.ControllerName
}
