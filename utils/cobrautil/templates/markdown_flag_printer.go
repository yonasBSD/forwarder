// Copyright 2023 Sauce Labs Inc. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package templates

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"
)

type MarkdownFlagPrinter struct {
	out       io.Writer
	envPrefix string
}

func NewMarkdownFlagPrinter(out io.Writer, envPrefix string) *MarkdownFlagPrinter {
	return &MarkdownFlagPrinter{
		out:       out,
		envPrefix: envPrefix,
	}
}

func (p *MarkdownFlagPrinter) PrintHelpFlag(f *pflag.Flag) {
	fmt.Fprintf(p.out, p.header(f))
	fmt.Fprint(p.out, "\n\n")

	body := p.body(f)
	body = strings.ReplaceAll(body, ". ", ".\n")
	fmt.Fprintf(p.out, body)
	fmt.Fprintf(p.out, "\n\n")
}

func (p *MarkdownFlagPrinter) header(f *pflag.Flag) string {
	format := "--%s"
	if f.Shorthand != "" {
		format = "-%s, " + format
	} else {
		format = "%s" + format
	}
	format = "### `" + format + "` {#%s}"

	return fmt.Sprintf(format, f.Shorthand, f.Name, f.Name)
}

func (p *MarkdownFlagPrinter) body(f *pflag.Flag) string {
	buf := new(bytes.Buffer)

	fmt.Fprintf(buf, "* Environment variable: `%s`\n", envName(p.envPrefix, f.Name))
	format, usage := flagNameAndUsage(f)
	fmt.Fprintf(buf, "* Value Format: `%s`\n", strings.TrimSpace(format))
	def := f.DefValue
	if def == "[]" {
		def = ""
	}
	if def != "" {
		fmt.Fprintf(buf, "* Default value: `%s`\n", def)
	}
	fmt.Fprintln(buf)

	if f.Deprecated != "" {
		fmt.Fprintf(buf, "DEPRECATED: %s\n\n", f.Deprecated)
	}

	fmt.Fprintf(buf, "%s", usage)

	return buf.String()
}