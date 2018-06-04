package util

import (
	"encoding/json"
	"strings"

	"github.com/sirupsen/logrus"
)

func JsonLogObject(text string, spec interface{}) {
	specBytes, err := json.MarshalIndent(spec, "", "    ")
	if err != nil {
		logrus.Errorf("failed to marshal spec for '%v': %v", text, err)
	}

	logrus.Info(text)
	for _, m := range strings.Split(string(specBytes), "\n") {
		logrus.Info(m)
	}
}
