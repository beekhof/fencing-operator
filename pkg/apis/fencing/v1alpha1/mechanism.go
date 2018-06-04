package v1alpha1

import (
	"fmt"

	"github.com/beekhof/fencing-operator/pkg/config"
	"github.com/beekhof/fencing-operator/pkg/util"
	//	"github.com/beekhof/fencing-operator/pkg/constants"

	"github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewFencingMethodFromConfig(cfg *config.Config) (error, *FencingMethod) {
	mechanisms := []FencingMechanism{}
	seconds := int32(cfg.GetInt("requireAfterSeconds"))

	for _, subcfg := range cfg.GetSubConfigArray("mechanisms") {
		err, m := newFencingMechanismFromConfig(subcfg)
		if err != nil {
			return fmt.Errorf("Bad method '%v': %v", cfg.GetString("name"), err), nil
		}
		mechanisms = append(mechanisms, *m)
	}

	logrus.Infof("Creating internal representation for %v", cfg.GetString("name"))
	return nil, &FencingMethod{
		Name:                cfg.GetString("name"),
		Retries:             1,
		RequireAfterSeconds: &seconds,
		StopOnSuccess:       cfg.GetBoolWithDefault("stopOnSuccess", true),
		Mechanisms:          mechanisms,
	}
	//	method.GetSliceOfStrings("namespaces"),
	//	method.GetString("openshift.namespace")
}

func newFencingMechanismFromConfig(cfg *config.Config) (error, *FencingMechanism) {
	dcs := []FencingDynamicConfig{}
	seconds := int32(cfg.GetInt("timeoutSeconds"))

	for _, subcfg := range cfg.GetSubConfigArray("dynamicConfig") {
		err, d := newDynamicAttributeFromConfig(subcfg)
		if err != nil {
			return err, nil
		}
		dcs = append(dcs, *d)
	}

	if err, container := newContainerFromConfig(cfg.GetSubConfig("container")); err != nil {
		return fmt.Errorf("Bad mechanism: %v", err), nil

	} else {
		return nil, &FencingMechanism{
			Container:      container,
			PassTargetAs:   cfg.GetString("passTargetAs"),
			ArgumentFormat: cfg.GetString("argumentFormat"),
			TimeoutSeconds: &seconds,
			Config:         cfg.GetMapOfStrings("config"),
			DynamicConfig:  dcs,
			Secrets:        cfg.GetMapOfStrings("secrets"),
		}
	}

}

func newContainerFromConfig(cfg *config.Config) (error, *v1.Container) {

	if subcfg == nil {
		return fmt.Errorf("No container specified"), nil
	}

	c := &v1.Container{
		Name:  cfg.GetString("name"),
		Image: cfg.GetString("image"),
		Env:   []v1.EnvVar{},
		// TODO: Either find a generic way or implement the other fields too
		// This might work: func (m *Container) Unmarshal(dAtA []byte) error
	}

	if cmd := cfg.GetSliceOfStrings("command"); cmd != nil {
		c.Command = cmd
	}

	for _, env := range cfg.GetSubConfigArray("env") {
		c.Env = append(c.Env, v1.EnvVar{
			Name:  env.GetString("name"),
			Value: env.GetString("value"),
		})
	}

	return nil, c
}

func newDynamicAttributeFromConfig(cfg *config.Config) (error, *FencingDynamicConfig) {
	return nil, &FencingDynamicConfig{
		Field:   cfg.GetString("field"),
		Default: cfg.GetString("default"),
		Values:  cfg.GetMapOfStrings("values"),
	}
}

func (m *FencingMechanism) CreateContainer(target string, secretsDir string) (error, *v1.Container) {
	container := m.Container.DeepCopy()

	if err, cmd := m.getContainerCommand(target); err != nil {
		return err, nil
	} else if len(cmd) > 0 {
		container.Command = cmd
	}

	if err, env := m.getContainerEnv(target, secretsDir); err != nil {
		return err, nil
	} else {
		container.Env = env
	}

	return nil, container
}

func (m *FencingMechanism) getContainerCommand(target string) (error, []string) {
	if m.ArgumentFormat == "env" {
		return nil, m.Container.Command
	}
	if m.ArgumentFormat == "cli" {
		command := m.Container.Command

		for name, value := range m.Config {
			command = append(command, fmt.Sprintf("--%s", name))
			command = append(command, value)
		}

		for _, dc := range m.DynamicConfig {
			command = append(command, fmt.Sprintf("--%s", dc.Field))
			if value, ok := dc.Lookup(target); ok {
				command = append(command, value)
			} else {
				return fmt.Errorf("No value of '%s' found for '%s'", dc.Field, target), []string{}
			}
		}

		if len(m.PassTargetAs) > 0 {
			command = append(command, fmt.Sprintf("--%s", m.PassTargetAs))
		}

		command = append(command, target)
		return nil, command
	}
	return fmt.Errorf("ArgumentFormat %s not supported", m.ArgumentFormat), []string{}
}

func (m *FencingMechanism) getContainerEnv(target string, secretsDir string) (error, []v1.EnvVar) {
	env := []v1.EnvVar{
		{
			Name:  "SECRET_FORMAT",
			Value: m.ArgumentFormat,
		},
	}

	for _, val := range m.Container.Env {
		env = append(env, val)
	}

	for name, value := range m.Secrets {
		// Relies on an ENTRYPOINT or CMD that looks for SECRETPATH-field=/path/to/file and re-exports: field=$(cat /path/to/file) or --field=$(cat /path/to/file)
		env = append(env, v1.EnvVar{
			Name:  fmt.Sprintf("SECRETPATH_%s", name),
			Value: fmt.Sprintf("%s/%s", secretsDir, value),
		})
	}

	if m.ArgumentFormat == "cli" {
		return nil, env
	}

	if m.ArgumentFormat == "env" {
		for name, value := range m.Config {
			env = append(env, v1.EnvVar{
				Name:  name,
				Value: value,
			})
		}

		for _, dc := range m.DynamicConfig {
			if value, ok := dc.Lookup(target); ok {
				env = append(env, v1.EnvVar{
					Name:  dc.Field,
					Value: value,
				})
			} else {
				return fmt.Errorf("No value of '%s' found for '%s'", dc.Field, target), nil
			}
		}

		if len(m.PassTargetAs) > 0 {
			env = append(env, v1.EnvVar{
				Name:  m.PassTargetAs,
				Value: target,
			})
		}

		return nil, env
	}
	return fmt.Errorf("ArgumentFormat %s not supported", m.ArgumentFormat), env
}

func (dc *FencingDynamicConfig) Lookup(key string) (string, bool) {
	if val, ok := dc.Values[key]; ok {
		return val, true
	} else if len(dc.Default) > 0 {
		return dc.Default, true
	}
	return "", false
}
