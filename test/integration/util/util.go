// +build integration

/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"k8s.io/kubernetes/pkg/api"
	commonutil "k8s.io/minikube/pkg/util"
)

type MinikubeRunner struct {
	T          *testing.T
	BinaryPath string
	Args       string
}

func (k *KubectlRunner) IsPodReady(p *api.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == "Ready" {
			if cond.Status == "True" {
				return true
			}
			k.T.Logf("Pod %s not ready. Ready: %s. Reason: %s", p.Name, cond.Status, cond.Reason)
			return false /**/
		}
	}
	k.T.Logf("Unable to find ready pod condition: %v", p.Status.Conditions)
	return false
}

func (m *MinikubeRunner) RunCommand(command string, checkError bool) string {
	commandArr := strings.Split(command, " ")
	path, _ := filepath.Abs(m.BinaryPath)
	cmd := exec.Command(path, commandArr...)
	stdout, err := cmd.Output()

	if checkError && err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			m.T.Fatalf("Error running command: %s %s. Output: %s", command, exitError.Stderr, stdout)
		} else {
			m.T.Fatalf("Error running command: %s %s. Output: %s", command, err, stdout)
		}
	}
	return string(stdout)
}

func (m *MinikubeRunner) Start() {
	m.RunCommand(fmt.Sprintf("start %s", m.Args), true)
}

func (m *MinikubeRunner) EnsureRunning() {
	if m.GetStatus() != "Running" {
		m.Start()
	}
	m.CheckStatus("Running")
}

func (m *MinikubeRunner) SetEnvFromEnvCmdOutput(dockerEnvVars string) error {
	re := regexp.MustCompile(`(\w+?) ?= ?"?(.+?)"?\n`)
	matches := re.FindAllStringSubmatch(dockerEnvVars, -1)
	seenEnvVar := false
	for _, m := range matches {
		seenEnvVar = true
		key, val := m[1], m[2]
		os.Setenv(key, val)
	}
	if !seenEnvVar {
		return fmt.Errorf("Error: No environment variables were found in docker-env command output: %s", dockerEnvVars)
	}
	return nil
}

func (m *MinikubeRunner) GetStatus() string {
	return m.RunCommand("status --format={{.MinikubeStatus}}", true)
}

func (m *MinikubeRunner) CheckStatus(desired string) {
	s := m.GetStatus()
	if s != desired {
		m.T.Fatalf("Machine is in the wrong state: %s, expected  %s", s, desired)
	}
}

type KubectlRunner struct {
	T          *testing.T
	BinaryPath string
}

func NewKubectlRunner(t *testing.T) *KubectlRunner {
	p, err := exec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("Couldn't find kubectl on path.")
	}
	return &KubectlRunner{BinaryPath: p, T: t}
}

func (k *KubectlRunner) RunCommandParseOutput(args []string, outputObj interface{}) error {
	args = append(args, "-o=json")
	output, err := k.RunCommand(args)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(output))
	if err := d.Decode(outputObj); err != nil {
		return err
	}
	return nil
}

func (k *KubectlRunner) RunCommand(args []string) (stdout []byte, err error) {
	inner := func() error {
		cmd := exec.Command(k.BinaryPath, args...)
		stdout, err = cmd.CombinedOutput()
		if err != nil {
			k.T.Logf("Error %s running command %s. Return code: %s", stdout, args, err)
			return &commonutil.RetriableError{Err: fmt.Errorf("Error running command. Error  %s. Output: %s", err, stdout)}
		}
		return nil
	}

	err = commonutil.RetryAfter(3, inner, 2*time.Second)
	return stdout, err
}

func (k *KubectlRunner) CreateRandomNamespace() string {
	const strLen = 20
	name := genRandString(strLen)
	if _, err := k.RunCommand([]string{"create", "namespace", name}); err != nil {
		k.T.Fatalf("Error creating namespace: %s", err)
	}
	return name
}

func genRandString(strLen int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	rand.Seed(time.Now().UTC().UnixNano())
	result := make([]byte, strLen)
	for i := 0; i < strLen; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

func (k *KubectlRunner) DeleteNamespace(namespace string) error {
	_, err := k.RunCommand([]string{"delete", "namespace", namespace})
	return err
}

func (k *KubectlRunner) GetPod(name, namespace string) *api.Pod {
	p := &api.Pod{}
	if err := k.RunCommandParseOutput([]string{"get", "pod", name, "--namespace=" + namespace}, p); err != nil {
		k.T.Fatalf("Error checking pod status: %s", err)
	}
	return p
}
