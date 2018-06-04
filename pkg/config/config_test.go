package config

import (
	"flag"
	//"fmt"
	"testing"
	log "github.com/sirupsen/logrus"
)

var podName, namespace, kubeconfig string

func init() {
	flag.StringVar(&podName, "pod", "", "pod to run test against")
	flag.StringVar(&namespace, "namespace", "", "namespace to which the pod belongs")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
}

func TestConfig(t *testing.T) {
	str := "methods:\n- name: echo\n  mechanisms:\n  # The operator attempts sets in order until one succeeds\n  # All mechanisms in a set are required to succeed in order for the set to succeed.\n  #\n  # A CLI tool/extension will be provided that allows an admin to\n  # create FencingReques CRs and unfence one way operations like\n  # network and disk based fencing events.\n  - driver: echo\n    module: ipmilan\n    dynamic_config:\n    - field: ip\n      default: 127.0.0.1\n      # If no default is supplied, an error will be logged and the\n      # mechanism will be considered to have failed\n      values:\n      - somehost: 1.2.3.4\n      - otherhost: 1.2.3.5\n    config:\n    - user: admin\n    secrets:\n    - password: ipmi-secret\n"
	
	cfg, err :=  NewConfigFromString(str)

	if err != nil {
		t.Fatal("empty strings are not ignored")
	}
	if cfg == nil {
		t.Fatal("empty strings are not ignored")
	}

	for _, subcfg := range cfg.GetSubConfigArray("methods") {
		for _, mcfg := range subcfg.GetSubConfigArray("mechanisms") {
			secrets := mcfg.GetMapOfStrings("secrets")
			if len(secrets) == 0 {
				t.Fatalf("no secret found: %v", mcfg.config["secrets"])
			} else {
				log.Infof("Got: %v", secrets)
			}
		}
	}
}
