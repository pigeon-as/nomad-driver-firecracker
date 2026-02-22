// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/plugins"
)

func main() {
	plugins.ServeCtx(factory)
}

func factory(ctx context.Context, log hclog.Logger) interface{} {
	return firecracker.NewPlugin(ctx, log)
}
