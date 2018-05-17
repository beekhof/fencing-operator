package v1alpha1

import (
	"os"
	"fmt"
	"github.com/beekhof/fencing-operator/pkg/constants"

	"k8s.io/api/core/v1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *FencingMechanism)CreateContainer(target string, secretsDir string) (error, *v1.Container) {
	switch m.Driver {
	case "openstack":
		return m.openstackContainer(target, secretsDir)
	case "baremetal":
		return m.baremetalContainer(target, secretsDir, false)
	case "echo":
		return m.baremetalContainer(target, secretsDir, true)
	}
	return fmt.Errorf("Driver %s not supported", m.Driver), nil
}

func (m *FencingMechanism)openstackContainer(target string, secretsDir string) (error, *v1.Container) {
	env := []v1.EnvVar{
		{
			Name:  "SECRET_FORMAT",
			Value: "env",
		},
	}

	for name, value := range m.Config {
		env = append(env, v1.EnvVar{
			Name:  name,
			Value: value,
			})
	}

	for name, value := range m.Secrets {
		// Relies on an ENTRYPOINT that looks for SECRETPATH-field=/path/to/file and re-exports: field=$(cat /path/to/file)
		env = append(env, v1.EnvVar{
			Name:  fmt.Sprintf("SECRETPATH_%s", name),
			Value: fmt.Sprintf("%s/%s", secretsDir, value),
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

	return nil, &v1.Container{
		Name: "nova",
		Image:   m.getImage(),
		Command: []string{"/bin/nova", "delete", target},
		Env: env,
	}
}

func (m *FencingMechanism)baremetalContainer(target string, secretsDir string, echo bool) (error, *v1.Container) {
	options := []string{}

	env := []v1.EnvVar{
		{
			Name:  "SECRET_FORMAT",
			Value: "args",
		},
	}

	if echo {
		options = append(options, "/bin/echo")
	}		

	options = append(options, fmt.Sprintf("/sbin/fence_%v", m.Module))
	options = append(options, "-v")

	for name, value := range m.Config {
		options = append(options, fmt.Sprintf("--%s", name))
		options = append(options, value)
	}
	
	for name, value := range m.Secrets {
		// Relies on an ENTRYPOINT that looks for SECRETPATH-field=/path/to/file and add: --field=$(cat /path/to/file) to the command line
		env = append(env, v1.EnvVar{
			Name:  fmt.Sprintf("SECRETPATH_%s", name),
			Value: fmt.Sprintf("%s/%s", secretsDir, value),
			})
	}
	
	for _, dc := range m.DynamicConfig {
		options = append(options, fmt.Sprintf("--%s", dc.Field))
		if value, ok := dc.Lookup(target); ok {
			options = append(options, value)
		} else {
			return fmt.Errorf("No value of '%s' found for '%s'", dc.Field, target), nil
		}
	}

	return nil, &v1.Container{
		Name: "baremetal",
		Image:   m.getImage(),
		Command: options,
		Env: env,
	}
}

func (m *FencingMechanism)getImage() string {
	if len(m.Image) > 0 {
		return m.Image
	}
	switch m.Driver {
	case "openstack":
		return "quay.io/beekhof/openstack-novaclient"
	case "baremetal":
		return "quay.io/beekhof/rhelha-fencing"
	case "echo":
		return "busybox"
	}
	return os.Getenv(constants.EnvOperatorPodImage)
}

func (dc *FencingDynamicConfig)Lookup(key string) (string, bool) {
	if val, ok := dc.Values[key]; ok {
		return val, true
	} else if len(dc.Default) > 0 {
		return dc.Default, true
	} 
	return "", false
}

