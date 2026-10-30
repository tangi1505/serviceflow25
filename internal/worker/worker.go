// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package worker runs the background role (Hauptplan E35). It has no jobs yet; River
// (Hauptplan E19) comes with the Plattform-Kern (PL).
package worker

import (
	"context"
	"log/slog"
)

// Run blocks until ctx is cancelled.
func Run(ctx context.Context, logger *slog.Logger) error {
	logger.Info("worker started", "jobs", 0)
	<-ctx.Done()
	logger.Info("worker stopped")
	return nil
}
