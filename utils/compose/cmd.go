// Copyright 2022-2024 Sauce Labs Inc., all rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package compose

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"time"
)

type Command struct {
	rt     string
	dir    string
	stdout io.Writer
	stderr io.Writer
}

func NewCommand(c *Compose, dir string, stdout, stderr io.Writer) (*Command, error) {
	rt := os.Getenv("CONTAINER_RUNTIME")
	if rt == "" {
		rt = "docker"
	}

	if dir == "" {
		d, err := os.MkdirTemp("", "compose-*")
		if err != nil {
			return nil, fmt.Errorf("create temp file: %w", err)
		}
		dir = d
	}

	f, err := os.Create(path.Join(dir, "compose.yaml"))
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	if _, err := c.WriteTo(f); err != nil {
		return nil, fmt.Errorf("write compose to file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	return &Command{
		rt:     rt,
		dir:    dir,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

func (c *Command) Runtime() string {
	return c.rt
}

func (c *Command) Project() string {
	if p := os.Getenv("COMPOSE_PROJECT_NAME"); p != "" {
		return p
	}

	return path.Base(c.dir)
}

func (c *Command) File() string {
	return path.Join(c.dir, "compose.yaml")
}

func (c *Command) Close() error {
	return os.RemoveAll(c.dir)
}

func (c *Command) Up(args ...string) error {
	if slices.ContainsFunc(args, func(s string) bool { return s == "-d" || s == "--detach" }) {
		return c.quietRun(c.cmd("up", args))
	}

	return c.run(c.cmd("up", args))
}

func (c *Command) Down(args ...string) error {
	return c.quietRun(c.cmd("down", args))
}

func (c *Command) Ps(args ...string) error {
	return c.run(c.cmd("ps", args))
}

func (c *Command) Logs(args ...string) error {
	return c.run(c.cmd("logs", args))
}

const healthy = "healthy"

type serviceHealth struct {
	Service string
	Health  string
}

func (c *Command) health() ([]serviceHealth, error) {
	cmd := c.cmd("ps", []string{"--format", "json"})
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		stdout.WriteTo(c.stdout)
		stderr.WriteTo(c.stderr)
	}

	var res []serviceHealth
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var ci serviceHealth
		if err := dec.Decode(&ci); err != nil { //nolint:musttag // the JSON output is CamelCase
			return nil, fmt.Errorf("decode container info: %w", err)
		}
		res = append(res, ci)
	}
	return res, nil
}

func (c *Command) Wait(interval, timeout time.Duration) error {
	to := time.NewTimer(timeout)
	defer to.Stop()
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-to.C:
			return errors.New("timeout waiting for services to start")
		case <-t.C:
			services, err := c.health()
			if err != nil {
				return err
			}

			n := 0
			for i := range services {
				s := &services[i]
				if s.Health == "" || s.Health == healthy {
					n++
				}
			}
			if n == len(services) {
				return nil
			}
		}
	}
}

func (c *Command) cmd(subcmd string, args []string) *exec.Cmd {
	allArgs := []string{
		"compose",
		subcmd,
	}
	allArgs = append(allArgs, args...)

	cmd := exec.Command(c.rt, allArgs...) //nolint:gosec // this is a command runner
	cmd.Dir = c.dir

	return cmd
}

func (c *Command) quietRun(cmd *exec.Cmd) error {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		stdout.WriteTo(c.stdout)
		stderr.WriteTo(c.stderr)
	}
	return err
}

func (c *Command) run(cmd *exec.Cmd) error {
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	return cmd.Run()
}