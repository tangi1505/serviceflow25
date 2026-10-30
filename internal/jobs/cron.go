// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package jobs

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

// parseCron accepts the five-field form (minute hour day month weekday) with ranges, lists
// and steps, plus @hourly, @daily, @weekly, @monthly (Hauptplan E85). Times are local to the
// process (TZ of the container). The returned schedule is River's PeriodicSchedule.
func parseCron(expr string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("%w: schedule is empty", ErrInvalid)
	}
	s, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: schedule %q: %w", ErrInvalid, expr, err)
	}
	return s, nil
}
